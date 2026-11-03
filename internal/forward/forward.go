// SPDX-License-Identifier: GPL-3.0-or-later

// Package forward publishes services of the own network on ports of the
// radio address: a station that connects to 10.73.0.2:2222 gets a
// connection to 192.168.1.10:22, for example, if the rule admits its
// callsign. It works in userspace with TCP, so the operating system needs no
// forwarding or NAT, and the own network stays closed except for exactly
// that service. The firewall opens the port for the same callsigns.
package forward

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/firewall"
)

// Rule publishes Target on Port of the radio address.
type Rule struct {
	Port   uint16 `json:"port"`   // TCP port on the radio address
	Target string `json:"target"` // host:port in the own network, or on this computer
	// Calls are the stations allowed; empty means every station. A callsign
	// without SSID stands for all its SSIDs.
	Calls []string `json:"calls,omitempty"`
	Note  string   `json:"note,omitempty"`
}

// Check reports what is wrong with a rule.
func (r Rule) Check() error {
	if r.Port == 0 {
		return errors.New("forward: port 0")
	}
	host, port, err := net.SplitHostPort(r.Target)
	if err != nil || host == "" {
		return fmt.Errorf("forward: target %q: want host:port, e.g. 192.168.1.10:22", r.Target)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("forward: target %q: port 1-65535", r.Target)
	}
	if strings.ContainsAny(host, " /") {
		return fmt.Errorf("forward: target %q", r.Target)
	}
	for _, c := range r.Calls {
		if _, err := ax25.ParseAddress(c); err != nil {
			return fmt.Errorf("forward: callsign %q: %w", c, err)
		}
	}
	if len(r.Note) > 80 {
		return errors.New("forward: note longer than 80 characters")
	}
	return nil
}

// ParseRule reads "2222=192.168.1.10:22" or "2222=192.168.1.10:22=CALL,CALL"
// as given on the command line.
func ParseRule(s string) (Rule, error) {
	port, rest, ok := strings.Cut(s, "=")
	if !ok {
		return Rule{}, errors.New("forward: want PORT=HOST:PORT, optionally =CALL,CALL")
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return Rule{}, fmt.Errorf("forward: port %q", port)
	}
	target, calls, _ := strings.Cut(rest, "=")
	r := Rule{Port: uint16(n), Target: target}
	if calls != "" {
		for _, c := range strings.Split(calls, ",") {
			r.Calls = append(r.Calls, strings.ToUpper(strings.TrimSpace(c)))
		}
	}
	return r, r.Check()
}

// Stats counts connections.
type Stats struct {
	Connections, Refused, Failed uint64
	BytesIn, BytesOut            uint64 // from the radio to the target, and back
}

// Config sets up a Forwarder.
type Config struct {
	Addr netip.Addr // the radio address to listen on
	// Who names the station behind a client address (station discovery);
	// a rule with callsigns refuses addresses it cannot name.
	Who    func(addr netip.Addr) (ax25.Address, bool)
	Dial   func(ctx context.Context, network, addr string) (net.Conn, error) // nil: net.Dialer
	Logger *slog.Logger
}

// maxConns per rule at the same time.
const maxConns = 8

// Forwarder listens on the ports of its rules.
type Forwarder struct {
	cfg Config

	mu   sync.Mutex
	open map[uint16]*listener

	connections, refused, failed, bytesIn, bytesOut atomic.Uint64
}

type listener struct {
	rule   Rule
	cancel context.CancelFunc
	conns  chan struct{} // a slot per connection
}

// New returns a forwarder without rules.
func New(cfg Config) *Forwarder {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Dial == nil {
		d := &net.Dialer{Timeout: 15 * time.Second}
		cfg.Dial = d.DialContext
	}
	return &Forwarder{cfg: cfg, open: map[uint16]*listener{}}
}

// Set opens the ports of rules and closes the others; ctx ends them all. A
// port that cannot be opened is reported and skipped.
func (f *Forwarder) Set(ctx context.Context, rules []Rule) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := map[uint16]Rule{}
	for _, r := range rules {
		want[r.Port] = r
	}
	for port, l := range f.open {
		if r, ok := want[port]; !ok || !sameRule(r, l.rule) {
			l.cancel()
			delete(f.open, port)
		}
	}
	var errs []error
	for port, r := range want {
		if _, ok := f.open[port]; ok {
			continue
		}
		addr := netip.AddrPortFrom(f.cfg.Addr, port).String()
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			errs = append(errs, fmt.Errorf("forward: %s: %w", addr, err))
			continue
		}
		lctx, cancel := context.WithCancel(ctx)
		l := &listener{rule: r, cancel: cancel, conns: make(chan struct{}, maxConns)}
		f.open[port] = l
		go func() {
			<-lctx.Done()
			ln.Close()
		}()
		go f.accept(lctx, ln, l)
		f.cfg.Logger.Info("port forwarding", "listen", addr, "target", r.Target, "only", r.Calls)
	}
	return errors.Join(errs...)
}

func sameRule(a, b Rule) bool {
	return a.Port == b.Port && a.Target == b.Target && strings.Join(a.Calls, ",") == strings.Join(b.Calls, ",")
}

func (f *Forwarder) accept(ctx context.Context, ln net.Listener, l *listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go f.handle(ctx, c, l)
	}
}

func (f *Forwarder) handle(ctx context.Context, c net.Conn, l *listener) {
	defer c.Close()
	r := l.rule
	from := "?"
	if ap, err := netip.ParseAddrPort(c.RemoteAddr().String()); err == nil {
		from = ap.Addr().String()
		if len(r.Calls) > 0 {
			call, known := ax25.Address{}, false
			if f.cfg.Who != nil {
				call, known = f.cfg.Who(ap.Addr().Unmap())
			}
			if !known || !firewall.Admits(r.Calls, call) {
				f.refused.Add(1)
				f.cfg.Logger.Info("port forwarding refused", "port", r.Port, "client", from)
				return
			}
			from = call.String()
		}
	}
	select {
	case l.conns <- struct{}{}:
		defer func() { <-l.conns }()
	default:
		f.refused.Add(1)
		f.cfg.Logger.Warn("port forwarding: too many connections", "port", r.Port, "client", from)
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	t, err := f.cfg.Dial(dctx, "tcp", r.Target)
	cancel()
	if err != nil {
		f.failed.Add(1)
		f.cfg.Logger.Warn("port forwarding: target not reached", "target", r.Target, "err", err)
		return
	}
	defer t.Close()
	f.connections.Add(1)
	f.cfg.Logger.Info("port forwarding", "client", from, "port", r.Port, "target", r.Target)
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn, n *atomic.Uint64) {
		k, _ := io.Copy(dst, src)
		n.Add(uint64(k))
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		} else {
			dst.Close()
		}
		done <- struct{}{}
	}
	go pipe(t, c, &f.bytesIn)
	go pipe(c, t, &f.bytesOut)
	select {
	case <-done:
		<-done
	case <-ctx.Done():
	}
}

// Ports lists the ports open now.
func (f *Forwarder) Ports() []uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []uint16
	for p := range f.open {
		out = append(out, p)
	}
	return out
}

// Stats returns a snapshot of the counters.
func (f *Forwarder) Stats() Stats {
	return Stats{Connections: f.connections.Load(), Refused: f.refused.Load(), Failed: f.failed.Load(),
		BytesIn: f.bytesIn.Load(), BytesOut: f.bytesOut.Load()}
}

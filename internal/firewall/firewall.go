// SPDX-License-Identifier: GPL-3.0-or-later

// Package firewall decides which packets from the radio reach this computer,
// before the operating system sees them. In the default mode "protect" a
// packet passes if it answers a connection this station started, if it is a
// ping (switchable), or if it goes to a port that a rule opens, optionally
// only for certain callsigns. Everything else is dropped, listed and, unless
// the firewall is silent, answered with a TCP reset or an ICMP message, so
// the other side learns at once that it is refused. Mode "open" lets every
// packet for the own address through. Packets for other addresses never
// pass; the bridge checks that before.
package firewall

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// The modes.
const (
	ModeProtect = "protect"
	ModeOpen    = "open"
)

// Rule opens a port of this computer to the radio network.
type Rule struct {
	Proto string `json:"proto"` // "tcp" or "udp"
	Port  uint16 `json:"port"`
	// Calls are the stations allowed; empty means every station. A callsign
	// without SSID stands for all its SSIDs.
	Calls []string `json:"calls,omitempty"`
	Note  string   `json:"note,omitempty"`
}

// Config is the firewall's setting.
type Config struct {
	Mode string `json:"mode"`
	Ping bool   `json:"ping"`
	// Silent drops refused packets without an answer; by default the sender
	// gets a TCP reset or ICMP "administratively prohibited" (see Refusal).
	Silent bool   `json:"silent,omitempty"`
	Rules  []Rule `json:"rules,omitempty"`
}

// Check reports what is wrong with a configuration.
func (c Config) Check() error {
	if c.Mode != ModeProtect && c.Mode != ModeOpen {
		return fmt.Errorf("firewall: mode %q (protect or open)", c.Mode)
	}
	if len(c.Rules) > 64 {
		return fmt.Errorf("firewall: more than 64 rules")
	}
	for _, r := range c.Rules {
		if err := r.Check(); err != nil {
			return err
		}
	}
	return nil
}

// Check reports what is wrong with a rule.
func (r Rule) Check() error {
	if r.Proto != "tcp" && r.Proto != "udp" {
		return fmt.Errorf("firewall: protocol %q (tcp or udp)", r.Proto)
	}
	if r.Port == 0 {
		return fmt.Errorf("firewall: port 0")
	}
	for _, c := range r.Calls {
		if _, err := ax25.ParseAddress(c); err != nil {
			return fmt.Errorf("firewall: callsign %q: %w", c, err)
		}
	}
	if len(r.Note) > 80 {
		return fmt.Errorf("firewall: note longer than 80 characters")
	}
	return nil
}

// ParseRule reads "8080/tcp" or "22/tcp=CALL,CALL" as given on the command line.
func ParseRule(s string) (Rule, error) {
	spec, calls, _ := strings.Cut(s, "=")
	port, proto, ok := strings.Cut(strings.ToLower(spec), "/")
	if !ok {
		return Rule{}, fmt.Errorf("firewall: want PORT/tcp or PORT/udp, optionally =CALL,CALL")
	}
	var n uint16
	if _, err := fmt.Sscan(port, &n); err != nil {
		return Rule{}, fmt.Errorf("firewall: port %q", port)
	}
	r := Rule{Proto: proto, Port: n}
	if calls != "" {
		for _, c := range strings.Split(calls, ",") {
			r.Calls = append(r.Calls, strings.ToUpper(strings.TrimSpace(c)))
		}
	}
	return r, r.Check()
}

// Blocked is a packet the firewall dropped.
type Blocked struct {
	At    time.Time `json:"at"`
	Call  string    `json:"call"`
	Proto string    `json:"proto"`
	Port  uint16    `json:"port,omitempty"`
}

// Stats counts packets from the radio.
type Stats struct {
	Passed, Blocked uint64
	Refused         uint64 // blocked packets answered (Refusal)
}

type flow struct {
	proto        uint8
	remote       netip.Addr
	rport, lport uint16 // for an ICMP echo: lport is its identifier
}

type fragKey struct {
	src netip.Addr
	id  uint16
}

// Firewall is safe for concurrent use.
type Firewall struct {
	cfg    atomic.Pointer[Config]
	system func() []Rule // ports the program opens itself (welcome page, gateway)
	now    func() time.Time

	mu       sync.Mutex
	flows    map[flow]time.Time // until when an answer may come
	frags    map[fragKey]time.Time
	recent   []Blocked
	answered map[string]time.Time // the last refusal per station
	answers  []time.Time          // the last refusals, at most refuseBurst

	passed, blocked, refused atomic.Uint64
}

// New returns a firewall. system lists the ports the program itself serves;
// it may be nil.
func New(cfg Config, system func() []Rule) *Firewall {
	f := &Firewall{system: system, now: time.Now, flows: map[flow]time.Time{}, frags: map[fragKey]time.Time{},
		answered: map[string]time.Time{}}
	f.Set(cfg)
	return f
}

// Set changes the configuration; it must have passed Check.
func (f *Firewall) Set(cfg Config) { f.cfg.Store(&cfg) }

// Config returns the configuration.
func (f *Firewall) Config() Config { return *f.cfg.Load() }

// Stats returns a snapshot of the counters.
func (f *Firewall) Stats() Stats {
	return Stats{Passed: f.passed.Load(), Blocked: f.blocked.Load(), Refused: f.refused.Load()}
}

// Recent returns the last packets dropped, newest first.
func (f *Firewall) Recent() []Blocked {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Blocked, len(f.recent))
	for i, b := range f.recent {
		out[len(out)-1-i] = b
	}
	return out
}

const (
	tcpIdle  = 30 * time.Minute
	udpIdle  = 3 * time.Minute
	pingIdle = time.Minute
	fragIdle = time.Minute
)

// Outbound learns from a packet this station sends: answers to it may come in.
func (f *Firewall) Outbound(pkt []byte) {
	h, ok := header(pkt)
	if !ok || h.offset != 0 {
		return
	}
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case (h.proto == 6 || h.proto == 17) && len(h.l4) >= 4:
		idle := udpIdle
		if h.proto == 6 {
			idle = tcpIdle
		}
		f.remember(flow{h.proto, h.dst, be16(h.l4[2:]), be16(h.l4[0:])}, now.Add(idle), now)
	case h.proto == 1 && len(h.l4) >= 8 && h.l4[0] == 8: // echo request
		f.remember(flow{1, h.dst, 0, be16(h.l4[4:])}, now.Add(pingIdle), now)
	}
}

func (f *Firewall) remember(k flow, until, now time.Time) {
	if len(f.flows) >= 4096 {
		for k, t := range f.flows {
			if now.After(t) {
				delete(f.flows, k)
			}
		}
	}
	f.flows[k] = until
}

// Inbound decides about a packet from the radio for this station's address,
// sent by the station from.
func (f *Firewall) Inbound(pkt []byte, from ax25.Address) bool {
	cfg := f.cfg.Load()
	if cfg.Mode == ModeOpen {
		f.passed.Add(1)
		return true
	}
	h, ok := header(pkt)
	if !ok {
		return f.drop(from, "ip", 0)
	}
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	if h.offset != 0 { // a later fragment: passes if its first one did
		if t, ok := f.frags[fragKey{h.src, h.id}]; ok && !now.After(t) {
			f.passed.Add(1)
			return true
		}
		return f.dropLocked(from, protoName(h.proto), 0)
	}
	pass, port := false, uint16(0)
	switch h.proto {
	case 6, 17:
		if len(h.l4) < 4 {
			break
		}
		port = be16(h.l4[2:])
		k := flow{h.proto, h.src, be16(h.l4[0:]), port}
		if t, ok := f.flows[k]; ok && !now.After(t) {
			idle := udpIdle
			if h.proto == 6 {
				idle = tcpIdle
			}
			f.flows[k] = now.Add(idle)
			pass = true
		} else {
			pass = f.opened(cfg, protoName(h.proto), port, from)
		}
	case 1:
		if len(h.l4) < 8 {
			break
		}
		switch h.l4[0] {
		case 0: // echo reply to our ping
			t, ok := f.flows[flow{1, h.src, 0, be16(h.l4[4:])}]
			pass = ok && !now.After(t)
		case 8:
			pass = cfg.Ping
		case 3, 4, 11, 12: // unreachable, quench, time exceeded, parameter problem
			pass = true
		}
	}
	if !pass {
		return f.dropLocked(from, protoName(h.proto), port)
	}
	if h.more { // a first fragment: its later fragments may follow
		if len(f.frags) >= 1024 {
			for k, t := range f.frags {
				if now.After(t) {
					delete(f.frags, k)
				}
			}
		}
		f.frags[fragKey{h.src, h.id}] = now.Add(fragIdle)
	}
	f.passed.Add(1)
	return true
}

// opened reports whether a rule, or a port the program serves, admits the
// packet.
func (f *Firewall) opened(cfg *Config, proto string, port uint16, from ax25.Address) bool {
	rules := cfg.Rules
	if f.system != nil {
		rules = append(append([]Rule(nil), rules...), f.system()...)
	}
	for _, r := range rules {
		if r.Proto == proto && r.Port == port && Admits(r.Calls, from) {
			return true
		}
	}
	return false
}

// Admits reports whether from is one of calls; an empty list admits everyone.
// A callsign without SSID stands for all its SSIDs.
func Admits(calls []string, from ax25.Address) bool {
	if len(calls) == 0 {
		return true
	}
	for _, c := range calls {
		a, err := ax25.ParseAddress(c)
		if err != nil {
			continue
		}
		if a.Call == from.Call && (a.SSID == from.SSID || !strings.Contains(c, "-")) {
			return true
		}
	}
	return false
}

func (f *Firewall) drop(from ax25.Address, proto string, port uint16) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dropLocked(from, proto, port)
}

func (f *Firewall) dropLocked(from ax25.Address, proto string, port uint16) bool {
	f.blocked.Add(1)
	b := Blocked{At: f.now(), Call: from.String(), Proto: proto, Port: port}
	if len(f.recent) == 8 {
		f.recent = append(f.recent[:0], f.recent[1:]...)
	}
	f.recent = append(f.recent, b)
	return false
}

type ipv4 struct {
	proto    uint8
	src, dst netip.Addr
	id       uint16
	offset   uint16
	more     bool
	l4       []byte // the transport header and data (first fragment only)
}

func header(p []byte) (ipv4, bool) {
	if len(p) < 20 || p[0]>>4 != 4 {
		return ipv4{}, false
	}
	ihl := int(p[0]&0x0F) * 4
	total := int(be16(p[2:]))
	if ihl < 20 || total < ihl || total > len(p) {
		return ipv4{}, false
	}
	frag := be16(p[6:])
	return ipv4{
		proto: p[9], src: netip.AddrFrom4([4]byte(p[12:16])), dst: netip.AddrFrom4([4]byte(p[16:20])),
		id: be16(p[4:]), offset: frag & 0x1FFF, more: frag&0x2000 != 0, l4: p[ihl:total],
	}, true
}

func protoName(p uint8) string {
	switch p {
	case 1:
		return "icmp"
	case 6:
		return "tcp"
	case 17:
		return "udp"
	}
	return fmt.Sprintf("ip%d", p)
}

func be16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }

// SPDX-License-Identifier: GPL-3.0-or-later

// Package gateway shares a station's internet connection with the radio
// network as an HTTP proxy. Clients send it requests with full URLs, and
// CONNECT requests for HTTPS; they use it only if they choose to (their proxy
// setting), so no default route over the radio is needed. The gateway
// resolves names itself, which keeps DNS off the air. By default it does not
// connect to private, loopback or link-local addresses: radio users get the
// internet, not the operator's own network.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/firewall"
)

// Config sets up a proxy.
type Config struct {
	// AllowLAN lets clients reach private (RFC 1918, fc00::/7), shared
	// (100.64.0.0/10), loopback and link-local addresses.
	AllowLAN bool
	// Who names the station behind a client address (station discovery
	// provides it); nil for none.
	Who func(addr netip.Addr) (ax25.Address, bool)
	// Only lists the stations that may use the gateway; empty: every station.
	// A callsign without SSID stands for all its SSIDs.
	Only     []string
	MaxConns int           // client connections at a time; 0 means 16
	Idle     time.Duration // a connection silent this long is closed; 0 means 10 min
	Logger   *slog.Logger
}

// Stats counts what the proxy did.
type Stats struct {
	Requests, Tunnels   uint64 // HTTP requests forwarded; CONNECT tunnels opened
	Refused, Failed     uint64 // destinations not allowed; upstream errors
	BytesIn, BytesOut   uint64 // from the internet to the radio; the other way
	Rejected, Overloads uint64 // not a proxy request; MaxConns reached
}

// Proxy is an HTTP proxy for the radio network.
type Proxy struct {
	cfg       Config
	access    atomic.Pointer[access] // AllowLAN and Only, changeable while running
	dialer    *net.Dialer
	transport *http.Transport

	requests, tunnels, refused, failed atomic.Uint64
	bytesIn, bytesOut                  atomic.Uint64
	rejected, overloads                atomic.Uint64
}

var errRefused = errors.New("gateway: destination not allowed")

type access struct {
	allowLAN bool
	only     []string
}

// SetAccess changes who may use the gateway and whether it reaches the own
// network, without a restart.
func (p *Proxy) SetAccess(allowLAN bool, only []string) {
	p.access.Store(&access{allowLAN: allowLAN, only: append([]string(nil), only...)})
}

// New returns a proxy; start it with Serve.
func New(cfg Config) *Proxy {
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = 16
	}
	if cfg.Idle <= 0 {
		cfg.Idle = 10 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	p := &Proxy{cfg: cfg}
	p.SetAccess(cfg.AllowLAN, cfg.Only)
	// The check runs on every address a name resolves to, right before the
	// connection, so a name cannot lead into the local network either.
	p.dialer = &net.Dialer{Timeout: 30 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil {
			return err
		}
		if !p.Allowed(ap.Addr()) {
			return errRefused
		}
		return nil
	}}
	p.transport = &http.Transport{
		Proxy:                 nil, // straight to the internet
		DialContext:           p.dialer.DialContext,
		DisableCompression:    true, // pass bodies through as they are
		ResponseHeaderTimeout: 2 * time.Minute,
		IdleConnTimeout:       time.Minute,
		MaxIdleConns:          8,
	}
	return p
}

// Allowed reports whether the proxy may connect to addr.
func (p *Proxy) Allowed(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() ||
		addr == netip.AddrFrom4([4]byte{255, 255, 255, 255}) || thisNetwork.Contains(addr) {
		return false
	}
	if p.access.Load().allowLAN {
		return true
	}
	return addr.IsGlobalUnicast() && !addr.IsPrivate() && !shared.Contains(addr)
}

var (
	thisNetwork = netip.MustParsePrefix("0.0.0.0/8")
	shared      = netip.MustParsePrefix("100.64.0.0/10") // carrier-grade NAT
)

// Stats returns a snapshot of the counters.
func (p *Proxy) Stats() Stats {
	return Stats{
		Requests: p.requests.Load(), Tunnels: p.tunnels.Load(),
		Refused: p.refused.Load(), Failed: p.failed.Load(),
		BytesIn: p.bytesIn.Load(), BytesOut: p.bytesOut.Load(),
		Rejected: p.rejected.Load(), Overloads: p.overloads.Load(),
	}
}

// Serve answers proxy requests on ln until ctx ends.
func (p *Proxy) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           p,
		ReadHeaderTimeout: 2 * time.Minute, // a slow radio needs time for the headers
		IdleTimeout:       p.cfg.Idle,
		ErrorLog:          slog.NewLogLogger(p.cfg.Logger.Handler(), slog.LevelDebug),
	}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	err := srv.Serve(&limitListener{Listener: ln, slots: make(chan struct{}, p.cfg.MaxConns), p: p})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// ServeHTTP handles one proxy request.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	client, call, known := p.who(r.RemoteAddr)
	if only := p.access.Load().only; len(only) > 0 && (!known || !firewall.Admits(only, call)) {
		p.refused.Add(1)
		http.Error(w, "AXLink gateway: open only to "+strings.Join(only, ", ")+".", http.StatusForbidden)
		p.cfg.Logger.Info("gateway", "client", client, "method", r.Method, "status", http.StatusForbidden, "err", "not admitted")
		return
	}
	if r.Method == http.MethodConnect {
		p.tunnel(w, r, client)
		return
	}
	if !r.URL.IsAbs() || r.URL.Host == "" || r.URL.Scheme != "http" && r.URL.Scheme != "https" {
		p.rejected.Add(1)
		http.Error(w, "AXLink gateway: this is an HTTP proxy. Set it as your proxy "+
			"(e.g. curl -x http://"+r.Host+"); HTTPS goes through CONNECT.", http.StatusBadRequest)
		return
	}
	start := time.Now()
	out := r.Clone(r.Context())
	out.RequestURI = ""
	removeHopHeaders(out.Header)
	countBody(out, &p.bytesOut)
	resp, err := p.transport.RoundTrip(out)
	if err != nil {
		code := p.upstreamError(err)
		http.Error(w, "AXLink gateway: "+err.Error(), code)
		p.cfg.Logger.Info("gateway", "client", client, "method", r.Method, "url", r.URL.String(), "status", code, "err", err)
		return
	}
	defer resp.Body.Close()
	removeHopHeaders(resp.Header)
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	p.requests.Add(1)
	body := &counter{r: resp.Body, total: &p.bytesIn}
	_, _ = io.Copy(w, body)
	p.cfg.Logger.Info("gateway", "client", client, "method", r.Method, "url", r.URL.String(),
		"status", resp.StatusCode, "bytes", body.n, "took", time.Since(start).Round(100*time.Millisecond))
}

// tunnel opens a CONNECT tunnel, normally for HTTPS.
func (p *Proxy) tunnel(w http.ResponseWriter, r *http.Request, client string) {
	start := time.Now()
	up, err := p.dialer.DialContext(r.Context(), "tcp", r.Host)
	if err != nil {
		code := p.upstreamError(err)
		http.Error(w, "AXLink gateway: "+err.Error(), code)
		p.cfg.Logger.Info("gateway", "client", client, "method", "CONNECT", "host", r.Host, "status", code, "err", err)
		return
	}
	defer up.Close()
	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		p.failed.Add(1)
		return
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}
	p.tunnels.Add(1)
	_ = conn.SetDeadline(time.Time{}) // the server's header timeout no longer applies
	var wg sync.WaitGroup
	sent := &counter{r: &idleReader{conn: conn, r: rw.Reader, idle: p.cfg.Idle}, total: &p.bytesOut}
	got := &counter{r: &idleReader{conn: up, r: up, idle: p.cfg.Idle}, total: &p.bytesIn}
	wg.Add(1)
	go func() { // radio → internet, starting with what the client sent early
		defer wg.Done()
		_, _ = io.Copy(up, sent)
		if tc, ok := up.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	_, _ = io.Copy(conn, got)
	_ = conn.Close() // ends the other direction too
	wg.Wait()
	p.cfg.Logger.Info("gateway", "client", client, "method", "CONNECT", "host", r.Host, "status", 200,
		"bytes", got.n, "sent", sent.n, "took", time.Since(start).Round(100*time.Millisecond))
}

func (p *Proxy) upstreamError(err error) int {
	if errors.Is(err, errRefused) {
		p.refused.Add(1)
		return http.StatusForbidden
	}
	p.failed.Add(1)
	return http.StatusBadGateway
}

// who names the client: its callsign if known, else its address.
func (p *Proxy) who(remote string) (string, ax25.Address, bool) {
	ap, err := netip.ParseAddrPort(remote)
	if err != nil {
		return remote, ax25.Address{}, false
	}
	if p.cfg.Who != nil {
		if call, ok := p.cfg.Who(ap.Addr()); ok {
			return call.String(), call, true
		}
	}
	return ap.Addr().String(), ax25.Address{}, false
}

// Hop-by-hop headers belong to one connection, not to the request (RFC 9110).
// Headers named in Connection are hop-by-hop too; Go's client drops a whole
// "Connection: close" header before the proxy sees it, so those few cannot
// be known.
var hopHeaders = []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

func removeHopHeaders(h http.Header) {
	for _, f := range h.Values("Connection") {
		for _, name := range strings.Split(f, ",") {
			if name = strings.TrimSpace(name); name != "" {
				h.Del(name)
			}
		}
	}
	for _, name := range hopHeaders {
		h.Del(name)
	}
}

// counter adds what passes through to a running total, so that the numbers
// grow while a long transfer over the radio is still under way.
type counter struct {
	r     io.Reader
	total *atomic.Uint64
	n     int64
}

func (c *counter) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	c.total.Add(uint64(n))
	return n, err
}

// countBody counts the bytes of a request body as they are sent.
func countBody(r *http.Request, total *atomic.Uint64) {
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = struct {
			io.Reader
			io.Closer
		}{&counter{r: r.Body, total: total}, r.Body}
	}
}

// idleReader closes nothing itself; it gives every read a deadline, so that
// a tunnel nobody uses any more ends after the idle time.
type idleReader struct {
	conn net.Conn
	r    io.Reader
	idle time.Duration
}

func (ir *idleReader) Read(b []byte) (int, error) {
	_ = ir.conn.SetReadDeadline(time.Now().Add(ir.idle))
	return ir.r.Read(b)
}

// limitListener accepts at most cap(slots) connections at a time.
type limitListener struct {
	net.Listener
	slots chan struct{}
	p     *Proxy
}

func (l *limitListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			return &slotConn{Conn: c, release: func() { <-l.slots }}, nil
		default:
			l.p.overloads.Add(1)
			fmt.Fprint(c, "HTTP/1.1 503 Service Unavailable\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			c.Close()
		}
	}
}

type slotConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *slotConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}

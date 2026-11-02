// SPDX-License-Identifier: GPL-3.0-or-later

package gateway

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// LocalConfig sets up a Local proxy.
type LocalConfig struct {
	// Upstream returns the chosen gateway's proxy and its callsign; ok is
	// false while none is chosen or the chosen one is not heard.
	Upstream func() (proxy netip.AddrPort, call string, ok bool)
	// Hint tells the user where to choose a gateway, e.g. the web UI's URL.
	Hint   string
	Idle   time.Duration // a tunnel silent this long is closed; 0 means 10 min
	Logger *slog.Logger
}

// LocalStats counts what the local proxy did.
type LocalStats struct {
	Requests, Tunnels, NoGateway, Failed uint64
	BytesIn, BytesOut                    uint64 // from the gateway; to the gateway
}

// Local is the client side of the internet gateway: a proxy on the own
// computer that hands every request to the chosen gateway's proxy over the
// radio. Browser and programs are set up once with the local proxy; the
// gateway is chosen in the web UI. Connections to the gateway are reused,
// which saves the TCP handshake over the radio.
type Local struct {
	cfg       LocalConfig
	transport *http.Transport

	requests, tunnels, noGateway, failed, bytesIn, bytesOut atomic.Uint64
}

type upstreamKey struct{}

// NewLocal returns a local proxy; start it with Serve.
func NewLocal(cfg LocalConfig) *Local {
	if cfg.Idle <= 0 {
		cfg.Idle = 10 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	l := &Local{cfg: cfg}
	l.transport = &http.Transport{
		Proxy: func(r *http.Request) (*url.URL, error) {
			ap, _ := r.Context().Value(upstreamKey{}).(netip.AddrPort)
			return &url.URL{Scheme: "http", Host: ap.String()}, nil
		},
		DialContext:           (&net.Dialer{Timeout: time.Minute}).DialContext, // a SYN over the radio may need repeats
		DisableCompression:    true,
		ResponseHeaderTimeout: 3 * time.Minute,
		IdleConnTimeout:       2 * time.Minute,
		MaxIdleConnsPerHost:   2,
	}
	return l
}

// Stats returns a snapshot of the counters.
func (l *Local) Stats() LocalStats {
	return LocalStats{Requests: l.requests.Load(), Tunnels: l.tunnels.Load(), NoGateway: l.noGateway.Load(),
		Failed: l.failed.Load(), BytesIn: l.bytesIn.Load(), BytesOut: l.bytesOut.Load()}
}

// Serve answers proxy requests on ln until ctx ends.
func (l *Local) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: l, ReadHeaderTimeout: time.Minute, IdleTimeout: l.cfg.Idle,
		ErrorLog: slog.NewLogLogger(l.cfg.Logger.Handler(), slog.LevelDebug)}
	go func() {
		<-ctx.Done()
		srv.Close()
		l.transport.CloseIdleConnections()
	}()
	err := srv.Serve(ln)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// ServeHTTP handles one request from a local program.
func (l *Local) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect && (!r.URL.IsAbs() || r.URL.Host == "") {
		http.Error(w, "AXLink: this is the local proxy to the internet gateway. Set it as the HTTP "+
			"and HTTPS proxy of your browser or program. "+l.cfg.Hint, http.StatusBadRequest)
		return
	}
	up, call, ok := l.cfg.Upstream()
	if !ok {
		l.noGateway.Add(1)
		http.Error(w, "AXLink: no internet gateway chosen or heard. "+l.cfg.Hint, http.StatusServiceUnavailable)
		return
	}
	if r.Method == http.MethodConnect {
		l.tunnel(w, r, up, call)
		return
	}
	out := r.Clone(context.WithValue(r.Context(), upstreamKey{}, up))
	out.RequestURI = ""
	removeHopHeaders(out.Header)
	countBody(out, &l.bytesOut)
	resp, err := l.transport.RoundTrip(out)
	if err != nil {
		l.failed.Add(1)
		http.Error(w, fmt.Sprintf("AXLink: gateway %s (%s): %v", call, up, err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	removeHopHeaders(resp.Header)
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	l.requests.Add(1)
	_, _ = io.Copy(w, &counter{r: resp.Body, total: &l.bytesIn})
}

// tunnel asks the gateway for a CONNECT tunnel and joins it to the program's
// connection.
func (l *Local) tunnel(w http.ResponseWriter, r *http.Request, up netip.AddrPort, call string) {
	d := net.Dialer{Timeout: time.Minute}
	uc, err := d.DialContext(r.Context(), "tcp", up.String())
	if err != nil {
		l.failed.Add(1)
		http.Error(w, fmt.Sprintf("AXLink: gateway %s (%s): %v", call, up, err), http.StatusBadGateway)
		return
	}
	defer uc.Close()
	if _, err := fmt.Fprintf(uc, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", r.Host, r.Host); err != nil {
		l.failed.Add(1)
		http.Error(w, "AXLink: "+err.Error(), http.StatusBadGateway)
		return
	}
	ur := bufio.NewReader(uc)
	_ = uc.SetReadDeadline(time.Now().Add(3 * time.Minute))
	resp, err := http.ReadResponse(ur, r)
	if err != nil {
		l.failed.Add(1)
		http.Error(w, fmt.Sprintf("AXLink: gateway %s: %v", call, err), http.StatusBadGateway)
		return
	}
	if resp.StatusCode != http.StatusOK { // the gateway's answer, e.g. 403, goes to the program
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}
	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		l.failed.Add(1)
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Time{})
	_ = uc.SetReadDeadline(time.Time{})
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}
	l.tunnels.Add(1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(uc, &counter{r: &idleReader{conn: conn, r: rw.Reader, idle: l.cfg.Idle}, total: &l.bytesOut})
		if tc, ok := uc.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	_, _ = io.Copy(conn, &counter{r: &idleReader{conn: uc, r: ur, idle: l.cfg.Idle}, total: &l.bytesIn})
	_ = conn.Close()
	wg.Wait()
}

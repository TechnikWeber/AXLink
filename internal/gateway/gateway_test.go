// SPDX-License-Identifier: GPL-3.0-or-later

package gateway

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// startProxy runs a proxy on a loopback port. The tests' upstream servers
// are on loopback too, so most tests allow the LAN.
func startProxy(t *testing.T, cfg Config) (*Proxy, string, *bytes.Buffer) {
	t.Helper()
	var log bytes.Buffer
	cfg.Logger = slog.New(slog.NewTextHandler(&log, nil))
	p := New(cfg)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return p, ln.Addr().String(), &log
}

func viaProxy(proxyAddr string) *http.Client {
	u, _ := url.Parse("http://" + proxyAddr)
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}, Timeout: 5 * time.Second}
}

func TestProxyForwardsHTTPWithoutHopHeaders(t *testing.T) {
	var seen http.Header
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.Header().Set("Connection", "X-Private")
		w.Header().Set("X-Private", "secret")
		io.WriteString(w, "hello from the internet")
	}))
	defer up.Close()
	p, addr, log := startProxy(t, Config{AllowLAN: true,
		Who: func(netip.Addr) (ax25.Address, bool) { return ax25.MustAddress("TESTA-1"), true }})

	req, _ := http.NewRequest("GET", up.URL+"/page", nil)
	req.Header.Set("Proxy-Connection", "keep-alive")
	resp, err := viaProxy(addr).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "hello from the internet" {
		t.Fatalf("%d %q", resp.StatusCode, body)
	}
	if seen.Get("Proxy-Connection") != "" {
		t.Fatal("a hop-by-hop header reached the server")
	}
	if resp.Header.Get("X-Private") != "" {
		t.Fatal("a header named in Connection reached the client")
	}
	if s := p.Stats(); s.Requests != 1 || s.BytesIn != uint64(len(body)) {
		t.Fatalf("stats %+v", s)
	}
	if !strings.Contains(log.String(), "client=TESTA-1") {
		t.Fatalf("log %s", log.String())
	}
}

func TestProxyTunnelsCONNECT(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		c, err := echo.Accept()
		if err == nil {
			io.Copy(c, c)
			c.Close()
		}
	}()
	p, addr, _ := startProxy(t, Config{AllowLAN: true})

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	// The client may send data right after the request, before the answer.
	io.WriteString(c, "CONNECT "+echo.Addr().String()+" HTTP/1.1\r\nHost: "+echo.Addr().String()+"\r\n\r\nping")
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(br, got); err != nil || string(got) != "ping" {
		t.Fatalf("%q %v", got, err)
	}
	c.Close()
	deadline := time.Now().Add(2 * time.Second)
	for p.Stats().Tunnels != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s := p.Stats(); s.Tunnels != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func TestProxyKeepsTheLocalNetworkOutByDefault(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the proxy reached a loopback server")
	}))
	defer up.Close()
	p, addr, _ := startProxy(t, Config{})
	resp, err := viaProxy(addr).Get(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d", resp.StatusCode)
	}

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	io.WriteString(c, "CONNECT 192.168.1.1:80 HTTP/1.1\r\nHost: 192.168.1.1:80\r\n\r\n")
	r, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil || r.StatusCode != http.StatusForbidden {
		t.Fatalf("CONNECT into the LAN: %v %v", r, err)
	}
	if s := p.Stats(); s.Refused != 2 {
		t.Fatalf("stats %+v", s)
	}
}

func TestProxyAnswersOtherRequestsWithAHint(t *testing.T) {
	p, addr, _ := startProxy(t, Config{})
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "HTTP proxy") || p.Stats().Rejected != 1 {
		t.Fatalf("%d %q", resp.StatusCode, body)
	}
}

func TestProxyRefusesConnectionsBeyondMaxConns(t *testing.T) {
	p, addr, _ := startProxy(t, Config{MaxConns: 1})
	first, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.SetDeadline(time.Now().Add(5 * time.Second))
	r, err := http.ReadResponse(bufio.NewReader(second), nil)
	if err != nil || r.StatusCode != http.StatusServiceUnavailable || p.Stats().Overloads != 1 {
		t.Fatalf("%v %v %+v", r, err, p.Stats())
	}
}

func TestAllowed(t *testing.T) {
	strict, lan := New(Config{}), New(Config{AllowLAN: true})
	for _, tc := range []struct {
		addr          string
		strict, inLAN bool
	}{
		{"192.0.2.80", true, true},
		{"93.184.215.14", true, true},
		{"2001:db8::1", true, true},
		{"10.0.0.1", false, true},
		{"172.16.5.4", false, true},
		{"192.168.1.1", false, true},
		{"100.64.0.1", false, true},
		{"127.0.0.1", false, true},
		{"169.254.1.1", false, true},
		{"fd00::1", false, true},
		{"::ffff:192.168.1.1", false, true},
		{"0.0.0.0", false, false},
		{"0.1.2.3", false, false},
		{"224.0.0.251", false, false},
		{"255.255.255.255", false, false},
	} {
		a := netip.MustParseAddr(tc.addr)
		if strict.Allowed(a) != tc.strict || lan.Allowed(a) != tc.inLAN {
			t.Errorf("%s: strict %v lan %v, want %v %v", tc.addr, strict.Allowed(a), lan.Allowed(a), tc.strict, tc.inLAN)
		}
	}
}

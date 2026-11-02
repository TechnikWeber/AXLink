// SPDX-License-Identifier: GPL-3.0-or-later

package gateway

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// startLocal runs a local proxy in front of the gateway at gw ("" for none).
func startLocal(t *testing.T, gw string) (*Local, string) {
	t.Helper()
	l := NewLocal(LocalConfig{
		Upstream: func() (netip.AddrPort, string, bool) {
			if gw == "" {
				return netip.AddrPort{}, "", false
			}
			return netip.MustParseAddrPort(gw), "TESTB-1", true
		},
		Hint:   "Choose one at http://127.0.0.1:7373/.",
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { l.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return l, ln.Addr().String()
}

func TestLocalForwardsThroughTheGateway(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "page "+r.URL.Path)
	}))
	defer up.Close()
	_, gw, _ := startProxy(t, Config{AllowLAN: true})
	l, local := startLocal(t, gw)
	for i := 0; i < 2; i++ { // the second request reuses the connection to the gateway
		resp, err := viaProxy(local).Get(up.URL + "/x")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(body) != "page /x" {
			t.Fatalf("%d %q", resp.StatusCode, body)
		}
	}
	if s := l.Stats(); s.Requests != 2 || s.BytesIn != 14 {
		t.Fatalf("stats %+v", s)
	}
}

func TestLocalTunnelsThroughTheGateway(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		if c, err := echo.Accept(); err == nil {
			io.Copy(c, c)
			c.Close()
		}
	}()
	_, gw, _ := startProxy(t, Config{AllowLAN: true})
	l, local := startLocal(t, gw)
	c, err := net.Dial("tcp", local)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(c, "CONNECT "+echo.Addr().String()+" HTTP/1.1\r\nHost: "+echo.Addr().String()+"\r\n\r\n")
	br := bufio.NewReader(c)
	if resp, err := http.ReadResponse(br, nil); err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
	io.WriteString(c, "hello")
	got := make([]byte, 5)
	if _, err := io.ReadFull(br, got); err != nil || string(got) != "hello" {
		t.Fatalf("%q %v", got, err)
	}
	c.Close()
	deadline := time.Now().Add(2 * time.Second)
	for l.Stats().Tunnels != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if l.Stats().Tunnels != 1 {
		t.Fatalf("stats %+v", l.Stats())
	}
}

func TestLocalWithoutAGatewayExplains(t *testing.T) {
	l, local := startLocal(t, "")
	resp, err := viaProxy(local).Get("http://192.0.2.80/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "127.0.0.1:7373") || l.Stats().NoGateway != 1 {
		t.Fatalf("%d %q", resp.StatusCode, body)
	}
}

func TestLocalPassesTheGatewaysRefusalOn(t *testing.T) {
	_, gw, _ := startProxy(t, Config{}) // keeps the local network out
	_, local := startLocal(t, gw)
	c, err := net.Dial("tcp", local)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(c, "CONNECT 127.0.0.1:22 HTTP/1.1\r\nHost: 127.0.0.1:22\r\n\r\n")
	if resp, err := http.ReadResponse(bufio.NewReader(c), nil); err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("%v %v", resp, err)
	}
}

func TestChoice(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cands := []Candidate{
		{Call: "TESTB-1", Proxy: netip.MustParseAddrPort("10.73.0.2:8080"), Heard: now},
		{Call: "TESTC-1", Proxy: netip.MustParseAddrPort("10.73.0.3:8080"), Heard: now.Add(time.Minute)},
	}
	c := NewChoice(func() []Candidate { return cands })
	if _, _, ok := c.Upstream(); ok {
		t.Fatal("off must give no gateway")
	}
	if err := c.Set(ModeStation, "testb-1"); err != nil {
		t.Fatal(err)
	}
	if ap, call, ok := c.Upstream(); !ok || call != "TESTB-1" || ap.String() != "10.73.0.2:8080" {
		t.Fatalf("%v %v %v", ap, call, ok)
	}
	if mode, call := c.Get(); mode != ModeStation || call != "TESTB-1" {
		t.Fatalf("%v %v", mode, call)
	}
	c.Set(ModeAuto, "")
	if _, call, _ := c.Upstream(); call != "TESTC-1" {
		t.Fatalf("auto took %s, want the one heard last", call)
	}
	cands[0].Heard = now.Add(2 * time.Minute) // B is heard again: auto stays with C
	if _, call, _ := c.Upstream(); call != "TESTC-1" {
		t.Fatalf("auto switched to %s while C is heard", call)
	}
	cands = cands[:1] // C is gone
	if _, call, _ := c.Upstream(); call != "TESTB-1" {
		t.Fatalf("auto took %s", call)
	}
	c.Set(ModeStation, "TESTC-1")
	if _, _, ok := c.Upstream(); ok {
		t.Fatal("a station that is not heard must give no gateway")
	}
	if c.Set(ModeStation, "") == nil || c.Set("sometimes", "") == nil {
		t.Fatal("invalid choices accepted")
	}
}

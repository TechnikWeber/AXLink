// SPDX-License-Identifier: GPL-3.0-or-later

package forward

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

func TestParseRule(t *testing.T) {
	r, err := ParseRule("2222=192.168.1.10:22=n0call-1,N0CALL")
	if err != nil || r.Port != 2222 || r.Target != "192.168.1.10:22" || len(r.Calls) != 2 || r.Calls[0] != "N0CALL-1" {
		t.Fatalf("%+v %v", r, err)
	}
	if r, err := ParseRule("8080=nas.local:80"); err != nil || r.Calls != nil {
		t.Fatalf("%+v %v", r, err)
	}
	for _, bad := range []string{"2222", "0=1.2.3.4:22", "2222=1.2.3.4", "2222=:22", "2222=1.2.3.4:0", "2222=1.2.3.4:22=not a call",
		"70000=1.2.3.4:22"} {
		if _, err := ParseRule(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// echoServer answers every line with the same line; it stands for the
// service in the own network.
func echoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	return ln.Addr().String()
}

// waitFor polls cond for up to 2 s: the counters follow the connection.
func waitFor(cond func() bool) {
	for i := 0; i < 200 && !cond(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
}

func freePort(t *testing.T) uint16 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	n, _ := strconv.Atoi(p)
	return uint16(n)
}

func roundTrip(t *testing.T, port uint16) (string, error) {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(int(port)), 2*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte("hello")); err != nil {
		return "", err
	}
	c.(*net.TCPConn).CloseWrite()
	b, err := io.ReadAll(c)
	return string(b), err
}

func TestAConnectionReachesTheTarget(t *testing.T) {
	target := echoServer(t)
	f := New(Config{Addr: netip.MustParseAddr("127.0.0.1"), Logger: slog.New(slog.DiscardHandler)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port := freePort(t)
	if err := f.Set(ctx, []Rule{{Port: port, Target: target}}); err != nil {
		t.Fatal(err)
	}
	if got, err := roundTrip(t, port); err != nil || got != "hello" {
		t.Fatalf("%q %v", got, err)
	}
	waitFor(func() bool { s := f.Stats(); return s.BytesIn == 5 && s.BytesOut == 5 })
	if s := f.Stats(); s.Connections != 1 || s.BytesIn != 5 || s.BytesOut != 5 {
		t.Fatalf("%+v", s)
	}
	// The rule goes: the port closes.
	f.Set(ctx, nil)
	time.Sleep(50 * time.Millisecond)
	if _, err := roundTrip(t, port); err == nil {
		t.Fatal("the port is still open")
	}
}

func TestOnlyTheListedStations(t *testing.T) {
	target := echoServer(t)
	var mu sync.Mutex
	known := map[netip.Addr]ax25.Address{}
	learn := func(call string) {
		mu.Lock()
		defer mu.Unlock()
		known[netip.MustParseAddr("127.0.0.1")] = ax25.MustAddress(call)
	}
	f := New(Config{Addr: netip.MustParseAddr("127.0.0.1"), Logger: slog.New(slog.DiscardHandler),
		Who: func(a netip.Addr) (ax25.Address, bool) {
			mu.Lock()
			defer mu.Unlock()
			c, ok := known[a]
			return c, ok
		}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port := freePort(t)
	f.Set(ctx, []Rule{{Port: port, Target: target, Calls: []string{"TESTA"}}})
	if got, _ := roundTrip(t, port); got != "" {
		t.Fatalf("an unknown station got %q", got)
	}
	learn("TESTB-1")
	if got, _ := roundTrip(t, port); got != "" {
		t.Fatalf("another station got %q", got)
	}
	learn("TESTA-7")
	if got, err := roundTrip(t, port); got != "hello" {
		t.Fatalf("TESTA-7 (TESTA stands for all its SSIDs): %q %v", got, err)
	}
	if s := f.Stats(); s.Refused != 2 || s.Connections != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestATargetThatDoesNotAnswer(t *testing.T) {
	f := New(Config{Addr: netip.MustParseAddr("127.0.0.1"), Logger: slog.New(slog.DiscardHandler)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port, closed := freePort(t), freePort(t)
	f.Set(ctx, []Rule{{Port: port, Target: "127.0.0.1:" + strconv.Itoa(int(closed))}})
	if got, _ := roundTrip(t, port); got != "" {
		t.Fatalf("%q", got)
	}
	waitFor(func() bool { return f.Stats().Failed == 1 })
	if s := f.Stats(); s.Failed != 1 {
		t.Fatalf("%+v", s)
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package channel

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/kiss"
	"github.com/TechnikWeber/AXLink/internal/pacing"
)

func listen(t *testing.T, name string) (net.Listener, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	return ln, path
}

func readFrame(t *testing.T, conn net.Conn) kiss.Frame {
	t.Helper()
	d := &kiss.Decoder{}
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var got []kiss.Frame
	for len(got) == 0 {
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		d.Feed(buf[:n], func(f kiss.Frame) { got = append(got, f) })
	}
	return got[0]
}

func run(t *testing.T, mode Mode) (a, b net.Conn) {
	t.Helper()
	lnA, pathA := listen(t, "a")
	lnB, pathB := listen(t, "b")
	c := New(Config{Mode: mode})
	c.AddStation("a", lnA)
	c.AddStation("b", lnB)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go c.Run(ctx)
	a, err := net.Dial("unix", pathA)
	if err != nil {
		t.Fatal(err)
	}
	b, err = net.Dial("unix", pathB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close(); b.Close() })
	time.Sleep(50 * time.Millisecond) // let the channel accept both
	return a, b
}

func TestIdealChannelDeliversToTheOtherStation(t *testing.T) {
	a, b := run(t, Ideal)
	frame, _ := ax25.NewUI(ax25.MustAddress("TESTB-1"), ax25.MustAddress("TESTA-1"), nil, ax25.PIDNoL3, []byte("hi")).Encode()
	enc, _ := kiss.Encode(kiss.Data(0, frame))
	start := time.Now()
	a.Write(enc)
	got := readFrame(t, b)
	if got.Command != kiss.CmdData || !bytes.Equal(got.Payload, frame) {
		t.Fatalf("got %+v", got)
	}
	if d := time.Since(start); d < 300*time.Millisecond {
		t.Fatalf("delivered after %v, faster than the airtime", d)
	}
}

func TestF4HWNChannelNeedsPacing(t *testing.T) {
	a, b := run(t, F4HWN)
	info := bytes.Repeat([]byte{0x55}, 256)
	frame, _ := ax25.NewUI(ax25.MustAddress("TESTB-1"), ax25.MustAddress("TESTA-1"), nil, ax25.PIDIPv4, info).Encode()
	enc, _ := kiss.Encode(kiss.Data(0, frame))

	// Paced like the f4hwn profile: it arrives.
	p := pacing.F4HWNKissTNC
	plan := p.Plan(0, enc, frame)
	start := time.Now()
	for _, w := range plan.Writes {
		time.Sleep(time.Until(start.Add(w.At)))
		a.Write(w.Data)
	}
	got := readFrame(t, b)
	if !bytes.Equal(got.Payload, frame) {
		t.Fatal("paced frame changed")
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package tnc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/kiss"
	"github.com/TechnikWeber/AXLink/internal/pacing"
	"github.com/TechnikWeber/AXLink/internal/sim/f4hwnkiss"
	"github.com/TechnikWeber/AXLink/internal/txqueue"
)

// --- the sender against the simulated F4HWN KISS app, in virtual time ---

type simClock struct{ app *f4hwnkiss.App }

func (c simClock) now() time.Duration { return c.app.Now() }
func (c simClock) sleepUntil(ctx context.Context, t time.Duration) error {
	c.app.Advance(t)
	return ctx.Err()
}

type simSink struct {
	app  *f4hwnkiss.App
	seen int
}

func (s *simSink) write(p []byte) error {
	s.app.Write(s.app.Now(), p)
	return nil
}

func (s *simSink) waitAck(ctx context.Context, tag uint16, deadline time.Duration) (bool, error) {
	for {
		acks := s.app.Acks()
		for s.seen < len(acks) {
			a := acks[s.seen]
			s.seen++
			if a.Tag == tag {
				return true, nil
			}
		}
		if s.app.Now() >= deadline {
			return false, nil
		}
		s.app.Advance(s.app.Now() + 10*time.Millisecond)
	}
}

func fullFrames(t *testing.T, n int) [][]byte {
	t.Helper()
	r := rand.New(rand.NewPCG(3, 4))
	var out [][]byte
	for i := 0; i < n; i++ {
		info := make([]byte, 256)
		for j := range info {
			info[j] = byte(r.UintN(256))
		}
		b, err := ax25.NewUI(ax25.MustAddress("TESTB-1"), ax25.MustAddress("TESTA-1"), nil, ax25.PIDIPv4, info).Encode()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func checkAllSent(t *testing.T, app *f4hwnkiss.App, frames [][]byte) {
	t.Helper()
	tx := app.Transmitted()
	if len(tx) != len(frames) {
		t.Fatalf("%d of %d frames on the air; losses %v, discarded %d", len(tx), len(frames), app.Losses(), app.Discarded())
	}
	for i := range frames {
		if !bytes.Equal(tx[i].Frame, frames[i]) {
			t.Fatalf("frame %d changed", i)
		}
	}
}

func TestSenderFeedsTheF4HWNAppWithoutLoss(t *testing.T) {
	app := f4hwnkiss.New(f4hwnkiss.Config{})
	s := &sender{profile: pacing.F4HWNKissTNC, clock: simClock{app}, sink: &simSink{app: app}}
	if err := s.init(); err != nil {
		t.Fatal(err)
	}
	frames := fullFrames(t, 8)
	for _, f := range frames {
		if err := s.send(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	app.Advance(app.Now() + 5*time.Second)
	checkAllSent(t, app, frames)
}

func TestSenderWithAckModeSurvivesBusyChannel(t *testing.T) {
	busy := func(t time.Duration) bool { return t > 200*time.Millisecond && t < 4*time.Second }
	app := f4hwnkiss.New(f4hwnkiss.Config{AckMode: true, Busy: busy})
	s := &sender{profile: pacing.F4HWNKissTNCAck, clock: simClock{app}, sink: &simSink{app: app}}
	if err := s.init(); err != nil {
		t.Fatal(err)
	}
	frames := fullFrames(t, 5)
	for _, f := range frames {
		if err := s.send(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	checkAllSent(t, app, frames)
	if s.ackTimeouts != 0 {
		t.Fatalf("%d ACK timeouts", s.ackTimeouts)
	}
}

// --- the port over an in-memory connection, in real time ---

// fakeTNC is the far end of a net.Pipe: it records the frames it gets and can
// send frames to the host.
type fakeTNC struct {
	conn   net.Conn
	mu     sync.Mutex
	frames []kiss.Frame
	got    chan kiss.Frame
}

func newFakeTNC(conn net.Conn) *fakeTNC {
	f := &fakeTNC{conn: conn, got: make(chan kiss.Frame, 64)}
	go func() {
		d := &kiss.Decoder{}
		buf := make([]byte, 512)
		for {
			n, err := conn.Read(buf)
			d.Feed(buf[:n], func(fr kiss.Frame) {
				f.mu.Lock()
				f.frames = append(f.frames, fr)
				f.mu.Unlock()
				f.got <- fr
			})
			if err != nil {
				close(f.got)
				return
			}
		}
	}()
	return f
}

func (f *fakeTNC) next(t *testing.T) kiss.Frame {
	t.Helper()
	select {
	case fr, ok := <-f.got:
		if !ok {
			t.Fatal("connection closed")
		}
		return fr
	case <-time.After(5 * time.Second):
		t.Fatal("no frame from the host")
	}
	return kiss.Frame{}
}

func (f *fakeTNC) send(t *testing.T, fr kiss.Frame) {
	t.Helper()
	b, _ := kiss.Encode(fr)
	if _, err := f.conn.Write(b); err != nil {
		t.Fatal(err)
	}
}

// pipeDialer hands out pre-made connections, one per Dial.
type pipeDialer struct{ conns chan net.Conn }

func (d pipeDialer) String() string { return "pipe" }
func (d pipeDialer) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	select {
	case c := <-d.conns:
		return c, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func startPort(t *testing.T, profile pacing.Profile, rx func([]byte)) (*Port, pipeDialer, context.CancelFunc) {
	t.Helper()
	d := pipeDialer{conns: make(chan net.Conn, 4)}
	p := NewPort(Config{Dialer: d, Profile: profile}, rx)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Run returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Run did not stop")
		}
	})
	return p, d, cancel
}

func TestPortSendsParametersThenFramesAndDeliversReceived(t *testing.T) {
	received := make(chan []byte, 4)
	p, d, _ := startPort(t, pacing.F4HWNKissTNC, func(b []byte) { received <- b })
	host, radio := net.Pipe()
	d.conns <- host
	tnc := newFakeTNC(radio)

	if f := tnc.next(t); f.Command != kiss.CmdTxDelay || f.Payload[0] != 27 {
		t.Fatalf("first frame %+v, want TXDELAY 27", f)
	}
	if f := tnc.next(t); f.Command != kiss.CmdPersist || f.Payload[0] != 255 {
		t.Fatalf("second frame %+v, want PERSIST 255", f)
	}
	frame := fullFrames(t, 1)[0]
	if !p.Send(frame) {
		t.Fatal("queue full")
	}
	if f := tnc.next(t); f.Command != kiss.CmdData || !bytes.Equal(f.Payload, frame) {
		t.Fatalf("data frame %+v", f)
	}

	// The radio hears something and hands it to the host.
	heard, _ := ax25.NewUI(ax25.MustAddress("TESTA-1"), ax25.MustAddress("TESTB-1"), nil, ax25.PIDIPv4, []byte{0x45}).Encode()
	tnc.send(t, kiss.Data(0, heard))
	select {
	case got := <-received:
		if !bytes.Equal(got, heard) {
			t.Fatalf("received % X", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing received")
	}
}

func TestPortReconnectsAfterLinkLoss(t *testing.T) {
	quick := pacing.Generic // whole frames, no waiting: keeps the test fast
	p, d, _ := startPort(t, quick, func([]byte) {})
	host1, radio1 := net.Pipe()
	d.conns <- host1
	radio1.Close() // a USB reset: the port vanishes
	host2, radio2 := net.Pipe()
	d.conns <- host2
	tnc := newFakeTNC(radio2)

	frame := fullFrames(t, 1)[0]
	p.Send(frame)
	if f := tnc.next(t); !bytes.Equal(f.Payload, frame) {
		t.Fatalf("frame after reconnect %+v", f)
	}
	if s := p.Stats(); s.Connects != 2 || s.Disconnects != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func TestPortWaitsForAckModeEcho(t *testing.T) {
	profile := pacing.F4HWNKissTNCAck
	p, d, _ := startPort(t, profile, func([]byte) {})
	host, radio := net.Pipe()
	d.conns <- host
	tnc := newFakeTNC(radio)
	tnc.next(t) // TXDELAY
	tnc.next(t) // PERSIST

	frames := fullFrames(t, 2)
	p.Send(frames[0])
	p.Send(frames[1])
	first := tnc.next(t)
	tag, _ := kiss.AckTag(kiss.Frame{Command: kiss.CmdAckMode, Payload: first.Payload[:2]})
	if first.Command != kiss.CmdAckMode || !bytes.Equal(first.Payload[2:], frames[0]) {
		t.Fatalf("first frame %+v", first)
	}
	// No echo yet: the second frame must not come.
	select {
	case f := <-tnc.got:
		t.Fatalf("second frame %+v before the echo", f)
	case <-time.After(700 * time.Millisecond):
	}
	tnc.send(t, kiss.Frame{Command: kiss.CmdAckMode, Payload: []byte{byte(tag >> 8), byte(tag)}})
	second := tnc.next(t)
	if second.Command != kiss.CmdAckMode || !bytes.Equal(second.Payload[2:], frames[1]) {
		t.Fatalf("second frame %+v", second)
	}
}

func TestPortDropsARepeatThatComesTooSoon(t *testing.T) {
	p, d, _ := startPort(t, pacing.Generic, func([]byte) {})
	host, radio := net.Pipe()
	d.conns <- host
	tnc := newFakeTNC(radio)

	syn, other := fullFrames(t, 2)[0], fullFrames(t, 2)[1]
	m := txqueue.Meta{Urgent: true, Replace: "seg 1", Repeat: "seg 1"}
	p.SendMeta(syn, m)
	if f := tnc.next(t); !bytes.Equal(f.Payload, syn) {
		t.Fatalf("first %+v", f)
	}
	// TCP repeats the segment before an answer could have arrived.
	if !p.SendMeta(syn, m) {
		t.Fatal("a repeat must not count as a full queue")
	}
	p.Send(other)
	if f := tnc.next(t); !bytes.Equal(f.Payload, other) {
		t.Fatalf("the repeat went out: %+v", f)
	}
	deadline := time.Now().Add(time.Second) // FramesOut counts after the write
	for p.Stats().FramesOut < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s := p.Stats(); s.Repeats != 1 || s.FramesOut != 2 {
		t.Fatalf("stats %+v", s)
	}
}

// The other station is on the air from 0.15 s to 2.15 s, so the first frame
// has to wait. Without noting that frame, the estimate runs out while the
// first frame is on the air: the next one reaches the app during that
// transmission and laps the USB ring.
func TestSenderWaitsLongerAfterHearingAnotherStation(t *testing.T) {
	busy := func(t time.Duration) bool { return t >= 150*time.Millisecond && t < 2150*time.Millisecond }
	frames := fullFrames(t, 3)
	run := func(heard *heardLog) *f4hwnkiss.App {
		app := f4hwnkiss.New(f4hwnkiss.Config{Busy: busy})
		s := &sender{profile: pacing.F4HWNKissTNC, clock: simClock{app}, sink: &simSink{app: app}, heard: heard}
		if err := s.init(); err != nil {
			t.Fatal(err)
		}
		for _, f := range frames {
			if err := s.send(context.Background(), f); err != nil {
				t.Fatal(err)
			}
		}
		app.Advance(app.Now() + 5*time.Second)
		return app
	}
	if app := run(nil); len(app.Transmitted()) == len(frames) {
		t.Fatal("without the heard frame this scenario should lose a frame")
	}
	heard := &heardLog{}
	heard.add(150*time.Millisecond, 2150*time.Millisecond)
	checkAllSent(t, run(heard), frames)
}

// The other station transmits until 3 s, longer than the estimate for the
// first frame. The next frame comes only after that station has finished
// (as a TCP acknowledgement does); the first frame is then still waiting or
// on the air, and handing over the next one at once would lose it.
func TestSenderRechecksThePreviousFrameBeforeTheNext(t *testing.T) {
	busy := func(t time.Duration) bool { return t >= 150*time.Millisecond && t < 3*time.Second }
	frames := fullFrames(t, 2)
	run := func(hear bool) *f4hwnkiss.App {
		app := f4hwnkiss.New(f4hwnkiss.Config{Busy: busy})
		heard := &heardLog{}
		s := &sender{profile: pacing.F4HWNKissTNC, clock: simClock{app}, sink: &simSink{app: app}, heard: heard}
		if err := s.init(); err != nil {
			t.Fatal(err)
		}
		if err := s.send(context.Background(), frames[0]); err != nil {
			t.Fatal(err)
		}
		if app.Now() >= 3*time.Second {
			t.Fatalf("the estimate ran to %v; the scenario needs it to end earlier", app.Now())
		}
		app.Advance(3050 * time.Millisecond)
		if hear {
			heard.add(150*time.Millisecond, 3*time.Second) // known only now
		}
		if err := s.send(context.Background(), frames[1]); err != nil {
			t.Fatal(err)
		}
		app.Advance(app.Now() + 5*time.Second)
		return app
	}
	if app := run(false); len(app.Transmitted()) == len(frames) {
		t.Fatal("without the heard frame this scenario should lose a frame")
	}
	checkAllSent(t, run(true), frames)
}

func TestHeardLogOnlyShowsWhatHasEnded(t *testing.T) {
	l := &heardLog{}
	l.add(1*time.Second, 2*time.Second)
	l.add(3*time.Second, 4*time.Second)
	if got := l.since(0, 3*time.Second); len(got) != 1 || got[0].end != 2*time.Second {
		t.Fatalf("%v", got)
	}
	if got := l.since(2*time.Second, 5*time.Second); len(got) != 1 || got[0].start != 3*time.Second {
		t.Fatalf("%v", got)
	}
	for i := 0; i < 40; i++ {
		l.add(time.Duration(i)*time.Second, time.Duration(i+1)*time.Second)
	}
	if n := len(l.since(0, time.Hour)); n != 16 {
		t.Fatalf("keeps %d frames", n)
	}
}

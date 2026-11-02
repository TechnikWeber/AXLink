// SPDX-License-Identifier: GPL-3.0-or-later

package pacing

import (
	"bytes"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/airtime"
	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/kiss"
	"github.com/TechnikWeber/AXLink/internal/sim/f4hwnkiss"
)

// fullFrames returns n UI frames with 256 info bytes, like a full IP packet
// at MTU 256.
func fullFrames(t *testing.T, n int) [][]byte {
	t.Helper()
	r := rand.New(rand.NewPCG(7, 7))
	var out [][]byte
	for i := 0; i < n; i++ {
		info := make([]byte, 256)
		for j := range info {
			info[j] = byte(r.UintN(256))
		}
		info[0] = byte(i) // tell the frames apart
		f := ax25.NewUI(ax25.MustAddress("TESTB-1"), ax25.MustAddress("TESTA-1"), nil, ax25.PIDIPv4, info)
		b, err := f.Encode()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func encode(t *testing.T, f kiss.Frame) []byte {
	t.Helper()
	b, err := kiss.Encode(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func apply(app *f4hwnkiss.App, plan Plan) {
	for _, w := range plan.Writes {
		app.Write(w.At, w.Data)
	}
}

func openPort(t *testing.T, app *f4hwnkiss.App, p Profile) time.Duration {
	t.Helper()
	at := time.Duration(0)
	for _, f := range p.Init(0) {
		app.Write(at, encode(t, f))
	}
	return 100 * time.Millisecond // the app has read the parameters by then
}

func sentFrames(app *f4hwnkiss.App) [][]byte {
	var out [][]byte
	for _, tx := range app.Transmitted() {
		out = append(out, tx.Frame)
	}
	return out
}

func sameFrames(t *testing.T, got, want [][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d frames on the air, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("frame %d changed on the way", i)
		}
	}
}

// Today's failure mode 1: a full frame written in one go laps the 256-byte ring.
func TestF4HWNWholeFullFrameIsLost(t *testing.T) {
	app := f4hwnkiss.New(f4hwnkiss.Config{})
	frame := fullFrames(t, 1)[0]
	enc := encode(t, kiss.Data(0, frame))
	if len(enc) < f4hwnkiss.RingSize {
		t.Fatalf("test frame too small: %d bytes", len(enc))
	}
	apply(app, Generic.Plan(0, enc, frame))
	app.Advance(5 * time.Second)
	if len(app.Transmitted()) != 0 || len(app.Losses()) == 0 {
		t.Fatalf("transmitted %d, losses %v", len(app.Transmitted()), app.Losses())
	}
}

// Today's failure mode 2: a second frame that arrives while the first one
// waits is discarded, and the radio shows no counter for it.
func TestF4HWNSecondFrameWhilePendingIsDiscarded(t *testing.T) {
	app := f4hwnkiss.New(f4hwnkiss.Config{})
	var stream []byte
	for i := 0; i < 2; i++ {
		f := ax25.NewUI(ax25.MustAddress("TESTB-1"), ax25.MustAddress("TESTA-1"), nil, ax25.PIDNoL3,
			[]byte{byte('A' + i), 'x', 'y', 'z'})
		b, _ := f.Encode()
		stream = append(stream, encode(t, kiss.Data(0, b))...)
	}
	app.Write(0, stream) // well below 256 bytes: no ring overrun
	app.Advance(5 * time.Second)
	if n := len(app.Transmitted()); n != 1 {
		t.Fatalf("%d frames transmitted, want 1", n)
	}
	if app.Discarded() == 0 || len(app.Losses()) != 0 {
		t.Fatalf("discarded %d, losses %v", app.Discarded(), app.Losses())
	}
}

// With the f4hwn profile, full frames reach the air intact and in order.
func TestF4HWNPacedFullFramesArrive(t *testing.T) {
	app := f4hwnkiss.New(f4hwnkiss.Config{})
	p := F4HWNKissTNC
	start := openPort(t, app, p)
	frames := fullFrames(t, 10)
	for _, f := range frames {
		plan := p.Plan(start, encode(t, kiss.Data(0, f)), f)
		for _, w := range plan.Writes {
			if len(w.Data) > p.ChunkSize {
				t.Fatalf("chunk of %d bytes", len(w.Data))
			}
		}
		apply(app, plan)
		start = plan.Done
	}
	app.Advance(start + time.Second)
	sameFrames(t, sentFrames(app), frames)
	if len(app.Losses()) != 0 || app.Discarded() != 0 {
		t.Fatalf("losses %v, discarded %d", app.Losses(), app.Discarded())
	}
	// The app sent 40 flags before each frame, as the profile set via TXDELAY.
	for _, tx := range app.Transmitted() {
		want := airtime.Duration(airtime.Bits(tx.Frame, 40, 3), 1200)
		if got := tx.End - tx.Start; got != want {
			t.Fatalf("airtime %v, want %v", got, want)
		}
	}
	// Each frame left before the host handed over the next one.
	sent := app.Transmitted()
	for i := 1; i < len(sent); i++ {
		if sent[i].Start < sent[i-1].End {
			t.Fatal("transmissions overlap")
		}
	}
}

// Without ACKMODE the host cannot see that the channel was busy: the first
// frame waits, the host's estimate runs out, and the next frame is lost.
// This documents why the firmware should get ACKMODE.
func TestF4HWNBusyChannelLosesAFrameWithoutAckMode(t *testing.T) {
	busy := func(t time.Duration) bool { return t > 150*time.Millisecond && t < 2500*time.Millisecond }
	app := f4hwnkiss.New(f4hwnkiss.Config{Busy: busy})
	p := F4HWNKissTNC
	start := openPort(t, app, p)
	frames := fullFrames(t, 3)
	for _, f := range frames {
		plan := p.Plan(start, encode(t, kiss.Data(0, f)), f)
		apply(app, plan)
		start = plan.Done
	}
	app.Advance(start + 10*time.Second)
	if got := len(app.Transmitted()); got >= len(frames) {
		t.Fatalf("all %d frames arrived; the model no longer shows the known limit", got)
	}
}

// With the proposed ACKMODE extension the host waits for the echo, so a busy
// channel delays frames but loses none.
func TestF4HWNAckModeSurvivesBusyChannel(t *testing.T) {
	busy := func(t time.Duration) bool { return t > 150*time.Millisecond && t < 2500*time.Millisecond }
	app := f4hwnkiss.New(f4hwnkiss.Config{Busy: busy, AckMode: true})
	p := F4HWNKissTNCAck
	start := openPort(t, app, p)
	frames := fullFrames(t, 5)
	for i, f := range frames {
		tag := uint16(i + 1)
		apply(app, p.Plan(start, encode(t, kiss.AckModeData(0, tag, f)), f))
		deadline := start + 30*time.Second
		for len(app.Acks()) <= i {
			if app.Now() > deadline {
				t.Fatalf("no echo for frame %d", i)
			}
			app.Advance(app.Now() + 10*time.Millisecond)
		}
		if got := app.Acks()[i].Tag; got != tag {
			t.Fatalf("echo tag %d, want %d", got, tag)
		}
		start = app.Now()
	}
	sameFrames(t, sentFrames(app), frames)
	if len(app.Losses()) != 0 || app.Discarded() != 0 {
		t.Fatalf("losses %v, discarded %d", app.Losses(), app.Discarded())
	}
}

// The firmware's own host test expects KISS TXDELAY 30 to give 45 flags.
func TestF4HWNTxDelayConversion(t *testing.T) {
	app := f4hwnkiss.New(f4hwnkiss.Config{})
	app.Write(0, encode(t, kiss.Param(0, kiss.CmdTxDelay, 30)))
	f := ax25.NewUI(ax25.MustAddress("TESTB-1"), ax25.MustAddress("TESTA-1"), nil, ax25.PIDNoL3, []byte("x"))
	b, _ := f.Encode()
	app.Write(50*time.Millisecond, encode(t, kiss.Data(0, b)))
	app.Advance(2 * time.Second)
	tx := app.Transmitted()
	if len(tx) != 1 {
		t.Fatalf("%d transmissions", len(tx))
	}
	if got, want := tx[0].End-tx[0].Start, airtime.Duration(airtime.Bits(b, 45, 3), 1200); got != want {
		t.Fatalf("airtime %v, want %v (45 flags)", got, want)
	}
}

func TestPlanChunksAndEstimate(t *testing.T) {
	frame := fullFrames(t, 1)[0]
	enc := encode(t, kiss.Data(0, frame))
	plan := F4HWNKissTNC.Plan(time.Second, enc, frame)
	var joined []byte
	for i, w := range plan.Writes {
		if w.At != time.Second+time.Duration(i)*40*time.Millisecond {
			t.Fatalf("write %d at %v", i, w.At)
		}
		joined = append(joined, w.Data...)
	}
	if !bytes.Equal(joined, enc) {
		t.Fatal("chunks do not add up to the frame")
	}
	tx := airtime.Duration(airtime.Bits(frame, 40, 3), 1200)
	if last := plan.Writes[len(plan.Writes)-1].At; plan.Done < last+tx {
		t.Fatalf("done %v before the frame can have been sent", plan.Done)
	}
	if g := Generic.Plan(0, enc, frame); len(g.Writes) != 1 || g.Done != 0 {
		t.Fatalf("generic plan %+v", g)
	}
}

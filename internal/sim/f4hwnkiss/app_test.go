// SPDX-License-Identifier: GPL-3.0-or-later

package f4hwnkiss

import (
	"bytes"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/kiss"
)

func uiFrame(t *testing.T, info string) []byte {
	t.Helper()
	b, err := ax25.NewUI(ax25.MustAddress("TESTB-1"), ax25.MustAddress("TESTA-1"), nil, ax25.PIDNoL3, []byte(info)).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func kissData(t *testing.T, frame []byte) []byte {
	t.Helper()
	b, err := kiss.Encode(kiss.Data(0, frame))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRingLapsAt256UnreadBytes(t *testing.T) {
	for _, tc := range []struct {
		n    int
		lost bool
	}{{255, false}, {256, true}} {
		app := New(Config{})
		app.Write(0, make([]byte, tc.n))
		app.Advance(PollEvery)
		if got := len(app.Losses()) == 1; got != tc.lost {
			t.Errorf("%d unread bytes: lost = %v, want %v", tc.n, got, tc.lost)
		}
	}
}

func TestFrameGoesOutInTheNextSlot(t *testing.T) {
	app := New(Config{})
	frame := uiFrame(t, "hello")
	app.Write(0, kissData(t, frame)) // 26 bytes: read by the poll at 20 ms
	app.Advance(time.Second)
	tx := app.Transmitted()
	if len(tx) != 1 || !bytes.Equal(tx[0].Frame, frame) {
		t.Fatalf("transmitted %+v", tx)
	}
	if tx[0].Start != SlotEvery {
		t.Fatalf("started at %v, want the 50 ms slot", tx[0].Start)
	}
}

func TestPersistenceDrawCanDefer(t *testing.T) {
	// rand32() & 255 = 200 > P 63: the app never sends.
	app := New(Config{Rand: func() uint32 { return 200 }})
	app.Write(0, kissData(t, uiFrame(t, "x")))
	app.Advance(5 * time.Second)
	if len(app.Transmitted()) != 0 {
		t.Fatal("sent despite the persistence draw")
	}
}

func TestBusyChannelDefers(t *testing.T) {
	app := New(Config{Busy: func(t time.Duration) bool { return t < time.Second }})
	app.Write(0, kissData(t, uiFrame(t, "x")))
	app.Advance(3 * time.Second)
	tx := app.Transmitted()
	if len(tx) != 1 || tx[0].Start < time.Second {
		t.Fatalf("transmitted %+v", tx)
	}
}

func TestHostDataHeldDuringTransmission(t *testing.T) {
	app := New(Config{})
	app.Write(0, kissData(t, uiFrame(t, "first")))
	app.Advance(SlotEvery) // the frame goes on the air at 50 ms
	if !app.Transmitting() {
		t.Fatal("not transmitting")
	}
	app.Write(SlotEvery+10*time.Millisecond, make([]byte, 300)) // held by USB
	end := app.Transmitted()[0].End
	app.Advance(end - time.Millisecond)
	if len(app.Losses()) != 0 {
		t.Fatal("held data reached the ring during the transmission")
	}
	app.Advance(end + PollEvery) // delivered at once, then the first poll
	if l := app.Losses(); len(l) != 1 || l[0].Bytes != 300 {
		t.Fatalf("losses %v, want one of 300 bytes", l)
	}
}

func TestAckModeOnlyWhenEnabled(t *testing.T) {
	frame := uiFrame(t, "tagged")
	enc, _ := kiss.Encode(kiss.AckModeData(0, 0xBEEF, frame))
	for _, ack := range []bool{false, true} {
		app := New(Config{AckMode: ack})
		app.Write(0, enc)
		app.Advance(3 * time.Second)
		if ack {
			tx := app.Transmitted()
			if len(tx) != 1 || !bytes.Equal(tx[0].Frame, frame) {
				t.Fatalf("ACKMODE frame transmitted as %+v", tx)
			}
			if a := app.Acks(); len(a) != 1 || a[0].Tag != 0xBEEF || a[0].At != tx[0].End {
				t.Fatalf("acks %+v", a)
			}
		} else if len(app.Transmitted()) != 0 || len(app.Acks()) != 0 {
			t.Fatal("the original app must ignore command 0x0C")
		}
	}
}

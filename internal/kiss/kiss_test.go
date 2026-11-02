// SPDX-License-Identifier: GPL-3.0-or-later

package kiss

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

func decodeAll(t *testing.T, d *Decoder, chunks ...[]byte) []Frame {
	t.Helper()
	var out []Frame
	for _, c := range chunks {
		d.Feed(c, func(f Frame) { out = append(out, f) })
	}
	return out
}

func TestEncodeEscapesPayload(t *testing.T) {
	got, err := Encode(Data(0, []byte{0x01, FEND, FESC, 0x02}))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{FEND, 0x00, 0x01, FESC, TFEND, FESC, TFESC, 0x02, FEND}
	if !bytes.Equal(got, want) {
		t.Fatalf("got % X, want % X", got, want)
	}
}

func TestEncodeEscapesTypeByte(t *testing.T) {
	// Port 12, data: the type byte is 0xC0 and must not end the frame.
	got, err := Encode(Data(12, []byte{0x41}))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{FEND, FESC, TFEND, 0x41, FEND}
	if !bytes.Equal(got, want) {
		t.Fatalf("got % X, want % X", got, want)
	}
	frames := decodeAll(t, &Decoder{}, got)
	if len(frames) != 1 || frames[0].Port != 12 || frames[0].Command != CmdData {
		t.Fatalf("decoded %+v", frames)
	}
}

func TestRejectsPortAbove15(t *testing.T) {
	if _, err := Encode(Frame{Port: 16}); err != ErrPort {
		t.Fatalf("err = %v, want ErrPort", err)
	}
}

func TestRoundTripAllByteValues(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var stream []byte
	var want []Frame
	for i := 0; i < 200; i++ {
		p := make([]byte, r.IntN(300))
		for j := range p {
			p[j] = byte(r.UintN(256))
		}
		f := Frame{Port: uint8(r.UintN(16)), Command: Command(r.UintN(16)), Payload: p}
		want = append(want, f)
		var err error
		if stream, err = Append(stream, f); err != nil {
			t.Fatal(err)
		}
	}
	// Feed one byte at a time: frames must survive arbitrary read boundaries.
	d := &Decoder{}
	var got []Frame
	for _, b := range stream {
		d.Feed([]byte{b}, func(f Frame) { got = append(got, f) })
	}
	if len(got) != len(want) {
		t.Fatalf("got %d frames, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Port != want[i].Port || got[i].Command != want[i].Command ||
			!bytes.Equal(got[i].Payload, want[i].Payload) {
			t.Fatalf("frame %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if d.Dropped() != 0 {
		t.Fatalf("dropped %d frames", d.Dropped())
	}
}

func TestDecoderIgnoresLeadingGarbageAndEmptyFrames(t *testing.T) {
	frames := decodeAll(t, &Decoder{}, []byte{0x41, 0x42, FEND, FEND, 0x00, 0xAA, FEND, FEND})
	if len(frames) != 1 || !bytes.Equal(frames[0].Payload, []byte{0xAA}) {
		t.Fatalf("decoded %+v", frames)
	}
}

func TestDecoderDropsInvalidEscape(t *testing.T) {
	d := &Decoder{}
	frames := decodeAll(t, d, []byte{FEND, 0x00, FESC, 0x41, FEND, 0x00, 0x42, FEND})
	if len(frames) != 1 || !bytes.Equal(frames[0].Payload, []byte{0x42}) {
		t.Fatalf("decoded %+v", frames)
	}
	if d.Dropped() != 1 {
		t.Fatalf("dropped = %d, want 1", d.Dropped())
	}
}

func TestDecoderDropsEscapeBeforeFEND(t *testing.T) {
	d := &Decoder{}
	frames := decodeAll(t, d, []byte{FEND, 0x00, 0x41, FESC, FEND})
	if len(frames) != 0 || d.Dropped() != 1 {
		t.Fatalf("frames %+v, dropped %d", frames, d.Dropped())
	}
}

func TestDecoderDropsOversizeFrame(t *testing.T) {
	d := &Decoder{MaxFrame: 4}
	frames := decodeAll(t, d, []byte{FEND, 0x00, 1, 2, 3, 4, FEND, 0x00, 9, FEND})
	if len(frames) != 1 || !bytes.Equal(frames[0].Payload, []byte{9}) {
		t.Fatalf("decoded %+v", frames)
	}
	if d.Dropped() != 1 {
		t.Fatalf("dropped = %d, want 1", d.Dropped())
	}
}

func TestAckMode(t *testing.T) {
	f := AckModeData(0, 0x1234, []byte{0xAA, 0xBB})
	if f.Command != CmdAckMode || !bytes.Equal(f.Payload, []byte{0x12, 0x34, 0xAA, 0xBB}) {
		t.Fatalf("frame %+v", f)
	}
	if _, ok := AckTag(f); ok {
		t.Fatal("a tagged data frame is not an echo")
	}
	tag, ok := AckTag(Frame{Command: CmdAckMode, Payload: []byte{0x12, 0x34}})
	if !ok || tag != 0x1234 {
		t.Fatalf("tag %#x ok %v", tag, ok)
	}
}

func FuzzDecoder(f *testing.F) {
	f.Add([]byte{FEND, 0x00, 0x01, FESC, TFEND, FEND})
	f.Add([]byte{FESC, FEND, FESC, FESC, 0x00, FEND})
	f.Fuzz(func(t *testing.T, in []byte) {
		d := &Decoder{MaxFrame: 512}
		d.Feed(in, func(fr Frame) {
			enc, err := Encode(fr)
			if err != nil {
				t.Fatal(err)
			}
			var again []Frame
			(&Decoder{MaxFrame: 512}).Feed(enc, func(g Frame) { again = append(again, g) })
			if len(again) != 1 || again[0].TypeByte() != fr.TypeByte() ||
				!bytes.Equal(again[0].Payload, fr.Payload) {
				t.Fatalf("re-encoding changed the frame: %+v -> %+v", fr, again)
			}
		})
	})
}

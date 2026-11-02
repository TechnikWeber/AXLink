// SPDX-License-Identifier: GPL-3.0-or-later

package ax25

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestParseAddress(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Address
	}{
		{"N0CALL", Address{Call: "N0CALL"}},
		{"n0call-7", Address{Call: "N0CALL", SSID: 7}},
		{"TESTB-0", Address{Call: "TESTB"}},
		{" QST ", Address{Call: "QST"}},
		{"A-15", Address{Call: "A", SSID: 15}},
	} {
		got, err := ParseAddress(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseAddress(%q) = %+v, %v; want %+v", tc.in, got, err, tc.want)
		}
	}
	for _, tc := range []struct {
		in   string
		want error
	}{
		{"", ErrCallsign},
		{"TOOLONG", ErrCallsign},
		{"N0 CAL", ErrCallsign},
		{"N0ÄB", ErrCallsign},
		{"N0CALL-16", ErrSSID},
		{"N0CALL-", ErrSSID},
		{"N0-CALL", ErrSSID},
	} {
		if _, err := ParseAddress(tc.in); !errors.Is(err, tc.want) {
			t.Errorf("ParseAddress(%q) error = %v, want %v", tc.in, err, tc.want)
		}
	}
}

func TestAddressString(t *testing.T) {
	if s := MustAddress("N0CALL-0").String(); s != "N0CALL" {
		t.Errorf("got %q", s)
	}
	if s := MustAddress("N0CALL-10").String(); s != "N0CALL-10" {
		t.Errorf("got %q", s)
	}
	if !MustAddress("TESTB").SameStation(Address{Call: "TESTB", Bit7: true}) {
		t.Error("Bit7 must not change the station identity")
	}
}

func TestUIFrameRoundTrip(t *testing.T) {
	path := []Address{MustAddress("WIDE1-1"), MustAddress("WIDE2-1")}
	path[0].Bit7 = true // already repeated
	f := NewUI(MustAddress("QST"), MustAddress("N0CALL-7"), path, PIDNoL3, []byte(">AXLink"))
	b, err := f.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 4*7+2+7 {
		t.Fatalf("length %d", len(b))
	}
	g, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !g.IsUI() || !g.IsCommand() || g.PID != PIDNoL3 || string(g.Info) != ">AXLink" {
		t.Fatalf("decoded %+v", g)
	}
	if g.Dest.String() != "QST" || g.Src.String() != "N0CALL-7" || len(g.Path) != 2 ||
		!g.Path[0].Bit7 || g.Path[1].Bit7 || g.Path[1].String() != "WIDE2-1" {
		t.Fatalf("addresses %+v", g)
	}
	again, _ := g.Encode()
	if !bytes.Equal(again, b) {
		t.Fatalf("re-encoded % X\nwant % X", again, b)
	}
}

func TestEncodeRejectsLongPath(t *testing.T) {
	path := make([]Address, MaxRepeaters+1)
	for i := range path {
		path[i] = MustAddress("WIDE1-1")
	}
	f := NewUI(MustAddress("QST"), MustAddress("N0CALL"), path, PIDNoL3, nil)
	if _, err := f.Encode(); err != ErrPathTooLong {
		t.Fatalf("err = %v", err)
	}
	f.Path = path[:MaxRepeaters]
	if _, err := f.Encode(); err != nil {
		t.Fatalf("8 repeaters must encode: %v", err)
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	good, _ := NewUI(MustAddress("TESTB-1"), MustAddress("TESTA-1"), nil, PIDIPv4, []byte{0x45}).Encode()
	mutate := func(fn func(b []byte) []byte) []byte {
		return fn(append([]byte(nil), good...))
	}
	for name, tc := range map[string]struct {
		frame []byte
		want  error
	}{
		"short":              {good[:10], ErrShort},
		"no control":         {good[:14], ErrShort},
		"no PID in UI":       {good[:15], ErrShort},
		"lower-case call":    {mutate(func(b []byte) []byte { b[0] = 't' << 1; return b }), ErrCallsign},
		"call after padding": {mutate(func(b []byte) []byte { b[3] = ' ' << 1; return b }), ErrCallsign},
		"end bit in call":    {mutate(func(b []byte) []byte { b[2] |= 1; return b }), ErrAddressField},
		"end bit never set": {mutate(func(b []byte) []byte {
			b[13] &^= 1
			var long []byte
			for i := 0; i < 10; i++ {
				long = append(long, b[7:14]...)
			}
			return append(b[:7], long...)
		}), ErrPathTooLong},
	} {
		if _, err := Decode(tc.frame); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	// The end bit on the destination address is a malformed address field.
	b := append([]byte(nil), good...)
	b[6] |= 1
	if _, err := Decode(b); !errors.Is(err, ErrAddressField) {
		t.Errorf("end bit on destination: err = %v", err)
	}
}

func TestFCS(t *testing.T) {
	// The CRC-16/X.25 check value.
	if got := FCS([]byte("123456789")); got != 0x906E {
		t.Fatalf("FCS = %#04x, want 0x906E", got)
	}
	b := AppendFCS([]byte("AXLink"))
	if !CheckFCS(b) {
		t.Fatal("valid FCS rejected")
	}
	b[1] ^= 0x01
	if CheckFCS(b) {
		t.Fatal("corrupted frame accepted")
	}
}

// TestPacketNetFrames decodes frames that Packet.NET sent and received in the
// two-node lab and compares the fields with Packet.NET's own strict parser.
func TestPacketNetFrames(t *testing.T) {
	frames := readJSONL(t, "testdata/packetnet-frames.jsonl")
	decoded := readJSONL(t, "testdata/packetnet-decoded.jsonl")
	if len(frames) != len(decoded) || len(frames) < 100 {
		t.Fatalf("%d frames, %d decoded", len(frames), len(decoded))
	}
	for i, raw := range frames {
		b, err := hex.DecodeString(raw["frame"].(string))
		if err != nil {
			t.Fatal(err)
		}
		f, err := Decode(b)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		want := decoded[i]
		if f.Src.String() != want["source"] || f.Dest.String() != want["destination"] ||
			int(f.PID) != int(want["pid"].(float64)) || !f.IsUI() ||
			!strings.EqualFold(hex.EncodeToString(f.Info), want["ipHex"].(string)) {
			t.Fatalf("frame %d: %s>%s pid %#x, want %v", i, f.Src, f.Dest, f.PID, want)
		}
		again, err := f.Encode()
		if err != nil || !bytes.Equal(again, b) {
			t.Fatalf("frame %d: re-encoding differs (%v)", i, err)
		}
	}
}

func readJSONL(t *testing.T, name string) []map[string]any {
	t.Helper()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var out []map[string]any
	s := bufio.NewScanner(file)
	s.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for s.Scan() {
		if strings.TrimSpace(s.Text()) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(s.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func FuzzDecode(f *testing.F) {
	good, _ := NewUI(MustAddress("TESTB-1"), MustAddress("TESTA-1"), nil, PIDIPv4, []byte{0x45, 0x00}).Encode()
	f.Add(good)
	f.Fuzz(func(t *testing.T, b []byte) {
		fr, err := Decode(b)
		if err != nil {
			return
		}
		// Anything we accept must encode again to the same bytes, except for
		// the reserved SSID bits, which we always send as 1.
		again, err := fr.Encode()
		if err != nil {
			t.Fatalf("accepted frame does not encode: %v", err)
		}
		if len(again) != len(b) {
			t.Fatalf("length changed: % X -> % X", b, again)
		}
	})
}

// SPDX-License-Identifier: GPL-3.0-or-later

package mesh

import (
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

func addr(s string, used bool) ax25.Address {
	a := ax25.MustAddress(s)
	a.Bit7 = used
	return a
}

func frame(t *testing.T, dest, src string, path []ax25.Address, pid byte, info string) []byte {
	t.Helper()
	b, err := ax25.NewUI(ax25.MustAddress(dest), ax25.MustAddress(src), path, pid, []byte(info)).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newRelay() (*Relay, *time.Time) {
	now := time.Unix(1_800_000_000, 0)
	r := NewRelay(ax25.MustAddress("TESTB-1"), func() time.Time { return now })
	r.SetOn(true)
	return r, &now
}

func path(t *testing.T, b []byte) string {
	t.Helper()
	f, err := ax25.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	s := ""
	for i, d := range f.Path {
		if i > 0 {
			s += ","
		}
		s += d.String()
		if d.Bit7 {
			s += "*"
		}
	}
	return s
}

func TestAFrameThatNamesTheRelayIsRepeated(t *testing.T) {
	r, _ := newRelay()
	in := frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTB-1", false)}, ax25.PIDIPv4, "an IP packet")
	out := r.Handle(in)
	if out == nil || path(t, out) != "TESTB-1*" {
		t.Fatalf("%q", path(t, out))
	}
	f, _ := ax25.Decode(out)
	if f.Dest.String() != "TESTC-1" || f.Src.String() != "TESTA-1" || string(f.Info) != "an IP packet" {
		t.Fatalf("%+v", f)
	}
	// The second relay of two.
	in = frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTD-1", true), addr("TESTB-1", false)}, ax25.PIDIPv4, "x")
	if out := r.Handle(in); path(t, out) != "TESTD-1*,TESTB-1*" {
		t.Fatalf("%q", path(t, out))
	}
	if r.Stats().Repeated != 2 {
		t.Fatalf("%+v", r.Stats())
	}
}

func TestNotRepeated(t *testing.T) {
	r, _ := newRelay()
	for name, in := range map[string][]byte{
		"no path":          frame(t, "TESTC-1", "TESTA-1", nil, ax25.PIDIPv4, "x"),
		"another relay":    frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTD-1", false)}, ax25.PIDIPv4, "x"),
		"already repeated": frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTB-1", true)}, ax25.PIDIPv4, "x"),
		"our own":          frame(t, "TESTC-1", "TESTB-1", []ax25.Address{addr("TESTB-1", false)}, ax25.PIDIPv4, "x"),
		"IP to flood":      frame(t, "TESTC-1", "TESTA-1", []ax25.Address{Flood}, ax25.PIDIPv4, "x"),
		"no hops left":     frame(t, "AXLINK", "TESTA-1", []ax25.Address{addr("AXHOP", false)}, ax25.PIDNoL3, "AXL1 ip=10.73.0.1/24"),
		"WIDE (APRS)":      frame(t, "APZK5M", "TESTA-7", []ax25.Address{addr("WIDE1-1", false)}, ax25.PIDNoL3, ":CQ       :hi"),
	} {
		if out := r.Handle(in); out != nil {
			t.Errorf("%s: repeated as %s", name, path(t, out))
		}
	}
	r.SetOn(false)
	if r.Handle(frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTB-1", false)}, ax25.PIDIPv4, "x")) != nil {
		t.Fatal("repeated while off")
	}
}

func TestABeaconIsFloodedOverTwoRelays(t *testing.T) {
	r, now := newRelay()
	in := frame(t, "AXLINK", "TESTC-1", []ax25.Address{Flood}, ax25.PIDNoL3, "AXL1 ip=10.73.0.3/24")
	out := r.Handle(in)
	if path(t, out) != "TESTB-1*,AXHOP-1" {
		t.Fatalf("%q", path(t, out))
	}
	// The same beacon over another relay, or once more: not again.
	other := frame(t, "AXLINK", "TESTC-1", []ax25.Address{addr("TESTD-1", true), addr("AXHOP-1", false)}, ax25.PIDNoL3,
		"AXL1 ip=10.73.0.3/24")
	if r.Handle(other) != nil || r.Handle(in) != nil {
		t.Fatal("a duplicate was repeated")
	}
	*now = now.Add(floodDup)
	// A second relay takes the last hop: the alias is used up.
	d := NewRelay(ax25.MustAddress("TESTD-1"), func() time.Time { return *now })
	d.SetOn(true)
	last := d.Handle(out)
	if path(t, last) != "TESTB-1*,TESTD-1*,AXHOP*" {
		t.Fatalf("%q", path(t, last))
	}
	// Nobody repeats it again, and B does not take what it already repeated.
	if r.Handle(last) != nil {
		t.Fatal("repeated after the last hop")
	}
	if s := r.Stats(); s.Flooded != 1 || s.Duplicates != 2 {
		t.Fatalf("%+v", s)
	}
}

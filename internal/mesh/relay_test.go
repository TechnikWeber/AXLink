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

// testRelay is a relay whose waits run when the test says so.
type testRelay struct {
	*Relay
	now       time.Time
	sent      [][]byte
	waits     []func()
	neighbors []ax25.Address
}

func newRelay(call string, neighbors ...string) *testRelay {
	tr := &testRelay{now: time.Unix(1_800_000_000, 0)}
	for _, n := range neighbors {
		tr.neighbors = append(tr.neighbors, ax25.MustAddress(n))
	}
	tr.Relay = NewRelay(Config{Call: ax25.MustAddress(call),
		Send:      func(f []byte) { tr.sent = append(tr.sent, f) },
		Neighbors: func() []ax25.Address { return tr.neighbors },
		Now:       func() time.Time { return tr.now },
		After:     func(d time.Duration, f func()) { tr.waits = append(tr.waits, f) },
		Rand:      func(n int64) int64 { return n / 2 }})
	tr.SetOn(true)
	return tr
}

// wait lets the waiting flooded frames go.
func (tr *testRelay) wait() {
	w := tr.waits
	tr.waits = nil
	for _, f := range w {
		f()
	}
}

func path(t *testing.T, b []byte) string {
	t.Helper()
	if b == nil {
		return "(nothing)"
	}
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

func (tr *testRelay) last(t *testing.T) string {
	t.Helper()
	if len(tr.sent) == 0 {
		return "(nothing)"
	}
	return path(t, tr.sent[len(tr.sent)-1])
}

func TestAFrameThatNamesTheRelayIsRepeatedAtOnce(t *testing.T) {
	r := newRelay("TESTB-1") // no neighbours needed for frames that name it
	r.Handle(frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTB-1", false)}, ax25.PIDIPv4, "an IP packet"))
	if len(r.sent) != 1 || r.last(t) != "TESTB-1*" {
		t.Fatalf("%d sent, %q", len(r.sent), r.last(t))
	}
	f, _ := ax25.Decode(r.sent[0])
	if f.Dest.String() != "TESTC-1" || f.Src.String() != "TESTA-1" || string(f.Info) != "an IP packet" {
		t.Fatalf("%+v", f)
	}
	// The second relay of two.
	r.Handle(frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTD-1", true), addr("TESTB-1", false)}, ax25.PIDIPv4, "x"))
	if r.last(t) != "TESTD-1*,TESTB-1*" || r.Stats().Repeated != 2 {
		t.Fatalf("%q %+v", r.last(t), r.Stats())
	}
}

func TestNotRepeated(t *testing.T) {
	r := newRelay("TESTB-1", "TESTA-1", "TESTC-1")
	for name, in := range map[string][]byte{
		"no path":          frame(t, "TESTC-1", "TESTA-1", nil, ax25.PIDIPv4, "x"),
		"another relay":    frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTD-1", false)}, ax25.PIDIPv4, "x"),
		"already repeated": frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTB-1", true)}, ax25.PIDIPv4, "x"),
		"our own":          frame(t, "TESTC-1", "TESTB-1", []ax25.Address{addr("TESTB-1", false)}, ax25.PIDIPv4, "x"),
		"IP to flood":      frame(t, "TESTC-1", "TESTA-1", []ax25.Address{Flood}, ax25.PIDIPv4, "x"),
		"no hops left":     frame(t, "AXLINK", "TESTA-1", []ax25.Address{addr("AXHOP", false)}, ax25.PIDNoL3, "AXL1 ip=10.73.0.1/24"),
		"WIDE (APRS)":      frame(t, "APZK5M", "TESTA-7", []ax25.Address{addr("WIDE1-1", false)}, ax25.PIDNoL3, ":CQ       :hi"),
	} {
		r.Handle(in)
		r.wait()
		if len(r.sent) != 0 {
			t.Fatalf("%s: repeated as %s", name, r.last(t))
		}
	}
	r.SetOn(false)
	r.Handle(frame(t, "TESTC-1", "TESTA-1", []ax25.Address{addr("TESTB-1", false)}, ax25.PIDIPv4, "x"))
	if len(r.sent) != 0 {
		t.Fatal("repeated while off")
	}
}

func TestABeaconIsFloodedForANeighbourThatMayNotHaveIt(t *testing.T) {
	// A line: A - B - C. B hears both; A and C hear only B.
	b := newRelay("TESTB-1", "TESTA-1", "TESTC-1")
	in := frame(t, "AXLINK", "TESTC-1", []ax25.Address{Flood}, ax25.PIDNoL3, "AXL1 ip=10.73.0.3/24")
	b.Handle(in)
	if len(b.sent) != 0 || len(b.waits) != 1 {
		t.Fatal("sent without its wait")
	}
	b.wait()
	if b.last(t) != "TESTB-1*,AXHOP-1" {
		t.Fatalf("%q", b.last(t))
	}
	// A hears B's copy: its only neighbour B has it, so A stays silent.
	a := newRelay("TESTA-1", "TESTB-1")
	a.Handle(b.sent[0])
	a.wait()
	if len(a.sent) != 0 || a.Stats().Covered != 1 {
		t.Fatalf("A repeated for nobody: %q %+v", a.last(t), a.Stats())
	}
	// The same beacon again, or over another relay: not again.
	b.Handle(in)
	b.Handle(frame(t, "AXLINK", "TESTC-1", []ax25.Address{addr("TESTD-1", true), addr("AXHOP-1", false)}, ax25.PIDNoL3,
		"AXL1 ip=10.73.0.3/24"))
	b.wait()
	if len(b.sent) != 1 || b.Stats().Duplicates != 2 {
		t.Fatalf("%d sent, %+v", len(b.sent), b.Stats())
	}
}

func TestEveryoneHearsEveryone(t *testing.T) {
	// X sends; Y and Z hear X and each other. Y's wait ends first: it
	// repeats. Z then hears Y's copy: X and Y have it, Z's neighbours are
	// all covered, and Z stays silent.
	y := newRelay("TESTY-1", "TESTX-1", "TESTZ-1")
	z := newRelay("TESTZ-1", "TESTX-1", "TESTY-1")
	in := frame(t, "AXLINK", "TESTX-1", []ax25.Address{Flood}, ax25.PIDNoL3, "AXL1 ip=10.73.0.9/24")
	y.Handle(in)
	z.Handle(in)
	y.wait()
	if len(y.sent) != 1 {
		t.Fatal("Y did not repeat")
	}
	z.Handle(y.sent[0])
	z.wait()
	if len(z.sent) != 0 || z.Stats().Covered != 1 {
		t.Fatalf("Z repeated as well: %q %+v", z.last(t), z.Stats())
	}
	// A station alone repeats nothing: nobody would hear it.
	alone := newRelay("TESTQ-1", "TESTX-1")
	alone.Handle(in)
	alone.wait()
	if len(alone.sent) != 0 {
		t.Fatal("repeated for the sender only")
	}
}

func TestTheLastHopUsesTheAliasUp(t *testing.T) {
	d := newRelay("TESTD-1", "TESTB-1", "TESTE-1")
	in := frame(t, "AXLINK", "TESTC-1", []ax25.Address{addr("TESTB-1", true), addr("AXHOP-1", false)}, ax25.PIDNoL3,
		"AXL1 ip=10.73.0.3/24")
	d.Handle(in)
	d.wait()
	if d.last(t) != "TESTB-1*,TESTD-1*,AXHOP*" {
		t.Fatalf("%q", d.last(t))
	}
	// Nobody repeats it after the last hop.
	e := newRelay("TESTE-1", "TESTD-1", "TESTF-1")
	e.Handle(d.sent[0])
	e.wait()
	if len(e.sent) != 0 {
		t.Fatal("repeated after the last hop")
	}
}

func TestARelayThatKnowsNobodyRepeatsAndACopyIsWeighedAgain(t *testing.T) {
	in := frame(t, "AXLINK", "TESTA-1", []ax25.Address{Flood}, ax25.PIDNoL3, "AXL1 ip=10.73.0.1/24")
	b := newRelay("TESTB-1") // just started, or the neighbours' beacons collided
	b.Handle(in)
	b.wait()
	if len(b.sent) != 1 {
		t.Fatal("a relay that knows nobody did not repeat")
	}
	// Knowing only A, A's beacon needs no repeat; once C is known, A's next
	// copy of it (A heard no relay's copy) is repeated.
	k := newRelay("TESTB-1", "TESTA-1")
	k.Handle(in)
	k.wait()
	if len(k.sent) != 0 {
		t.Fatal("repeated for the sender only")
	}
	k.neighbors = append(k.neighbors, ax25.MustAddress("TESTC-1"))
	k.now = k.now.Add(12 * time.Second)
	k.Handle(in)
	k.wait()
	if len(k.sent) != 1 {
		t.Fatal("the sender's next copy was not weighed again")
	}
	// The sender sends it once more: our copy did not reach it. Repeated again.
	k.now = k.now.Add(12 * time.Second)
	k.Handle(in)
	k.wait()
	if len(k.sent) != 2 {
		t.Fatal("the sender's copy after 12 s was dropped")
	}
	// The same copy within 5 s is a duplicate.
	k.now = k.now.Add(time.Second)
	k.Handle(in)
	k.wait()
	if len(k.sent) != 2 {
		t.Fatal("repeated twice within 5 s")
	}
}

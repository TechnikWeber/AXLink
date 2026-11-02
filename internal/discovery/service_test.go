// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"bytes"
	"context"
	"log/slog"
	"math/rand/v2"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

type testService struct {
	*Service
	now  time.Time
	sent []Beacon
	log  bytes.Buffer
}

func newTestService(t *testing.T, every time.Duration) *testService {
	t.Helper()
	ts := &testService{now: time.Unix(1_800_000_000, 0)}
	ts.Service = New(Config{
		Call: ax25.MustAddress("TESTA-1"), Addr: netip.MustParsePrefix("10.73.0.1/24"),
		Tags: []string{"AUTO"}, Every: every,
		Send: func(frame []byte) bool {
			f, err := ax25.Decode(frame)
			if err != nil {
				t.Fatal(err)
			}
			_, b, ok, err := FromFrame(f)
			if !ok || err != nil {
				t.Fatalf("sent something that is not a beacon: %v", err)
			}
			ts.sent = append(ts.sent, b)
			return true
		},
		Logger: slog.New(slog.NewTextHandler(&ts.log, nil)),
		Now:    func() time.Time { return ts.now },
		Rand:   rand.New(rand.NewPCG(1, 2)),
	})
	return ts
}

// step advances the clock to t and sends whatever is due by then.
func (ts *testService) step(t time.Time) {
	for {
		send, ask, wake := ts.due(ts.now)
		if send {
			ts.send(ts.now, ask)
			continue
		}
		if wake.After(t) {
			ts.now = t
			return
		}
		ts.now = wake
	}
}

func (ts *testService) after(d time.Duration) time.Time { return ts.now.Add(d) }

func beaconFrom(t *testing.T, call string, b Beacon) []byte {
	t.Helper()
	if b.Every == 0 {
		b.Every = DefaultEvery
	}
	raw, err := b.Frame(ax25.MustAddress(call))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func addr(s string) netip.Prefix { return netip.MustParsePrefix(s) }

func TestFirstBeaconAsksAndComesWithinOneToFiveSeconds(t *testing.T) {
	ts := newTestService(t, DefaultEvery)
	start := ts.now
	ts.step(start.Add(999 * time.Millisecond))
	if len(ts.sent) != 0 {
		t.Fatal("sent before 1 s")
	}
	ts.step(start.Add(5 * time.Second))
	if len(ts.sent) != 1 || !ts.sent[0].Ask || ts.sent[0].Addr != addr("10.73.0.1/24") {
		t.Fatalf("sent %+v", ts.sent)
	}
	first := ts.now
	ts.step(first.Add(DefaultEvery * 9 / 10).Add(-time.Second))
	if len(ts.sent) != 1 {
		t.Fatal("periodic beacon before 90 % of the interval")
	}
	ts.step(first.Add(DefaultEvery * 11 / 10).Add(5 * time.Second))
	if len(ts.sent) != 2 || ts.sent[1].Ask {
		t.Fatalf("sent %+v", ts.sent)
	}
}

func TestHandleLearnsAStation(t *testing.T) {
	ts := newTestService(t, DefaultEvery)
	if !ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24"), Tags: []string{"INET"}})) {
		t.Fatal("not taken as a beacon")
	}
	call, ok := ts.Lookup(netip.MustParseAddr("10.73.0.2"))
	if !ok || call.String() != "TESTB-1" {
		t.Fatalf("lookup %v %v", call, ok)
	}
	if got := ts.Summary(); got != "TESTB-1 10.73.0.2/24 INET" {
		t.Fatalf("summary %q", got)
	}
	if !strings.Contains(ts.log.String(), `msg="station heard" call=TESTB-1 ip=10.73.0.2/24 tags=INET`) {
		t.Fatalf("log %s", ts.log.String())
	}
}

func TestStationIsForgottenAfterThreeIntervals(t *testing.T) {
	ts := newTestService(t, 0)
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24"), Every: 5 * time.Minute}))
	ts.now = ts.after(16 * time.Minute) // 3 × 5 min + 60 s
	if _, ok := ts.Lookup(netip.MustParseAddr("10.73.0.2")); !ok {
		t.Fatal("forgotten too early")
	}
	ts.now = ts.after(time.Second)
	if _, ok := ts.Lookup(netip.MustParseAddr("10.73.0.2")); ok || len(ts.Stations()) != 0 {
		t.Fatal("not forgotten")
	}
}

func TestTheNewerStationWinsAnAddress(t *testing.T) {
	ts := newTestService(t, 0)
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24")}))
	ts.now = ts.after(time.Second)
	ts.Handle(beaconFrom(t, "TESTC-1", Beacon{Addr: addr("10.73.0.2/24")}))
	if call, _ := ts.Lookup(netip.MustParseAddr("10.73.0.2")); call.String() != "TESTC-1" {
		t.Fatalf("lookup %v", call)
	}
	if ts.Stats().Conflicts != 1 || !strings.Contains(ts.log.String(), "two stations announce the same address") {
		t.Fatalf("stats %+v, log %s", ts.Stats(), ts.log.String())
	}
}

func TestAnswerToAnAskComesTwoToEightSecondsLater(t *testing.T) {
	ts := newTestService(t, DefaultEvery)
	ts.step(ts.after(time.Minute)) // the own first beacon is out
	heard := ts.now
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24"), Ask: true}))
	ts.step(heard.Add(2*time.Second - time.Millisecond))
	if len(ts.sent) != 1 {
		t.Fatal("answered before 2 s")
	}
	ts.step(heard.Add(8 * time.Second))
	if len(ts.sent) != 2 || ts.sent[1].Ask {
		t.Fatalf("sent %+v", ts.sent)
	}
}

func TestOnlyANewStationOrAnAskGetsAnAnswer(t *testing.T) {
	ts := newTestService(t, DefaultEvery)
	ts.step(ts.after(time.Minute))
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24")})) // new: answer
	ts.step(ts.after(time.Minute))
	if len(ts.sent) != 2 {
		t.Fatalf("a new station got no answer: %d", len(ts.sent))
	}
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24")})) // known: no answer
	ts.step(ts.after(time.Minute))
	if len(ts.sent) != 2 {
		t.Fatal("answered a known station")
	}
}

func TestAtMostOneAnswerIn30Seconds(t *testing.T) {
	ts := newTestService(t, DefaultEvery)
	ts.step(ts.after(5 * time.Second)) // the first beacon
	sentAt := ts.lastSent
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24"), Ask: true}))
	ts.step(sentAt.Add(30*time.Second - time.Millisecond))
	if len(ts.sent) != 1 {
		t.Fatal("answered within 30 s of the last beacon")
	}
	ts.step(sentAt.Add(30 * time.Second))
	if len(ts.sent) != 2 {
		t.Fatal("the postponed answer did not come")
	}
}

func TestEveryBeaconAsksUntilTheFirstHasGone(t *testing.T) {
	ts := newTestService(t, DefaultEvery)
	// Another station's ask arrives before our own first beacon.
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24"), Ask: true}))
	ts.step(ts.after(10 * time.Second))
	if len(ts.sent) != 1 || !ts.sent[0].Ask {
		t.Fatalf("sent %+v", ts.sent)
	}
}

func TestOwnCallsign(t *testing.T) {
	ts := newTestService(t, DefaultEvery)
	ts.Handle(beaconFrom(t, "TESTA-1", Beacon{Addr: addr("10.73.0.1/24"), Tags: []string{"AUTO"}, Ask: true}))
	if ts.log.Len() != 0 || len(ts.Stations()) != 0 {
		t.Fatalf("an echo of our own beacon: log %s", ts.log.String())
	}
	ts.Handle(beaconFrom(t, "TESTA-1", Beacon{Addr: addr("10.73.0.9/24")}))
	if !strings.Contains(ts.log.String(), "another station sends beacons with our callsign") {
		t.Fatalf("log %s", ts.log.String())
	}
}

func TestOwnAddressIsWarned(t *testing.T) {
	ts := newTestService(t, 0)
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.1/24")}))
	if !strings.Contains(ts.log.String(), "a station announces our own address") {
		t.Fatalf("log %s", ts.log.String())
	}
}

func TestOtherFramesAreLeftToTheCaller(t *testing.T) {
	ts := newTestService(t, 0)
	ip, _ := ax25.NewUI(ax25.MustAddress("TESTA-1"), ax25.MustAddress("TESTB-1"), nil, ax25.PIDIPv4, []byte{0x45}).Encode()
	if ts.Handle(ip) || ts.Handle([]byte{1, 2, 3}) {
		t.Fatal("took a non-beacon")
	}
	bad, _ := ax25.NewUI(Dest, ax25.MustAddress("TESTB-1"), nil, ax25.PIDNoL3, []byte("AXL1 ip=nonsense")).Encode()
	relayed, _ := ax25.NewUI(Dest, ax25.MustAddress("TESTB-1"), []ax25.Address{ax25.MustAddress("WIDE1-1")}, ax25.PIDNoL3, []byte("AXL1 ip=10.73.0.2/24")).Encode()
	if !ts.Handle(bad) || !ts.Handle(relayed) {
		t.Fatal("a beacon frame must be taken even if it is not used")
	}
	if s := ts.Stats(); s.Invalid != 1 || s.Relayed != 1 || s.Heard != 0 || len(ts.Stations()) != 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestListenOnlySendsNothing(t *testing.T) {
	ts := newTestService(t, 0)
	ts.Handle(beaconFrom(t, "TESTB-1", Beacon{Addr: addr("10.73.0.2/24"), Ask: true}))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := ts.Run(ctx); err == nil || len(ts.sent) != 0 {
		t.Fatalf("Run %v, sent %d", err, len(ts.sent))
	}
}

func TestRunSendsBeaconsInRealTime(t *testing.T) {
	saved := firstDelay
	firstDelay = [2]time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	defer func() { firstDelay = saved }()
	got := make(chan []byte, 4)
	s := New(Config{Call: ax25.MustAddress("TESTA-1"), Addr: addr("10.73.0.1/24"), Every: DefaultEvery,
		Send: func(f []byte) bool { got <- f; return true }})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	select {
	case f := <-got:
		if !bytes.Contains(f, []byte("AXL1 ip=10.73.0.1/24 ask=1")) {
			t.Fatalf("% X", f)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no beacon")
	}
}

func TestAChangedAnnouncementGoesOutSoon(t *testing.T) {
	ts := newTestService(t, DefaultEvery)
	ts.step(ts.after(time.Minute)) // the first beacon is out
	changed := ts.now
	if err := ts.SetAnnounce([]string{"INET"}, 8080); err != nil {
		t.Fatal(err)
	}
	ts.step(changed.Add(30 * time.Second)) // at most one answer in 30 s still applies
	if len(ts.sent) != 2 || ts.sent[1].Proxy != 8080 || ts.sent[1].Tags[0] != "INET" {
		t.Fatalf("sent %+v", ts.sent)
	}
	if tags, proxy := ts.Announce(); proxy != 8080 || len(tags) != 1 {
		t.Fatalf("%v %v", tags, proxy)
	}
	if ts.SetAnnounce([]string{"not a tag"}, 0) == nil {
		t.Fatal("a bad tag was accepted")
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package txqueue

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time            { return c.t }
func (c *fakeClock) add(d time.Duration)       { c.t = c.t.Add(d) }
func newClock() *fakeClock                     { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }
func frame(name string) []byte                 { return []byte(name) }
func urgent(flow string) Meta                  { return Meta{Urgent: true, Flow: flow} }
func ordinary(flow string) Meta                { return Meta{Flow: flow} }
func keyed(replace, repeat string) Meta        { return Meta{Replace: replace, Repeat: repeat} }
func newQueue(c *fakeClock, cfg Config) *Queue { cfg.Now = c.now; return New(cfg) }

// order pops everything and returns the frame names.
func order(t *testing.T, q *Queue) []string {
	t.Helper()
	var out []string
	for q.Len() > 0 {
		it, err := q.Pop(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(it.Frame))
	}
	return out
}

func expect(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
}

func TestUrgentFramesGoFirst(t *testing.T) {
	q := newQueue(newClock(), Config{})
	q.Push(frame("data1"), ordinary("a"))
	q.Push(frame("data2"), ordinary("a"))
	q.Push(frame("ping"), urgent("b"))
	expect(t, order(t, q), "ping", "data1", "data2")
}

func TestOrdinaryFrameWaitsBehindAtMostUrgentRun(t *testing.T) {
	q := newQueue(newClock(), Config{})
	q.Push(frame("data"), ordinary("a"))
	for _, n := range []string{"u1", "u2", "u3", "u4", "u5"} {
		q.Push(frame(n), urgent("b"))
	}
	expect(t, order(t, q), "u1", "u2", "u3", "data", "u4", "u5")
}

func TestUrgentFrameDoesNotOvertakeItsOwnFlow(t *testing.T) {
	q := newQueue(newClock(), Config{})
	q.Push(frame("data"), ordinary("tcp1"))
	q.Push(frame("ack1"), urgent("tcp1")) // same flow: stays behind the data
	q.Push(frame("ping"), urgent("icmp"))
	expect(t, order(t, q), "ping", "data", "ack1")
}

func TestReplaceKeepsThePlaceAndTakesTheNewestFrame(t *testing.T) {
	q := newQueue(newClock(), Config{})
	q.Push(frame("seg-old"), keyed("seg", ""))
	q.Push(frame("other"), Meta{})
	if r := q.Push(frame("seg-new"), keyed("seg", "")); r != Replaced {
		t.Fatalf("result %v", r)
	}
	expect(t, order(t, q), "seg-new", "other")
	if s := q.Stats(); s.Replaced != 1 || s.Queued != 2 {
		t.Fatalf("stats %+v", s)
	}
}

func TestRepeatWithinTheWindowIsDropped(t *testing.T) {
	c := newClock()
	q := newQueue(c, Config{RepeatWindow: func([]byte) time.Duration { return 2 * time.Second }})
	q.Push(frame("syn"), keyed("syn", "syn"))
	expect(t, order(t, q), "syn") // goes out now
	c.add(time.Second)
	if r := q.Push(frame("syn"), keyed("syn", "syn")); r != Repeat {
		t.Fatalf("1 s after: %v, want Repeat", r)
	}
	c.add(1500 * time.Millisecond)
	if r := q.Push(frame("syn"), keyed("syn", "syn")); r != Queued {
		t.Fatalf("2.5 s after: %v, want Queued", r)
	}
	if s := q.Stats(); s.Repeats != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func TestWithoutRepeatWindowNothingIsDroppedAsRepeat(t *testing.T) {
	q := newQueue(newClock(), Config{})
	q.Push(frame("syn"), keyed("", "syn"))
	order(t, q)
	if r := q.Push(frame("syn"), keyed("", "syn")); r != Queued {
		t.Fatalf("%v", r)
	}
}

func TestStaleFramesAreDropped(t *testing.T) {
	c := newClock()
	q := newQueue(c, Config{MaxAge: time.Second})
	q.Push(frame("old"), Meta{})
	c.add(2 * time.Second)
	q.Push(frame("new"), Meta{})
	expect(t, order(t, q), "new")
	if q.Stats().Stale != 1 {
		t.Fatalf("stats %+v", q.Stats())
	}
}

func TestFullQueueRefuses(t *testing.T) {
	q := newQueue(newClock(), Config{Len: 2})
	q.Push(frame("a"), Meta{})
	q.Push(frame("b"), Meta{})
	if r := q.Push(frame("c"), Meta{}); r != Full {
		t.Fatalf("%v", r)
	}
	// A replacement needs no room.
	q2 := newQueue(newClock(), Config{Len: 1})
	q2.Push(frame("a"), keyed("k", ""))
	if r := q2.Push(frame("a2"), keyed("k", "")); r != Replaced {
		t.Fatalf("%v", r)
	}
}

func TestPopWaitsForAFrameOrTheContext(t *testing.T) {
	q := New(Config{})
	got := make(chan string, 1)
	go func() {
		it, err := q.Pop(context.Background())
		if err == nil {
			got <- string(it.Frame)
		}
	}()
	time.Sleep(20 * time.Millisecond)
	q.Push(frame("late"), Meta{})
	select {
	case f := <-got:
		if f != "late" {
			t.Fatalf("got %q", f)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Pop did not wake up")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := q.Pop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
}

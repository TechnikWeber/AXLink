// SPDX-License-Identifier: GPL-3.0-or-later

package monitor

import (
	"context"
	"testing"
	"time"
)

func TestSamplesAreRatesAndShares(t *testing.T) {
	var cur Totals
	m := New(2*time.Second, 3, func() Totals { return cur })
	start := time.Unix(1_800_000_000, 0)
	m.lastT = start
	cur = Totals{BytesIn: 200, BytesOut: 100, AirOwn: time.Second, AirHeard: 500 * time.Millisecond, Queue: 4}
	m.take(start.Add(2*time.Second), cur)
	s := m.Samples()
	if len(s) != 1 || s[0].In != 100 || s[0].Out != 50 || s[0].Own != 0.5 || s[0].Heard != 0.25 || s[0].Queue != 4 {
		t.Fatalf("%+v", s)
	}
}

func TestRingKeepsTheNewestOldestFirst(t *testing.T) {
	var cur Totals
	m := New(time.Second, 3, func() Totals { return cur })
	start := time.Unix(1_800_000_000, 0)
	m.lastT = start
	for i := 1; i <= 5; i++ {
		cur.BytesIn += uint64(i)
		m.take(start.Add(time.Duration(i)*time.Second), cur)
	}
	s := m.Samples()
	if len(s) != 3 || s[0].In != 3 || s[1].In != 4 || s[2].In != 5 {
		t.Fatalf("%+v", s)
	}
}

func TestSharesStayWithinZeroAndOne(t *testing.T) {
	var cur Totals
	m := New(time.Second, 2, func() Totals { return cur })
	start := time.Unix(1_800_000_000, 0)
	m.lastT = start
	cur.AirHeard = 5 * time.Second // an estimate can exceed the interval
	m.take(start.Add(time.Second), cur)
	if s := m.Samples(); s[0].Heard != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestRunSamplesInRealTime(t *testing.T) {
	n := 0
	m := New(10*time.Millisecond, 10, func() Totals { n += 10; return Totals{BytesIn: uint64(n)} })
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	m.Run(ctx)
	if s := m.Samples(); len(s) < 3 || s[len(s)-1].In <= 0 {
		t.Fatalf("%+v", s)
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

// Package monitor samples the radio traffic at a fixed interval for the live
// view of the web interface: bytes per second in both directions, the busy
// share of the channel and the transmit queue. It keeps a few minutes in a
// ring; a sample costs a few dozen bytes.
package monitor

import (
	"context"
	"sync"
	"time"
)

// Totals are the running counters the monitor reads.
type Totals struct {
	BytesIn, BytesOut uint64        // frame bytes from and to the radio
	AirOwn, AirHeard  time.Duration // airtime of frames sent and heard
	Queue             int           // frames waiting now
}

// Sample is one interval.
type Sample struct {
	T     int64   `json:"t"`     // Unix seconds at the end of the interval
	In    float64 `json:"in"`    // bytes per second from the radio
	Out   float64 `json:"out"`   // bytes per second to the radio
	Own   float64 `json:"own"`   // share of the interval this station transmitted, 0..1
	Heard float64 `json:"heard"` // share it heard others, 0..1
	Queue int     `json:"queue"` // frames waiting at the end
}

// Monitor keeps the last samples.
type Monitor struct {
	interval time.Duration
	read     func() Totals

	mu    sync.Mutex
	ring  []Sample
	next  int
	full  bool
	last  Totals
	lastT time.Time
}

// New returns a monitor that reads every interval and keeps size samples.
func New(interval time.Duration, size int, read func() Totals) *Monitor {
	return &Monitor{interval: interval, read: read, ring: make([]Sample, size)}
}

// Interval is the time between two samples.
func (m *Monitor) Interval() time.Duration { return m.interval }

// Run samples until ctx ends.
func (m *Monitor) Run(ctx context.Context) {
	m.mu.Lock()
	m.last, m.lastT = m.read(), time.Now()
	m.mu.Unlock()
	t := time.NewTicker(m.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			m.take(now, m.read())
		}
	}
}

// take records the interval that ends at now with the totals read then.
func (m *Monitor) take(now time.Time, cur Totals) {
	m.mu.Lock()
	defer m.mu.Unlock()
	secs := now.Sub(m.lastT).Seconds()
	if secs <= 0 {
		return
	}
	share := func(d time.Duration) float64 { return min(1, max(0, d.Seconds()/secs)) }
	s := Sample{
		T:     now.Unix(),
		In:    float64(cur.BytesIn-m.last.BytesIn) / secs,
		Out:   float64(cur.BytesOut-m.last.BytesOut) / secs,
		Own:   share(cur.AirOwn - m.last.AirOwn),
		Heard: share(cur.AirHeard - m.last.AirHeard),
		Queue: cur.Queue,
	}
	m.ring[m.next] = s
	m.next = (m.next + 1) % len(m.ring)
	m.full = m.full || m.next == 0
	m.last, m.lastT = cur, now
}

// Samples returns the samples kept, oldest first.
func (m *Monitor) Samples() []Sample {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.full {
		return append([]Sample(nil), m.ring[:m.next]...)
	}
	return append(append([]Sample(nil), m.ring[m.next:]...), m.ring[:m.next]...)
}

// SPDX-License-Identifier: GPL-3.0-or-later

// Package txqueue holds the frames that wait for the radio. At 1200 bit/s a
// full frame is two seconds on the air, so the order matters:
//
//   - Urgent (small) frames such as TCP acknowledgements, connection set-up,
//     pings and DNS go before ordinary ones. An ordinary frame waits behind at
//     most UrgentRun urgent ones, and an urgent frame never overtakes an
//     ordinary one of its own flow.
//   - A frame takes the place of a waiting one with the same Replace key: a
//     TCP retransmission that of its original, a newer pure ACK that of an
//     older one.
//   - A frame is dropped if one with the same Repeat key went to the TNC
//     within the repeat window: TCP repeats a SYN after one second, while the
//     answer over the radio takes about two.
package txqueue

import (
	"context"
	"sync"
	"time"
)

// Meta describes a frame for the queue. The zero value is an ordinary frame.
type Meta struct {
	Urgent  bool   // goes before ordinary frames
	Flow    string // frames of one flow keep their order; "" for none
	Replace string // a newer frame with this key replaces a waiting one
	Repeat  string // dropped if a frame with this key just went out
}

// Config sets up a queue.
type Config struct {
	Len    int           // frames that may wait; 0 means 32
	MaxAge time.Duration // a frame waiting longer is dropped; 0 means 60 s
	// UrgentRun is how many urgent frames may go in a row while an ordinary
	// one waits; 0 means 3.
	UrgentRun int
	// RepeatWindow tells how long after a frame went out a repeat of it is
	// dropped; nil drops no repeats.
	RepeatWindow func(frame []byte) time.Duration
	Now          func() time.Time // nil means time.Now
}

// Result tells what Push did with a frame.
type Result int

const (
	Queued   Result = iota
	Replaced        // it took the place of a waiting frame with the same key
	Repeat          // dropped: the same frame has just gone out
	Full            // dropped: the queue is full
)

// Stats counts what the queue did.
type Stats struct {
	Queued, Replaced, Repeats, Full, Stale uint64
}

// Item is a frame taken from the queue.
type Item struct {
	Frame []byte
	At    time.Time // when it was queued or last replaced
}

type entry struct {
	frame []byte
	m     Meta
	at    time.Time
}

type sentAt struct {
	at     time.Time
	window time.Duration
}

// Queue is safe for use by several goroutines.
type Queue struct {
	cfg    Config
	notify chan struct{}

	mu             sync.Mutex
	urgent, normal []entry
	run            int // urgent frames taken in a row while ordinary ones waited
	sent           map[string]sentAt
	stats          Stats
}

// New returns an empty queue.
func New(cfg Config) *Queue {
	if cfg.Len <= 0 {
		cfg.Len = 32
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 60 * time.Second
	}
	if cfg.UrgentRun <= 0 {
		cfg.UrgentRun = 3
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Queue{cfg: cfg, notify: make(chan struct{}, 1), sent: map[string]sentAt{}}
}

// Push adds a frame. The queue keeps the slice; the caller must not change it
// afterwards.
func (q *Queue) Push(frame []byte, m Meta) Result {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.cfg.Now()
	if m.Repeat != "" {
		if s, ok := q.sent[m.Repeat]; ok && now.Before(s.at.Add(s.window)) {
			q.stats.Repeats++
			return Repeat
		}
	}
	if m.Replace != "" {
		for _, list := range [][]entry{q.urgent, q.normal} {
			for i := range list {
				if list[i].m.Replace == m.Replace {
					list[i] = entry{frame: frame, m: m, at: now} // same place, newest content
					q.stats.Replaced++
					return Replaced
				}
			}
		}
	}
	if len(q.urgent)+len(q.normal) >= q.cfg.Len {
		q.stats.Full++
		return Full
	}
	e := entry{frame: frame, m: m, at: now}
	if m.Urgent && !q.ordinaryWaits(m.Flow) {
		q.urgent = append(q.urgent, e)
	} else {
		q.normal = append(q.normal, e)
	}
	q.stats.Queued++
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return Queued
}

func (q *Queue) ordinaryWaits(flow string) bool {
	if flow == "" {
		return false
	}
	for _, e := range q.normal {
		if e.m.Flow == flow {
			return true
		}
	}
	return false
}

// Pop takes the next frame and waits for one if the queue is empty. Frames
// older than MaxAge are dropped on the way.
func (q *Queue) Pop(ctx context.Context) (Item, error) {
	for {
		if it, ok := q.take(); ok {
			return it, nil
		}
		select {
		case <-q.notify:
		case <-ctx.Done():
			return Item{}, ctx.Err()
		}
	}
}

func (q *Queue) take() (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.cfg.Now()
	for len(q.urgent)+len(q.normal) > 0 {
		var e entry
		if len(q.urgent) > 0 && (len(q.normal) == 0 || q.run < q.cfg.UrgentRun) {
			e = q.urgent[0]
			q.urgent[0] = entry{}
			q.urgent = q.urgent[1:]
			if len(q.normal) > 0 {
				q.run++
			} else {
				q.run = 0
			}
		} else {
			e = q.normal[0]
			q.normal[0] = entry{}
			q.normal = q.normal[1:]
			q.run = 0
		}
		if now.Sub(e.at) > q.cfg.MaxAge {
			q.stats.Stale++
			continue
		}
		if e.m.Repeat != "" && q.cfg.RepeatWindow != nil {
			q.remember(e.m.Repeat, now, q.cfg.RepeatWindow(e.frame))
		}
		return Item{Frame: e.frame, At: e.at}, true
	}
	return Item{}, false
}

func (q *Queue) remember(key string, now time.Time, window time.Duration) {
	if len(q.sent) >= 64 {
		for k, s := range q.sent {
			if !now.Before(s.at.Add(s.window)) {
				delete(q.sent, k)
			}
		}
	}
	q.sent[key] = sentAt{at: now, window: window}
}

// Len returns how many frames wait.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.urgent) + len(q.normal)
}

// Stats returns a snapshot of the counters.
func (q *Queue) Stats() Stats {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.stats
}

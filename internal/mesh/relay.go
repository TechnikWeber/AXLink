// SPDX-License-Identifier: GPL-3.0-or-later

// Package mesh lets stations reach others beyond their own range. Every
// station repeats frames for others as an AX.25 digipeater, on the same
// frequency, unless its operator switches that off (the role RELAY):
//
//   - a frame that names the relay as its next digipeater (TESTC-1 via
//     TESTB-1) is repeated as it is, with the relay marked as used, as every
//     digipeater does;
//   - a beacon or a message to CQ with the alias AXHOP-n (n hops left) is
//     repeated for everyone, with the relay's callsign inserted before the
//     alias and n lowered by one, as APRS digipeaters do with WIDEn-N. The
//     path then shows who repeated it, so the receivers learn the way back.
//
// So that not every station repeats every beacon, a relay repeats a flooded
// frame only while a station it hears directly may not have it yet: one that
// neither sent it nor repeated it. It waits a random moment first; copies it
// hears meanwhile from other relays count too, and when they leave nobody of
// its neighbours out, it stays silent. A relay that knows no neighbour yet
// (it has just started, or their beacons collided) repeats, and a frame it
// did not repeat is weighed again when it comes again: the sender repeats a
// beacon that no relay passed on.
//
// Stations learn from relayed beacons over which relays another station is
// reached, and address their frames to it over that path (see package
// discovery). Only frames without a network layer (PID 0xF0) are flooded;
// IP packets always name their relays. A relay forwards on the radio only:
// what it repeats never reaches its own computer or network.
package mesh

import (
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// FloodHops is how far beacons and messages to CQ travel at most: over two
// relays.
const FloodHops = 2

// Flood is the alias that asks relays to pass a frame on to every station.
var Flood = ax25.Address{Call: "AXHOP", SSID: FloodHops}

// The time in which the same frame is not repeated twice: as APRS
// digipeaters check, for frames reaching the relay on two ways, and a
// shorter one for frames that name the relay. floodWait is the random wait
// before a flooded frame goes out.
var (
	floodDup  = 30 * time.Second
	directDup = 5 * time.Second
	floodWait = [2]time.Duration{500 * time.Millisecond, 2500 * time.Millisecond}
)

// Config sets up a Relay.
type Config struct {
	Call ax25.Address
	// Send hands a frame to repeat to the radio.
	Send func(frame []byte)
	// Neighbors lists the stations heard directly (by their beacons).
	Neighbors func() []ax25.Address
	Now       func() time.Time                // nil: time.Now
	After     func(d time.Duration, f func()) // nil: time.AfterFunc
	Rand      func(n int64) int64             // a number in [0, n); nil: math/rand/v2
}

// Stats counts what a relay did.
type Stats struct {
	Repeated   uint64 // frames that named this relay
	Flooded    uint64 // beacons and CQ messages passed on
	Duplicates uint64 // heard again, not repeated again
	Covered    uint64 // not passed on: every neighbour had them already
}

// Relay decides which frames this station repeats. Its methods may be
// called from any goroutine.
type Relay struct {
	cfg Config
	on  atomic.Bool

	mu      sync.Mutex
	seen    map[uint64]time.Time
	pending map[uint64]*waiting
	stats   Stats
}

// waiting is a flooded frame in its random wait.
type waiting struct {
	frame   []byte
	covered []ax25.Address // stations known to have it
}

// NewRelay returns a relay, switched off.
func NewRelay(cfg Config) *Relay {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.After == nil {
		cfg.After = func(d time.Duration, f func()) { time.AfterFunc(d, f) }
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Int64N
	}
	cfg.Call.Bit7 = false
	return &Relay{cfg: cfg, seen: map[uint64]time.Time{}, pending: map[uint64]*waiting{}}
}

// SetOn switches repeating on or off.
func (r *Relay) SetOn(on bool) { r.on.Store(on) }

// On reports whether the relay repeats.
func (r *Relay) On() bool { return r.on.Load() }

// Handle takes a frame heard on the radio and repeats it if it should.
func (r *Relay) Handle(frame []byte) {
	if !r.on.Load() {
		return
	}
	f, err := ax25.Decode(frame)
	if err != nil || len(f.Path) == 0 {
		return
	}
	src := f.Src
	src.Bit7 = false
	if src.SameStation(r.cfg.Call) {
		return
	}
	key := frameKey(f)
	covered := append([]ax25.Address{src}, f.Repeaters()...)
	r.mu.Lock()
	if w, ok := r.pending[key]; ok { // another copy while we wait: who has it now
		w.covered = append(w.covered, covered...)
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	i := slices.IndexFunc(f.Path, func(d ax25.Address) bool { return !d.Bit7 })
	if i < 0 {
		return
	}
	next := f.Path[i]
	switch {
	case next.SameStation(r.cfg.Call):
		if r.seenWithin(key, directDup) {
			return
		}
		r.mark(key)
		f.Path[i].Bit7 = true
		out, err := f.Encode()
		if err != nil {
			return
		}
		r.count(func(s *Stats) { s.Repeated++ })
		r.cfg.Send(out)
	case next.Call == Flood.Call && next.SSID >= 1 && f.IsUI() && f.PID == ax25.PIDNoL3:
		// A copy straight from its sender comes again when the sender sends
		// the beacon again (no relay's copy reached it): weigh it anew then.
		window := floodDup
		if len(covered) == 1 {
			window = directDup
		}
		if len(f.Path) >= ax25.MaxRepeaters || slices.ContainsFunc(f.Path, r.cfg.Call.SameStation) ||
			r.seenWithin(key, window) {
			return
		}
		if !r.missing(covered) {
			r.count(func(s *Stats) { s.Covered++ })
			return
		}
		r.mark(key)
		me := r.cfg.Call
		me.Bit7 = true
		next.SSID--
		next.Bit7 = next.SSID == 0
		f.Path = slices.Concat(f.Path[:i], []ax25.Address{me, next}, f.Path[i+1:])
		out, err := f.Encode()
		if err != nil {
			return
		}
		r.mu.Lock()
		r.pending[key] = &waiting{frame: out, covered: covered}
		r.mu.Unlock()
		wait := floodWait[0] + time.Duration(r.cfg.Rand(int64(floodWait[1]-floodWait[0])+1))
		r.cfg.After(wait, func() { r.flood(key) })
	}
}

// flood sends a flooded frame after its wait, unless the copies heard
// meanwhile reached every neighbour.
func (r *Relay) flood(key uint64) {
	r.mu.Lock()
	w := r.pending[key]
	delete(r.pending, key)
	r.mu.Unlock()
	if w == nil {
		return
	}
	if !r.missing(w.covered) {
		r.count(func(s *Stats) { s.Covered++ })
		return
	}
	r.count(func(s *Stats) { s.Flooded++ })
	r.cfg.Send(w.frame)
}

// missing reports whether a station heard directly may not have the frame:
// it is neither its sender nor one of the stations that repeated it. With no
// neighbour known at all, any may be missing.
func (r *Relay) missing(covered []ax25.Address) bool {
	if r.cfg.Neighbors == nil {
		return true
	}
	neighbors := r.cfg.Neighbors()
	if len(neighbors) == 0 {
		return true
	}
	for _, n := range neighbors {
		if !n.SameStation(r.cfg.Call) && !slices.ContainsFunc(covered, n.SameStation) {
			return true
		}
	}
	return false
}

func (r *Relay) count(f func(*Stats)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f(&r.stats)
}

// frameKey names a frame by its source, destination, PID and information;
// the path changes on the way.
func frameKey(f *ax25.Frame) uint64 {
	h := fnv.New64a()
	h.Write([]byte(f.Src.String() + ">" + f.Dest.String()))
	h.Write([]byte{f.PID})
	h.Write(f.Info)
	return h.Sum64()
}

// seenWithin reports whether the frame was repeated within window.
func (r *Relay) seenWithin(key uint64, window time.Duration) bool {
	now := r.cfg.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.seen[key]; ok && now.Sub(t) < window {
		r.stats.Duplicates++
		return true
	}
	return false
}

// mark notes the frame as repeated (or about to be) now.
func (r *Relay) mark(key uint64) {
	now := r.cfg.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.seen) >= 1024 {
		for k, t := range r.seen {
			if now.Sub(t) >= floodDup {
				delete(r.seen, k)
			}
		}
	}
	r.seen[key] = now
}

// Stats returns a snapshot of the counters.
func (r *Relay) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

// SPDX-License-Identifier: GPL-3.0-or-later

// Package mesh lets stations reach others beyond their own range. A station
// whose operator makes it a relay (the role RELAY) repeats frames as an AX.25
// digipeater, on the same frequency:
//
//   - a frame that names the relay as its next digipeater (TESTC-1 via
//     TESTB-1) is repeated as it is, with the relay marked as used, as every
//     digipeater does;
//   - a beacon or a message to CQ with the alias AXHOP-n (n hops left) is
//     repeated for everyone, with the relay's callsign inserted before the
//     alias and n lowered by one, as APRS digipeaters do with WIDEn-N. The
//     path then shows who repeated it, so the receivers learn the way back.
//
// Stations learn from relayed beacons over which relays another station is
// reached, and address their frames to it over that path (see package
// discovery). Only frames without a network layer (PID 0xF0) are flooded;
// IP packets always name their relays. A relay forwards on the radio only:
// what it repeats never reaches its own computer or network.
package mesh

import (
	"hash/fnv"
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
// shorter one for frames that name the relay.
var (
	floodDup  = 30 * time.Second
	directDup = 5 * time.Second
)

// Stats counts what a relay did.
type Stats struct {
	Repeated   uint64 // frames that named this relay
	Flooded    uint64 // beacons and CQ messages passed on
	Duplicates uint64 // not repeated again
}

// Relay decides which frames this station repeats. Its methods may be
// called from any goroutine.
type Relay struct {
	call ax25.Address
	on   atomic.Bool
	now  func() time.Time

	mu    sync.Mutex
	seen  map[uint64]time.Time
	stats Stats
}

// NewRelay returns a relay for call, switched off; now nil means time.Now.
func NewRelay(call ax25.Address, now func() time.Time) *Relay {
	if now == nil {
		now = time.Now
	}
	call.Bit7 = false
	return &Relay{call: call, now: now, seen: map[uint64]time.Time{}}
}

// SetOn switches repeating on or off.
func (r *Relay) SetOn(on bool) { r.on.Store(on) }

// On reports whether the relay repeats.
func (r *Relay) On() bool { return r.on.Load() }

// Handle takes a frame heard on the radio and returns the frame to send for
// it, or nil.
func (r *Relay) Handle(frame []byte) []byte {
	if !r.on.Load() {
		return nil
	}
	f, err := ax25.Decode(frame)
	if err != nil || len(f.Path) == 0 || f.Src.SameStation(r.call) {
		return nil
	}
	i := slices.IndexFunc(f.Path, func(d ax25.Address) bool { return !d.Bit7 })
	if i < 0 {
		return nil
	}
	next := f.Path[i]
	window := directDup
	switch {
	case next.SameStation(r.call):
		f.Path[i].Bit7 = true
	case next.Call == Flood.Call && next.SSID >= 1 && f.IsUI() && f.PID == ax25.PIDNoL3:
		if len(f.Path) >= ax25.MaxRepeaters || slices.ContainsFunc(f.Path, r.call.SameStation) {
			return nil
		}
		me := r.call
		me.Bit7 = true
		next.SSID--
		next.Bit7 = next.SSID == 0
		f.Path = slices.Concat(f.Path[:i], []ax25.Address{me, next}, f.Path[i+1:])
		window = floodDup
	default:
		return nil
	}
	if r.duplicate(f, window) {
		return nil
	}
	out, err := f.Encode()
	if err != nil {
		return nil
	}
	r.mu.Lock()
	if window == floodDup {
		r.stats.Flooded++
	} else {
		r.stats.Repeated++
	}
	r.mu.Unlock()
	return out
}

// duplicate reports whether the same frame (source, destination, PID and
// information; the path changes on the way) was repeated within window.
func (r *Relay) duplicate(f *ax25.Frame, window time.Duration) bool {
	h := fnv.New64a()
	h.Write([]byte(f.Src.String() + ">" + f.Dest.String()))
	h.Write([]byte{f.PID})
	h.Write(f.Info)
	key := h.Sum64()
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.seen[key]; ok && now.Sub(t) < window {
		r.stats.Duplicates++
		return true
	}
	if len(r.seen) >= 1024 {
		for k, t := range r.seen {
			if now.Sub(t) >= floodDup {
				delete(r.seen, k)
			}
		}
	}
	r.seen[key] = now
	return false
}

// Stats returns a snapshot of the counters.
func (r *Relay) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

// SPDX-License-Identifier: GPL-3.0-or-later

// Package pacing plans how AXLink hands frames to a KISS TNC so that TNCs
// with small buffers do not lose them. A plan is pure data (write times and
// bytes); the serial driver executes it and the simulator checks it.
package pacing

import (
	"time"

	"github.com/TechnikWeber/AXLink/internal/airtime"
	"github.com/TechnikWeber/AXLink/internal/kiss"
)

// Profile describes how one kind of TNC wants to be fed.
type Profile struct {
	Name string

	// ChunkSize limits one serial write; 0 writes whole frames.
	ChunkSize int
	// ChunkGap is the pause between two chunks of one frame.
	ChunkGap time.Duration
	// OneInFlight hands the next frame over only after the TNC has sent the
	// previous one. Without AckMode the end of the transmission is estimated.
	OneInFlight bool
	// AckMode: the TNC echoes KISS ACKMODE tags when a frame is sent, and
	// the host waits for the echo instead of estimating.
	AckMode bool

	// Resync is sent first whenever the port opens. For the F4HWN app it is
	// an invalid escape and a FEND: that aborts a frame cut off by a USB
	// reset, which the app would otherwise close and send truncated.
	Resync []byte

	// KISS parameters sent when the port opens; 0 leaves the TNC's value.
	TxDelayParam byte // KISS TXDELAY in 10 ms units
	PersistParam byte // KISS PERSIST

	// TNC timing, for the estimate without AckMode.
	Baud         int
	TxDelayFlags int           // flags the TNC sends before a frame
	TailFlags    int           // flags after the frame
	PollEvery    time.Duration // how often the TNC reads from USB
	ReadChunk    int           // bytes per read
	SlotTime     time.Duration // channel-access slot of the TNC
	Margin       time.Duration
}

// Generic hands whole frames to the TNC and relies on its queue (Dire Wolf,
// most hardware TNCs).
var Generic = Profile{Name: "generic-kiss"}

// F4HWNKissTNC is the KISS TNC app of the F4HWN firmware as it is today
// (v6.3.0): one frame in flight, writes the app can drain at 1.6 kB/s, and
// PERSIST 255 so that a frame goes out in the first clear slot. Channel-access
// randomisation then belongs to the host (a backoff before Plan).
var F4HWNKissTNC = Profile{
	Name:         "f4hwn-kisstnc",
	ChunkSize:    64,
	ChunkGap:     40 * time.Millisecond,
	OneInFlight:  true,
	Resync:       []byte{kiss.FESC, 0x00, kiss.FEND},
	TxDelayParam: 27, // the app sends 27 + 27/2 = 40 flags (267 ms)
	PersistParam: 255,
	Baud:         1200,
	TxDelayFlags: 40,
	TailFlags:    3,
	PollEvery:    10 * time.Millisecond,
	ReadChunk:    16,
	SlotTime:     50 * time.Millisecond,
	Margin:       100 * time.Millisecond,
}

// F4HWNKissTNCAck is the same app with the proposed ACKMODE extension. The
// host knows when a frame has gone out, so the app may randomise channel
// access again (PERSIST 63, the app's default: a quarter of the 50 ms slots).
// With 255 two stations waiting for the same busy channel would both start
// in the first clear slot and collide.
var F4HWNKissTNCAck = func() Profile {
	p := F4HWNKissTNC
	p.Name = "f4hwn-kisstnc-ackmode"
	p.AckMode = true
	p.PersistParam = 63
	return p
}()

// Profiles lists the known profiles by name.
func Profiles() []Profile { return []Profile{F4HWNKissTNC, F4HWNKissTNCAck, Generic} }

// ByName returns the profile with this name.
func ByName(name string) (Profile, bool) {
	for _, p := range Profiles() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// Init returns the parameter frames to send when the port opens.
func (p Profile) Init(port uint8) []kiss.Frame {
	var frames []kiss.Frame
	if p.TxDelayParam != 0 {
		frames = append(frames, kiss.Param(port, kiss.CmdTxDelay, p.TxDelayParam))
	}
	if p.PersistParam != 0 {
		frames = append(frames, kiss.Param(port, kiss.CmdPersist, p.PersistParam))
	}
	return frames
}

// Write is one serial write at a time offset.
type Write struct {
	At   time.Duration
	Data []byte
}

// Plan is the schedule for one frame.
type Plan struct {
	Writes []Write
	// Done is when the next frame may start. With OneInFlight and without
	// AckMode it is the estimated end of the transmission plus a margin;
	// with AckMode the host waits for the echo instead.
	Done time.Duration
	// TxStart is the earliest moment the TNC can start sending (it has read
	// the whole frame), Airtime the frame's time on the air. Both are set
	// only with OneInFlight and without AckMode.
	TxStart, Airtime time.Duration
}

// DoneAfterHeard returns when the next frame may start if the radio heard a
// frame that ended at end while this one could not have been on the air yet:
// the TNC sends only on a clear channel, so this frame went out after it.
func (p Profile) DoneAfterHeard(end time.Duration, plan Plan) time.Duration {
	return end + p.SlotTime + plan.Airtime + p.Margin
}

// Plan schedules one encoded KISS frame that carries ax25 (the AX.25 frame
// without FCS, used for the airtime estimate), starting at start.
func (p Profile) Plan(start time.Duration, encoded, ax25 []byte) Plan {
	size := p.ChunkSize
	if size <= 0 || size > len(encoded) {
		size = len(encoded)
	}
	var plan Plan
	at := start
	for off := 0; off < len(encoded); off += size {
		end := min(off+size, len(encoded))
		plan.Writes = append(plan.Writes, Write{At: at, Data: encoded[off:end]})
		at += p.ChunkGap
	}
	last := start
	if n := len(plan.Writes); n > 0 {
		last = plan.Writes[n-1].At
	}
	plan.Done = last
	if !p.OneInFlight || p.AckMode {
		return plan
	}
	// When has the TNC read everything? It drains ReadChunk bytes per poll.
	drained := start
	for _, w := range plan.Writes {
		reads := (len(w.Data) + p.ReadChunk - 1) / p.ReadChunk
		drained = max(drained, w.At) + time.Duration(reads)*p.PollEvery
	}
	drained += p.PollEvery // the poll phase is unknown
	tx := airtime.Duration(airtime.Bits(ax25, p.TxDelayFlags, p.TailFlags), p.Baud)
	plan.TxStart, plan.Airtime = drained, tx
	plan.Done = drained + p.SlotTime + tx + p.Margin
	return plan
}

// SPDX-License-Identifier: GPL-3.0-or-later

// Package channel is a frame-level radio channel for the AXLink lab. Each
// station connects over a KISS stream (a Unix socket); what one station sends
// the others hear after its airtime. The channel is half duplex: a station
// that transmits hears nothing, and overlapping transmissions are lost. With
// Links, only the stations of a link hear each other: a station out of range
// neither hears a frame nor sees the channel busy, and two stations that do
// not hear each other collide at a third that hears both (hidden stations).
//
// In F4HWN mode every station's KISS stream first goes through the model of
// the F4HWN KISS TNC app (package f4hwnkiss), so the host must pace exactly
// as it would have to with the radio. Ideal mode is a TNC with a queue.
package channel

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"hash/fnv"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/TechnikWeber/AXLink/internal/airtime"
	"github.com/TechnikWeber/AXLink/internal/kiss"
	"github.com/TechnikWeber/AXLink/internal/sim/f4hwnkiss"
)

// Mode selects the TNC in front of each station.
type Mode int

const (
	Ideal Mode = iota // a TNC that queues and sends every frame
	F4HWN             // the model of the F4HWN KISS TNC app
)

// Config sets up the channel.
type Config struct {
	Mode    Mode
	AckMode bool      // F4HWN mode: the model with the proposed ACKMODE extension
	Trace   io.Writer // JSON lines of transmissions and receptions; may be nil
	Logger  *slog.Logger
	// Links are the pairs of stations (by name) that hear each other; none
	// means every station hears every other.
	Links [][2]string
}

const tick = 5 * time.Millisecond

// Channel connects the stations. Add stations, then call Run once.
type Channel struct {
	cfg      Config
	log      *slog.Logger
	start    time.Time
	inputs   chan input
	stations []*station
	air      []*airFrame
	next     int // round robin in ideal mode
}

type station struct {
	name string
	ln   net.Listener

	mu   sync.Mutex
	conn net.Conn

	// F4HWN mode
	app                                 *f4hwnkiss.App
	seenTx, seenAck, seenDrop, seenLoss int
	// ideal mode
	dec   kiss.Decoder
	queue []pending
}

type pending struct {
	frame []byte
	tag   uint16
	ack   bool
}

type airFrame struct {
	from       *station
	start, end time.Duration
	frame      []byte
	tag        uint16
	ack        bool
	done       bool
}

type input struct {
	st   *station
	data []byte
}

// New returns an empty channel.
func New(cfg Config) *Channel {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Channel{cfg: cfg, log: log, inputs: make(chan input, 256)}
}

// AddStation adds a station that connects through ln.
func (c *Channel) AddStation(name string, ln net.Listener) {
	st := &station{name: name, ln: ln}
	if c.cfg.Mode == F4HWN {
		// The app draws its p-persistence from rand32(); a fixed seed per
		// station keeps runs comparable.
		h := fnv.New64a()
		h.Write([]byte(name))
		r := rand.New(rand.NewPCG(h.Sum64(), 0x41584c))
		st.app = f4hwnkiss.New(f4hwnkiss.Config{
			AckMode: c.cfg.AckMode,
			Busy:    func(t time.Duration) bool { return c.othersOnAir(st, t) },
			Rand:    r.Uint32,
		})
	}
	c.stations = append(c.stations, st)
}

// Run serves the stations until ctx ends.
func (c *Channel) Run(ctx context.Context) error {
	c.start = time.Now()
	for _, st := range c.stations {
		go c.accept(ctx, st)
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, st := range c.stations {
				st.ln.Close()
				st.setConn(nil)
			}
			return ctx.Err()
		case in := <-c.inputs:
			c.receiveFromHost(in.st, in.data)
		case <-t.C:
			c.step(time.Since(c.start))
		}
	}
}

func (c *Channel) now() time.Duration { return time.Since(c.start) }

func (c *Channel) accept(ctx context.Context, st *station) {
	for {
		conn, err := st.ln.Accept()
		if err != nil {
			return
		}
		c.log.Info("station connected", "station", st.name)
		st.setConn(conn)
		go func() {
			buf := make([]byte, 2048)
			for {
				n, err := conn.Read(buf)
				if n > 0 {
					select {
					case c.inputs <- input{st: st, data: append([]byte(nil), buf[:n]...)}:
					case <-ctx.Done():
						return
					}
				}
				if err != nil {
					c.log.Info("station disconnected", "station", st.name)
					return
				}
			}
		}()
	}
}

func (st *station) setConn(conn net.Conn) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.conn != nil {
		st.conn.Close()
	}
	st.conn = conn
}

// toHost writes a KISS frame to the station's host; a station without a host
// simply misses it.
func (st *station) toHost(f kiss.Frame) {
	b, err := kiss.Encode(f)
	if err != nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.conn == nil {
		return
	}
	st.conn.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := st.conn.Write(b); err != nil {
		st.conn.Close()
		st.conn = nil
	}
}

func (c *Channel) receiveFromHost(st *station, data []byte) {
	if st.app != nil {
		st.app.Write(c.now(), data)
		return
	}
	st.dec.Feed(data, func(f kiss.Frame) {
		switch f.Command {
		case kiss.CmdData:
			st.queue = append(st.queue, pending{frame: f.Payload})
		case kiss.CmdAckMode:
			if len(f.Payload) > 2 {
				tag := uint16(f.Payload[0])<<8 | uint16(f.Payload[1])
				st.queue = append(st.queue, pending{frame: f.Payload[2:], tag: tag, ack: true})
			}
		}
	})
}

func (c *Channel) step(now time.Duration) {
	if c.cfg.Mode == F4HWN {
		for _, st := range c.stations {
			st.app.Advance(now)
			tx := st.app.Transmitted()
			for ; st.seenTx < len(tx); st.seenTx++ {
				c.onAir(&airFrame{from: st, start: tx[st.seenTx].Start, end: tx[st.seenTx].End, frame: tx[st.seenTx].Frame})
			}
			acks := st.app.Acks()
			for ; st.seenAck < len(acks); st.seenAck++ {
				tag := acks[st.seenAck].Tag
				st.toHost(kiss.Frame{Command: kiss.CmdAckMode, Payload: []byte{byte(tag >> 8), byte(tag)}})
			}
			// Losses inside the app never reach the air; the trace shows them too.
			for ; st.seenDrop < st.app.DroppedFrames(); st.seenDrop++ {
				c.trace(map[string]any{"t": now.Seconds(), "ev": "drop", "from": st.name, "why": "tnc-busy"})
			}
			losses := st.app.Losses()
			for ; st.seenLoss < len(losses); st.seenLoss++ {
				l := losses[st.seenLoss]
				c.trace(map[string]any{"t": l.At.Seconds(), "ev": "drop", "from": st.name, "why": "usb-ring", "bytes": l.Bytes})
			}
		}
	} else if !c.busy(now) {
		c.startIdealTransmission(now)
	}
	c.deliver(now)
}

func (c *Channel) startIdealTransmission(now time.Duration) {
	for i := range c.stations {
		st := c.stations[(c.next+i)%len(c.stations)]
		if len(st.queue) == 0 {
			continue
		}
		p := st.queue[0]
		st.queue = st.queue[1:]
		end := now + airtime.Duration(airtime.Bits(p.frame, f4hwnkiss.TxDelayFlags, f4hwnkiss.TailFlags), f4hwnkiss.Baud)
		c.onAir(&airFrame{from: st, start: now, end: end, frame: p.frame, tag: p.tag, ack: p.ack})
		c.next = (c.next + i + 1) % len(c.stations)
		return
	}
}

func (c *Channel) onAir(af *airFrame) {
	c.air = append(c.air, af)
	c.trace(map[string]any{"t": af.start.Seconds(), "ev": "tx", "from": af.from.name,
		"dur": (af.end - af.start).Seconds(), "frame": hex.EncodeToString(af.frame)})
}

func (c *Channel) busy(now time.Duration) bool {
	for _, af := range c.air {
		if af.start <= now && now < af.end {
			return true
		}
	}
	return false
}

func (c *Channel) othersOnAir(st *station, t time.Duration) bool {
	for _, af := range c.air {
		if af.from != st && af.start <= t && t < af.end && c.hears(st, af.from) {
			return true
		}
	}
	return false
}

// hears reports whether rx is in range of tx.
func (c *Channel) hears(rx, tx *station) bool {
	if len(c.cfg.Links) == 0 {
		return true
	}
	for _, l := range c.cfg.Links {
		if l[0] == rx.name && l[1] == tx.name || l[1] == rx.name && l[0] == tx.name {
			return true
		}
	}
	return false
}

func (c *Channel) deliver(now time.Duration) {
	kept := c.air[:0]
	for _, af := range c.air {
		if !af.done && af.end <= now {
			af.done = true
			if af.ack {
				af.from.toHost(kiss.Frame{Command: kiss.CmdAckMode, Payload: []byte{byte(af.tag >> 8), byte(af.tag)}})
			}
			for _, rx := range c.stations {
				if rx == af.from || !c.hears(rx, af.from) {
					continue
				}
				if why := c.lostAt(rx, af); why != "" {
					c.trace(map[string]any{"t": now.Seconds(), "ev": "lost", "from": af.from.name, "to": rx.name, "why": why})
					continue
				}
				rx.toHost(kiss.Data(0, af.frame))
				c.trace(map[string]any{"t": now.Seconds(), "ev": "rx", "from": af.from.name, "to": rx.name,
					"frame": hex.EncodeToString(af.frame)})
			}
		}
		if now-af.end < 30*time.Second {
			kept = append(kept, af)
		}
	}
	c.air = kept
}

// lostAt reports why rx cannot hear af: it transmitted itself meanwhile
// (half duplex) or another transmission overlapped (collision).
func (c *Channel) lostAt(rx *station, af *airFrame) string {
	for _, other := range c.air {
		if other == af || other.end <= af.start || other.start >= af.end {
			continue
		}
		if other.from == rx {
			return "half duplex"
		}
		if c.hears(rx, other.from) {
			return "collision"
		}
	}
	return ""
}

func (c *Channel) trace(ev map[string]any) {
	if c.cfg.Trace == nil {
		return
	}
	b, _ := json.Marshal(ev)
	c.cfg.Trace.Write(append(b, '\n'))
}

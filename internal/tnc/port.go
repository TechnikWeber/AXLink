// SPDX-License-Identifier: GPL-3.0-or-later

// Package tnc runs the connection to one KISS TNC: it connects and, after a
// USB reset or a lost link, connects again; it delivers received AX.25 frames
// and hands outgoing frames to the TNC paced by the TNC's profile.
package tnc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/TechnikWeber/AXLink/internal/airtime"
	"github.com/TechnikWeber/AXLink/internal/kiss"
	"github.com/TechnikWeber/AXLink/internal/pacing"
	"github.com/TechnikWeber/AXLink/internal/txqueue"
)

// Config sets up a Port.
type Config struct {
	Dialer   Dialer
	Profile  pacing.Profile
	KISSPort uint8         // KISS port number, normally 0
	QueueLen int           // outgoing frames that may wait; 0 means 32
	MaxAge   time.Duration // a frame waiting longer is dropped; 0 means 60 s
	Logger   *slog.Logger
	// OnSent is called with each frame the TNC has taken completely; nil for
	// none.
	OnSent func(frame []byte)
}

// Stats counts what happened on the port.
type Stats struct {
	Connects, Disconnects  uint64
	FramesIn, FramesOut    uint64
	BytesIn, BytesOut      uint64 // AX.25 frame bytes (without FCS)
	QueueFull, Stale       uint64 // outgoing frames dropped
	Replaced, Repeats      uint64 // merged with a waiting frame; dropped as a repeat
	AckTimeouts, BadKISSIn uint64
	// AirOut and AirHeard estimate the airtime of the frames sent and of the
	// frames the radio decoded: together the busy share of the channel.
	AirOut, AirHeard time.Duration
}

// Port is the connection to one TNC. Create it with NewPort and start Run.
type Port struct {
	cfg   Config
	rx    func(ax25 []byte)
	queue *txqueue.Queue
	log   *slog.Logger
	retry *queued // a frame cut off by a lost connection; owned by Run

	connects, disconnects, framesIn, framesOut atomic.Uint64
	bytesIn, bytesOut                          atomic.Uint64
	airOut, airHeard                           atomic.Int64 // nanoseconds
	staleRetries, ackTimeouts, badKISS         atomic.Uint64
	connectedAt                                atomic.Int64 // Unix nanoseconds; 0 while not connected
}

type queued struct {
	frame []byte
	at    time.Time
	tries int
}

// NewPort prepares a port. rx is called from the reader goroutine with every
// received AX.25 frame (without FCS); it must not block for long.
func NewPort(cfg Config, rx func(ax25 []byte)) *Port {
	if cfg.QueueLen <= 0 {
		cfg.QueueLen = 32
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 60 * time.Second
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	q := txqueue.New(txqueue.Config{Len: cfg.QueueLen, MaxAge: cfg.MaxAge, RepeatWindow: repeatWindow(cfg.Profile)})
	return &Port{cfg: cfg, rx: rx, queue: q, log: log}
}

// repeatWindow covers the frame's airtime and an answer of the same size: a
// TCP retransmission within it cannot be a reaction to a lost answer yet.
func repeatWindow(p pacing.Profile) func(frame []byte) time.Duration {
	return func(frame []byte) time.Duration { return 2*airtimeOf(p, frame) + time.Second }
}

// airtimeOf estimates a frame's time on the air with the profile's timing;
// without timing in the profile it assumes 1200 bit/s and 40 flags.
func airtimeOf(p pacing.Profile, frame []byte) time.Duration {
	baud, flags := p.Baud, p.TxDelayFlags
	if baud == 0 {
		baud = 1200
	}
	if flags == 0 {
		flags = 40
	}
	return airtime.Duration(airtime.Bits(frame, flags, 3), baud)
}

// Send queues an ordinary AX.25 frame (without FCS). It reports false when
// the queue is full; the frame is then dropped.
func (p *Port) Send(frame []byte) bool { return p.SendMeta(frame, txqueue.Meta{}) }

// SendMeta queues a frame with what the transmit queue needs to know about
// it (see package txqueue). It reports false when the queue is full. A frame
// merged with a waiting one or dropped as a repeat counts as sent.
func (p *Port) SendMeta(frame []byte, m txqueue.Meta) bool {
	return p.queue.Push(frame, m) != txqueue.Full
}

// Name names the TNC connection, e.g. "serial:auto" or "tcp:host:8001".
func (p *Port) Name() string { return p.cfg.Dialer.String() }

// Profile returns the TNC profile in use.
func (p *Port) Profile() pacing.Profile { return p.cfg.Profile }

// QueueLen returns how many frames wait for the TNC.
func (p *Port) QueueLen() int { return p.queue.Len() }

// QueueCap returns how many frames may wait.
func (p *Port) QueueCap() int { return p.cfg.QueueLen }

// Connected reports whether the TNC is connected now, and since when.
func (p *Port) Connected() (bool, time.Time) {
	ns := p.connectedAt.Load()
	if ns == 0 {
		return false, time.Time{}
	}
	return true, time.Unix(0, ns)
}

// Stats returns a snapshot of the counters.
func (p *Port) Stats() Stats {
	q := p.queue.Stats()
	return Stats{
		Connects: p.connects.Load(), Disconnects: p.disconnects.Load(),
		FramesIn: p.framesIn.Load(), FramesOut: p.framesOut.Load(),
		BytesIn: p.bytesIn.Load(), BytesOut: p.bytesOut.Load(),
		AirOut: time.Duration(p.airOut.Load()), AirHeard: time.Duration(p.airHeard.Load()),
		QueueFull: q.Full, Stale: q.Stale + p.staleRetries.Load(),
		Replaced: q.Replaced, Repeats: q.Repeats,
		AckTimeouts: p.ackTimeouts.Load(), BadKISSIn: p.badKISS.Load(),
	}
}

// Run connects and keeps the port connected until ctx ends.
func (p *Port) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		conn, err := p.cfg.Dialer.Dial(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.log.Warn("TNC not reachable", "tnc", p.cfg.Dialer.String(), "err", err, "retry", backoff)
			if !sleep(ctx, backoff) {
				return ctx.Err()
			}
			backoff = min(2*backoff, 10*time.Second)
			continue
		}
		backoff = time.Second
		p.connects.Add(1)
		p.connectedAt.Store(time.Now().UnixNano())
		p.log.Info("TNC connected", "tnc", p.cfg.Dialer.String(), "profile", p.cfg.Profile.Name)
		err = p.session(ctx, conn)
		p.connectedAt.Store(0)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.disconnects.Add(1)
		p.log.Warn("TNC connection lost", "tnc", p.cfg.Dialer.String(), "err", err)
		if !sleep(ctx, time.Second) {
			return ctx.Err()
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// session serves one connection until it fails or ctx ends.
func (p *Port) session(ctx context.Context, conn io.ReadWriteCloser) error {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	acks := make(chan uint16, 16)
	clk := realClock{start: time.Now()}
	heard := &heardLog{}
	readDone := make(chan error, 1)
	go func() {
		readDone <- p.readLoop(conn, acks, heard, clk)
		cancel()
	}()
	finish := func(err error) error {
		cancel()
		conn.Close() // unblocks the reader
		rerr := <-readDone
		if err == nil || errors.Is(err, context.Canceled) {
			err = rerr // the reader saw why the session ended
		}
		return err
	}

	s := &sender{profile: p.cfg.Profile, port: p.cfg.KISSPort, clock: clk,
		sink: connSink{w: conn, acks: acks, clock: clk}, heard: heard}
	// Send the profile's KISS parameters on every (re)connect: after a USB
	// reset the app starts again with its defaults.
	if err := s.init(); err != nil {
		return finish(err)
	}
	for {
		var q queued
		if p.retry != nil {
			q, p.retry = *p.retry, nil
			if time.Since(q.at) > p.cfg.MaxAge {
				p.staleRetries.Add(1)
				continue
			}
		} else {
			it, err := p.queue.Pop(sctx)
			if err != nil {
				return finish(nil)
			}
			q = queued{frame: it.Frame, at: it.At}
		}
		before := s.ackTimeouts
		err := s.send(sctx, q.frame)
		if s.ackTimeouts != before {
			p.ackTimeouts.Add(1)
		}
		if err != nil {
			if !s.wroteAll && q.tries == 0 {
				q.tries++
				p.retry = &q // the TNC never got all of it: send it once more after reconnecting
			}
			return finish(err)
		}
		if p.cfg.OnSent != nil {
			p.cfg.OnSent(q.frame)
		}
		p.bytesOut.Add(uint64(len(q.frame)))
		p.airOut.Add(int64(airtimeOf(p.cfg.Profile, q.frame)))
		p.framesOut.Add(1) // last: whoever sees the frame counted sees its bytes too
	}
}

// readLoop delivers received frames and ACKMODE echoes, and notes for the
// sender when the radio heard a frame.
func (p *Port) readLoop(r io.Reader, acks chan<- uint16, heard *heardLog, clk clock) error {
	d := &kiss.Decoder{}
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		before := d.Dropped()
		d.Feed(buf[:n], func(f kiss.Frame) {
			if tag, ok := kiss.AckTag(f); ok {
				select {
				case acks <- tag:
				default:
				}
				return
			}
			if f.Command == kiss.CmdData {
				// Any frame the radio decoded kept the channel busy until now.
				now, air := clk.now(), airtimeOf(p.cfg.Profile, f.Payload)
				heard.add(now-air, now)
				p.airHeard.Add(int64(air))
				p.bytesIn.Add(uint64(len(f.Payload)))
			}
			if f.Command == kiss.CmdData && f.Port == p.cfg.KISSPort {
				p.framesIn.Add(1)
				p.rx(f.Payload)
			}
		})
		if dropped := d.Dropped() - before; dropped > 0 {
			p.badKISS.Add(uint64(dropped))
		}
		if err != nil {
			return err
		}
	}
}

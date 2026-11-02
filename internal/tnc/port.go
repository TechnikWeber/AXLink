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

	"github.com/TechnikWeber/AXLink/internal/kiss"
	"github.com/TechnikWeber/AXLink/internal/pacing"
)

// Config sets up a Port.
type Config struct {
	Dialer   Dialer
	Profile  pacing.Profile
	KISSPort uint8         // KISS port number, normally 0
	QueueLen int           // outgoing frames that may wait; 0 means 32
	MaxAge   time.Duration // a frame waiting longer is dropped; 0 means 60 s
	Logger   *slog.Logger
}

// Stats counts what happened on the port.
type Stats struct {
	Connects, Disconnects  uint64
	FramesIn, FramesOut    uint64
	QueueFull, Stale       uint64 // outgoing frames dropped
	AckTimeouts, BadKISSIn uint64
}

// Port is the connection to one TNC. Create it with NewPort and start Run.
type Port struct {
	cfg   Config
	rx    func(ax25 []byte)
	queue chan queued
	log   *slog.Logger
	retry *queued // a frame cut off by a lost connection; owned by Run

	connects, disconnects, framesIn, framesOut atomic.Uint64
	queueFull, stale, ackTimeouts, badKISS     atomic.Uint64
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
	return &Port{cfg: cfg, rx: rx, queue: make(chan queued, cfg.QueueLen), log: log}
}

// Send queues an AX.25 frame (without FCS). It reports false when the queue
// is full; the frame is then dropped.
func (p *Port) Send(frame []byte) bool {
	select {
	case p.queue <- queued{frame: frame, at: time.Now()}:
		return true
	default:
		p.queueFull.Add(1)
		return false
	}
}

// Stats returns a snapshot of the counters.
func (p *Port) Stats() Stats {
	return Stats{
		Connects: p.connects.Load(), Disconnects: p.disconnects.Load(),
		FramesIn: p.framesIn.Load(), FramesOut: p.framesOut.Load(),
		QueueFull: p.queueFull.Load(), Stale: p.stale.Load(),
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
		p.log.Info("TNC connected", "tnc", p.cfg.Dialer.String(), "profile", p.cfg.Profile.Name)
		err = p.session(ctx, conn)
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
	readDone := make(chan error, 1)
	go func() {
		readDone <- p.readLoop(conn, acks)
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

	clk := realClock{start: time.Now()}
	s := &sender{profile: p.cfg.Profile, port: p.cfg.KISSPort, clock: clk,
		sink: connSink{w: conn, acks: acks, clock: clk}}
	// Send the profile's KISS parameters on every (re)connect: after a USB
	// reset the app starts again with its defaults.
	if err := s.init(); err != nil {
		return finish(err)
	}
	for {
		var q queued
		if p.retry != nil {
			q, p.retry = *p.retry, nil
		} else {
			select {
			case <-sctx.Done():
				return finish(nil)
			case q = <-p.queue:
			}
		}
		if time.Since(q.at) > p.cfg.MaxAge {
			p.stale.Add(1)
			continue
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
		p.framesOut.Add(1)
	}
}

func (p *Port) readLoop(r io.Reader, acks chan<- uint16) error {
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

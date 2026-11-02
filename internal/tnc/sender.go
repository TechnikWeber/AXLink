// SPDX-License-Identifier: GPL-3.0-or-later

package tnc

import (
	"context"
	"io"
	"time"

	"github.com/TechnikWeber/AXLink/internal/airtime"
	"github.com/TechnikWeber/AXLink/internal/kiss"
	"github.com/TechnikWeber/AXLink/internal/pacing"
)

// clock and sink let the sender run against the real TNC connection or, in
// tests, against the simulated F4HWN KISS app in virtual time.
type clock interface {
	now() time.Duration
	sleepUntil(ctx context.Context, t time.Duration) error
}

type sink interface {
	write(p []byte) error
	// waitAck waits for the ACKMODE echo of tag until the clock reaches deadline.
	waitAck(ctx context.Context, tag uint16, deadline time.Duration) (bool, error)
}

// sender hands frames to the TNC one at a time, as the profile asks.
type sender struct {
	profile     pacing.Profile
	port        uint8
	clock       clock
	sink        sink
	tag         uint16
	ackTimeouts uint64
	wroteAll    bool // the last frame reached the TNC completely
}

func (s *sender) init() error {
	if len(s.profile.Resync) > 0 {
		if err := s.sink.write(s.profile.Resync); err != nil {
			return err
		}
	}
	for _, f := range s.profile.Init(s.port) {
		b, err := kiss.Encode(f)
		if err != nil {
			return err
		}
		if err := s.sink.write(b); err != nil {
			return err
		}
	}
	return nil
}

func (s *sender) send(ctx context.Context, frame []byte) error {
	f := kiss.Data(s.port, frame)
	var tag uint16
	if s.profile.AckMode {
		s.tag++
		if s.tag == 0 {
			s.tag = 1
		}
		tag = s.tag
		f = kiss.AckModeData(s.port, tag, frame)
	}
	enc, err := kiss.Encode(f)
	if err != nil {
		return err
	}
	plan := s.profile.Plan(s.clock.now(), enc, frame)
	s.wroteAll = false
	for _, w := range plan.Writes {
		if err := s.clock.sleepUntil(ctx, w.At); err != nil {
			return err
		}
		if err := s.sink.write(w.Data); err != nil {
			return err
		}
	}
	s.wroteAll = true
	if !s.profile.OneInFlight {
		return nil
	}
	if !s.profile.AckMode {
		return s.clock.sleepUntil(ctx, plan.Done)
	}
	ok, err := s.sink.waitAck(ctx, tag, s.clock.now()+s.ackTimeout(frame))
	if err != nil {
		return err
	}
	if !ok {
		s.ackTimeouts++ // the frame may be lost; carry on with the next one
	}
	return nil
}

// ackTimeout allows for the frame's airtime plus a long busy channel.
func (s *sender) ackTimeout(frame []byte) time.Duration {
	baud := s.profile.Baud
	if baud == 0 {
		baud = 1200
	}
	tx := airtime.Duration(airtime.Bits(frame, max(s.profile.TxDelayFlags, 40), 3), baud)
	return 2*tx + 10*time.Second
}

// realClock measures from the start of a TNC session.
type realClock struct{ start time.Time }

func (c realClock) now() time.Duration { return time.Since(c.start) }

func (c realClock) sleepUntil(ctx context.Context, t time.Duration) error {
	d := t - c.now()
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// connSink writes to the TNC connection and receives ACKMODE echoes from the
// reader.
type connSink struct {
	w     io.Writer
	acks  <-chan uint16
	clock clock
}

func (s connSink) write(p []byte) error {
	_, err := s.w.Write(p)
	return err
}

func (s connSink) waitAck(ctx context.Context, tag uint16, deadline time.Duration) (bool, error) {
	for {
		d := deadline - s.clock.now()
		if d <= 0 {
			return false, nil
		}
		timer := time.NewTimer(d)
		select {
		case got := <-s.acks:
			timer.Stop()
			if got == tag {
				return true, nil
			} // a late echo of an earlier frame: keep waiting
		case <-timer.C:
			return false, nil
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		}
	}
}

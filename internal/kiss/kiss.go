// SPDX-License-Identifier: GPL-3.0-or-later

// Package kiss implements the KISS host-to-TNC framing (Chepponis and Karn,
// 1987) and the G8BPQ ACKMODE extension (command 0x0C).
package kiss

import "errors"

// Special bytes of the KISS framing.
const (
	FEND  byte = 0xC0 // frame end
	FESC  byte = 0xDB // frame escape
	TFEND byte = 0xDC // transposed frame end
	TFESC byte = 0xDD // transposed frame escape
)

// Command is the low nibble of a KISS type byte; the high nibble is the port.
type Command byte

// KISS commands.
const (
	CmdData        Command = 0x0 // AX.25 frame without FCS
	CmdTxDelay     Command = 0x1 // key-up delay in 10 ms units
	CmdPersist     Command = 0x2 // p-persistence: send with probability (P+1)/256
	CmdSlotTime    Command = 0x3 // slot interval in 10 ms units
	CmdTxTail      Command = 0x4 // obsolete; 10 ms units
	CmdFullDuplex  Command = 0x5
	CmdSetHardware Command = 0x6 // TNC specific
	CmdAckMode     Command = 0xC // G8BPQ ACKMODE: tagged data frame or TX-complete echo
)

// Return is the type byte that tells a TNC to leave KISS mode. A decoded
// Return frame shows up as port 15, command 0xF.
const Return byte = 0xFF

// DefaultMaxFrame bounds a decoded frame, type byte included.
const DefaultMaxFrame = 1024

// ErrPort reports a port number above 15.
var ErrPort = errors.New("kiss: port must be 0..15")

// Frame is one KISS frame without framing and escapes.
type Frame struct {
	Port    uint8 // 0..15
	Command Command
	Payload []byte
}

// TypeByte returns the KISS type byte of f.
func (f Frame) TypeByte() byte { return f.Port<<4 | byte(f.Command)&0x0F }

// Append appends the encoded frame (FEND, type byte, payload, FEND) to dst.
// The type byte is escaped like the payload: port 12 data has type 0xC0.
func Append(dst []byte, f Frame) ([]byte, error) {
	if f.Port > 15 {
		return dst, ErrPort
	}
	dst = append(dst, FEND)
	dst = appendEscaped(dst, f.TypeByte())
	for _, b := range f.Payload {
		dst = appendEscaped(dst, b)
	}
	return append(dst, FEND), nil
}

// Encode returns the encoded frame.
func Encode(f Frame) ([]byte, error) { return Append(nil, f) }

func appendEscaped(dst []byte, b byte) []byte {
	switch b {
	case FEND:
		return append(dst, FESC, TFEND)
	case FESC:
		return append(dst, FESC, TFESC)
	}
	return append(dst, b)
}

// Data returns a data frame carrying an AX.25 frame (without FCS).
func Data(port uint8, ax25 []byte) Frame {
	return Frame{Port: port, Command: CmdData, Payload: ax25}
}

// Param returns a one-byte parameter frame such as TXDELAY or PERSIST.
func Param(port uint8, cmd Command, value byte) Frame {
	return Frame{Port: port, Command: cmd, Payload: []byte{value}}
}

// AckModeData returns an ACKMODE data frame: the 2-byte tag, then the AX.25
// frame. A TNC that supports ACKMODE echoes the tag once the frame is sent.
func AckModeData(port uint8, tag uint16, ax25 []byte) Frame {
	p := make([]byte, 2+len(ax25))
	p[0], p[1] = byte(tag>>8), byte(tag)
	copy(p[2:], ax25)
	return Frame{Port: port, Command: CmdAckMode, Payload: p}
}

// AckTag reports the tag of an ACKMODE TX-complete echo: command 0x0C with a
// payload of exactly two bytes.
func AckTag(f Frame) (uint16, bool) {
	if f.Command != CmdAckMode || len(f.Payload) != 2 {
		return 0, false
	}
	return uint16(f.Payload[0])<<8 | uint16(f.Payload[1]), true
}

// Decoder splits a KISS byte stream into frames. Bytes before the first FEND
// are ignored, as are empty frames. A frame with an invalid escape or with
// more than MaxFrame bytes is dropped and counted.
type Decoder struct {
	MaxFrame int // 0 means DefaultMaxFrame

	buf     []byte
	synced  bool
	escaped bool
	bad     bool
	dropped int
}

// Dropped returns how many frames were discarded as invalid or too long.
func (d *Decoder) Dropped() int { return d.dropped }

// Feed consumes p and calls emit for every complete frame. Each payload is a
// fresh copy that emit may keep.
func (d *Decoder) Feed(p []byte, emit func(Frame)) {
	limit := d.MaxFrame
	if limit <= 0 {
		limit = DefaultMaxFrame
	}
	for _, b := range p {
		if b == FEND {
			d.finish(emit)
			d.synced = true
			continue
		}
		if !d.synced || d.bad {
			continue
		}
		if d.escaped {
			d.escaped = false
			switch b {
			case TFEND:
				b = FEND
			case TFESC:
				b = FESC
			default:
				d.bad = true
				continue
			}
		} else if b == FESC {
			d.escaped = true
			continue
		}
		if len(d.buf) >= limit {
			d.bad = true
			continue
		}
		d.buf = append(d.buf, b)
	}
}

func (d *Decoder) finish(emit func(Frame)) {
	switch {
	case d.bad || d.escaped:
		d.dropped++
	case len(d.buf) > 0:
		t := d.buf[0]
		payload := append([]byte(nil), d.buf[1:]...)
		emit(Frame{Port: t >> 4, Command: Command(t & 0x0F), Payload: payload})
	}
	d.buf = d.buf[:0]
	d.escaped, d.bad = false, false
}

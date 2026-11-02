// SPDX-License-Identifier: GPL-3.0-or-later

// Package ax25 encodes and decodes AX.25 v2.2 address fields and modulo-8
// frames as a KISS TNC carries them: without flags, bit stuffing and FCS.
// Decoding is strict: callsigns must be A-Z/0-9 padded with spaces.
package ax25

import (
	"errors"
	"strconv"
	"strings"
)

// Common PIDs.
const (
	PIDSegment byte = 0x08 // AX.25 segmentation fragment
	PIDIPv4    byte = 0xCC // ARPA Internet Protocol
	PIDARP     byte = 0xCD // ARPA Address Resolution
	PIDNetROM  byte = 0xCF
	PIDNoL3    byte = 0xF0 // no layer 3 (APRS, plain text)
)

// Control field of UI frames, with the P/F bit clear or set.
const (
	ControlUI   byte = 0x03
	ControlUIPF byte = 0x13
)

// MaxRepeaters is the longest digipeater path AX.25 v2.2 allows.
const MaxRepeaters = 8

const addressLen = 7

// Errors returned by the parsers and encoders.
var (
	ErrCallsign     = errors.New("ax25: callsign must be 1-6 characters A-Z or 0-9")
	ErrSSID         = errors.New("ax25: SSID must be 0-15")
	ErrShort        = errors.New("ax25: frame too short")
	ErrAddressField = errors.New("ax25: malformed address field")
	ErrPathTooLong  = errors.New("ax25: more than 8 repeaters")
)

// Address is a station address. Bit7 is bit 7 of the SSID octet: the C bit
// in destination and source, the H bit ("has been repeated") in the path.
type Address struct {
	Call string // 1-6 characters A-Z, 0-9
	SSID uint8  // 0-15
	Bit7 bool
}

// ParseAddress parses "CALL" or "CALL-SSID". Letters are upper-cased;
// "CALL-0" and "CALL" are the same address.
func ParseAddress(s string) (Address, error) {
	call, ssidText, hasSSID := strings.Cut(strings.TrimSpace(s), "-")
	call = strings.ToUpper(call)
	if err := checkCall(call); err != nil {
		return Address{}, err
	}
	var ssid uint64
	if hasSSID {
		v, err := strconv.ParseUint(ssidText, 10, 8)
		if err != nil || v > 15 {
			return Address{}, ErrSSID
		}
		ssid = v
	}
	return Address{Call: call, SSID: uint8(ssid)}, nil
}

// MustAddress is ParseAddress for constants in code and tests; it panics on error.
func MustAddress(s string) Address {
	a, err := ParseAddress(s)
	if err != nil {
		panic(err)
	}
	return a
}

func checkCall(c string) error {
	if len(c) < 1 || len(c) > 6 {
		return ErrCallsign
	}
	for i := 0; i < len(c); i++ {
		if !isCallChar(c[i]) {
			return ErrCallsign
		}
	}
	return nil
}

func isCallChar(ch byte) bool { return ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' }

// String returns "CALL" for SSID 0 and "CALL-SSID" otherwise.
func (a Address) String() string {
	if a.SSID == 0 {
		return a.Call
	}
	return a.Call + "-" + strconv.Itoa(int(a.SSID))
}

// SameStation reports whether a and b name the same station; Bit7 is ignored.
func (a Address) SameStation(b Address) bool { return a.Call == b.Call && a.SSID == b.SSID }

func appendAddress(dst []byte, a Address, last bool) ([]byte, error) {
	if err := checkCall(a.Call); err != nil {
		return dst, err
	}
	if a.SSID > 15 {
		return dst, ErrSSID
	}
	for i := 0; i < 6; i++ {
		ch := byte(' ')
		if i < len(a.Call) {
			ch = a.Call[i]
		}
		dst = append(dst, ch<<1)
	}
	o := 0x60 | a.SSID<<1 // reserved bits set, as AX.25 v2.2 specifies
	if a.Bit7 {
		o |= 0x80
	}
	if last {
		o |= 0x01
	}
	return append(dst, o), nil
}

// decodeAddress parses one 7-byte address and reports the end-of-field bit.
func decodeAddress(b []byte) (Address, bool, error) {
	var call [6]byte
	n := 0
	padding := false
	for i := 0; i < 6; i++ {
		if b[i]&0x01 != 0 {
			return Address{}, false, ErrAddressField
		}
		ch := b[i] >> 1
		switch {
		case ch == ' ':
			padding = true
		case isCallChar(ch) && !padding:
			call[n] = ch
			n++
		default:
			return Address{}, false, ErrCallsign
		}
	}
	if n == 0 {
		return Address{}, false, ErrCallsign
	}
	o := b[6]
	a := Address{Call: string(call[:n]), SSID: o >> 1 & 0x0F, Bit7: o&0x80 != 0}
	return a, o&0x01 != 0, nil
}

// Frame is a modulo-8 AX.25 frame without flags and FCS.
type Frame struct {
	Dest, Src Address
	Path      []Address // digipeaters, at most MaxRepeaters
	Control   byte
	PID       byte // only for I and UI frames, see HasPID
	Info      []byte
}

// HasPID reports whether frames with this control field carry a PID octet:
// I frames and UI frames.
func HasPID(control byte) bool { return control&0x01 == 0 || control&^0x10 == ControlUI }

// IsUI reports whether f is an unnumbered-information frame.
func (f *Frame) IsUI() bool { return f.Control&^0x10 == ControlUI }

// IsCommand reports the AX.25 v2 command bits: destination C=1, source C=0.
func (f *Frame) IsCommand() bool { return f.Dest.Bit7 && !f.Src.Bit7 }

// NewUI returns a UI command frame.
func NewUI(dest, src Address, path []Address, pid byte, info []byte) *Frame {
	dest.Bit7, src.Bit7 = true, false
	return &Frame{Dest: dest, Src: src, Path: path, Control: ControlUI, PID: pid, Info: info}
}

// Append encodes f without FCS and appends it to dst.
func (f *Frame) Append(dst []byte) ([]byte, error) {
	if len(f.Path) > MaxRepeaters {
		return dst, ErrPathTooLong
	}
	var err error
	if dst, err = appendAddress(dst, f.Dest, false); err != nil {
		return dst, err
	}
	if dst, err = appendAddress(dst, f.Src, len(f.Path) == 0); err != nil {
		return dst, err
	}
	for i, r := range f.Path {
		if dst, err = appendAddress(dst, r, i == len(f.Path)-1); err != nil {
			return dst, err
		}
	}
	dst = append(dst, f.Control)
	if HasPID(f.Control) {
		dst = append(dst, f.PID)
	}
	return append(dst, f.Info...), nil
}

// Encode returns f encoded without FCS.
func (f *Frame) Encode() ([]byte, error) { return f.Append(nil) }

// Decode parses a frame without FCS, as delivered by a KISS TNC.
// Info aliases b.
func Decode(b []byte) (*Frame, error) {
	var addrs []Address
	off := 0
	for {
		if len(b)-off < addressLen {
			return nil, ErrShort
		}
		a, last, err := decodeAddress(b[off : off+addressLen])
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, a)
		off += addressLen
		if last {
			break
		}
		if len(addrs) == 2+MaxRepeaters {
			return nil, ErrPathTooLong
		}
	}
	if len(addrs) < 2 {
		return nil, ErrAddressField
	}
	if off >= len(b) {
		return nil, ErrShort
	}
	f := &Frame{Dest: addrs[0], Src: addrs[1], Path: addrs[2:], Control: b[off]}
	off++
	if HasPID(f.Control) {
		if off >= len(b) {
			return nil, ErrShort
		}
		f.PID = b[off]
		off++
	}
	f.Info = b[off:]
	return f, nil
}

// FCS returns the AX.25 frame check sequence (CRC-16/X.25) of b.
func FCS(b []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, v := range b {
		crc ^= uint16(v)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0x8408
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}

// AppendFCS appends the FCS of b, low byte first as it goes on the air.
func AppendFCS(b []byte) []byte {
	c := FCS(b)
	return append(b, byte(c), byte(c>>8))
}

// CheckFCS reports whether the last two bytes of b are the FCS of the rest.
func CheckFCS(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	n := len(b) - 2
	c := FCS(b[:n])
	return b[n] == byte(c) && b[n+1] == byte(c>>8)
}

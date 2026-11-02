// SPDX-License-Identifier: GPL-3.0-or-later

// Package discovery announces this station and learns the others from their
// beacons: callsign, IPv4 address and tags. The format is specified in
// docs/protocol/beacon.md.
package discovery

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// Dest is the destination address of every beacon.
var Dest = ax25.Address{Call: "AXLINK"}

// DefaultEvery is the interval of periodic beacons when a beacon names none.
const DefaultEvery = 10 * time.Minute

// The roles a station can announce.
const (
	TagRelay = "RELAY" // forwards for others
	TagInet  = "INET"  // shares an internet connection
	TagLink  = "LINK"  // connects radio islands over the internet
	TagAuto  = "AUTO"  // runs unattended
)

const (
	magic     = "AXL1"
	maxInfo   = 128
	minEvery  = time.Minute
	maxEvery  = 24 * time.Hour
	maxTags   = 8
	maxTagLen = 8
	maxKeyLen = 16
)

var (
	errInvalid = errors.New("discovery: invalid beacon")
	errRelayed = errors.New("discovery: beacon came through a digipeater")
)

// Beacon is what one beacon says.
type Beacon struct {
	Addr  netip.Prefix  // IPv4 address with the prefix length of its subnet
	Tags  []string      // roles, see the Tag constants; unknown tags are kept
	Every time.Duration // until the sender's next periodic beacon
	Ask   bool          // the sender has just started and asks for beacons
}

// Marshal returns the information field.
func (b Beacon) Marshal() ([]byte, error) {
	if !b.Addr.IsValid() || !b.Addr.Addr().Is4() || b.Addr.Bits() == 0 {
		return nil, fmt.Errorf("%w: address %v", errInvalid, b.Addr)
	}
	if b.Every < minEvery || b.Every > maxEvery || b.Every%time.Second != 0 {
		return nil, fmt.Errorf("%w: interval %v", errInvalid, b.Every)
	}
	if err := checkTags(b.Tags); err != nil {
		return nil, err
	}
	var sb strings.Builder
	sb.WriteString(magic + " ip=" + b.Addr.String())
	if b.Every != DefaultEvery {
		sb.WriteString(" every=" + strconv.Itoa(int(b.Every/time.Second)))
	}
	if len(b.Tags) > 0 {
		sb.WriteString(" tags=" + strings.Join(b.Tags, ","))
	}
	if b.Ask {
		sb.WriteString(" ask=1")
	}
	if sb.Len() > maxInfo {
		return nil, fmt.Errorf("%w: longer than %d bytes", errInvalid, maxInfo)
	}
	return []byte(sb.String()), nil
}

// Parse reads an information field. It ignores unknown keys and keeps
// unknown tags; anything else that does not follow the format is an error.
func Parse(info []byte) (Beacon, error) {
	if len(info) > maxInfo {
		return Beacon{}, fmt.Errorf("%w: longer than %d bytes", errInvalid, maxInfo)
	}
	fields := strings.Split(string(info), " ")
	if fields[0] != magic {
		return Beacon{}, fmt.Errorf("%w: does not start with %s", errInvalid, magic)
	}
	b := Beacon{Every: DefaultEvery}
	seen := map[string]bool{}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok || !validKey(k) || !validValue(v) || seen[k] {
			return Beacon{}, fmt.Errorf("%w: field %q", errInvalid, f)
		}
		seen[k] = true
		switch k {
		case "ip":
			p, err := netip.ParsePrefix(v)
			if err != nil || !p.Addr().Is4() || p.Bits() == 0 {
				return Beacon{}, fmt.Errorf("%w: ip %q", errInvalid, v)
			}
			b.Addr = p
		case "every":
			n, err := strconv.Atoi(v)
			if err != nil || v != strconv.Itoa(n) {
				return Beacon{}, fmt.Errorf("%w: every %q", errInvalid, v)
			}
			b.Every = time.Duration(n) * time.Second
			if b.Every < minEvery || b.Every > maxEvery {
				return Beacon{}, fmt.Errorf("%w: every %q", errInvalid, v)
			}
		case "tags":
			b.Tags = strings.Split(v, ",")
			if err := checkTags(b.Tags); err != nil {
				return Beacon{}, err
			}
		case "ask":
			if v != "1" {
				return Beacon{}, fmt.Errorf("%w: ask %q", errInvalid, v)
			}
			b.Ask = true
		}
	}
	if !seen["ip"] {
		return Beacon{}, fmt.Errorf("%w: no ip", errInvalid)
	}
	return b, nil
}

// ParseTags reads a comma-separated tag list as given on the command line;
// letters are upper-cased.
func ParseTags(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	tags := strings.Split(strings.ToUpper(s), ",")
	return tags, checkTags(tags)
}

func checkTags(tags []string) error {
	if len(tags) > maxTags {
		return fmt.Errorf("%w: more than %d tags", errInvalid, maxTags)
	}
	for i, t := range tags {
		if len(t) == 0 || len(t) > maxTagLen || strings.IndexFunc(t, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
		}) >= 0 {
			return fmt.Errorf("%w: tag %q (1-%d characters A-Z, 0-9)", errInvalid, t, maxTagLen)
		}
		if slices.Contains(tags[:i], t) {
			return fmt.Errorf("%w: tag %q twice", errInvalid, t)
		}
	}
	return nil
}

func validKey(k string) bool {
	if len(k) == 0 || len(k) > maxKeyLen {
		return false
	}
	for i := 0; i < len(k); i++ {
		if !(k[i] >= 'a' && k[i] <= 'z' || k[i] >= '0' && k[i] <= '9') {
			return false
		}
	}
	return true
}

func validValue(v string) bool {
	if len(v) == 0 {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x21 || v[i] > 0x7E {
			return false
		}
	}
	return true
}

// Frame returns the beacon as an AX.25 UI frame (without FCS) from call.
func (b Beacon) Frame(call ax25.Address) ([]byte, error) {
	info, err := b.Marshal()
	if err != nil {
		return nil, err
	}
	return ax25.NewUI(Dest, call, nil, ax25.PIDNoL3, info).Encode()
}

// FromFrame reports whether f is a beacon (a UI frame to AXLINK with PID
// 0xF0) and returns its sender and content. err is set for a beacon that is
// invalid or came through a digipeater.
func FromFrame(f *ax25.Frame) (call ax25.Address, b Beacon, ok bool, err error) {
	if !f.IsUI() || f.PID != ax25.PIDNoL3 || !f.Dest.SameStation(Dest) {
		return ax25.Address{}, Beacon{}, false, nil
	}
	if len(f.Path) > 0 {
		return f.Src, Beacon{}, true, errRelayed
	}
	b, err = Parse(f.Info)
	return f.Src, b, true, err
}

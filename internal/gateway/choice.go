// SPDX-License-Identifier: GPL-3.0-or-later

package gateway

import (
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Candidate is a station that offers an internet gateway.
type Candidate struct {
	Call  string
	Proxy netip.AddrPort
	Heard time.Time // its last beacon
}

// Mode says how the client picks its gateway.
type Mode string

// The modes.
const (
	ModeOff     Mode = "off"     // no internet over the radio
	ModeAuto    Mode = "auto"    // the gateway heard most recently, kept while it is heard
	ModeStation Mode = "station" // one station, chosen by its callsign
)

// Choice is the client's choice of an internet gateway. It is safe for
// concurrent use.
type Choice struct {
	candidates func() []Candidate

	mu   sync.Mutex
	mode Mode
	call string // ModeStation: the chosen station
	auto string // ModeAuto: the station in use
}

// NewChoice starts with ModeOff; candidates lists the gateways heard.
func NewChoice(candidates func() []Candidate) *Choice {
	return &Choice{candidates: candidates, mode: ModeOff}
}

// Set changes the choice. call is needed for ModeStation.
func (c *Choice) Set(mode Mode, call string) error {
	call = strings.ToUpper(strings.TrimSpace(call))
	switch mode {
	case ModeOff, ModeAuto:
		call = ""
	case ModeStation:
		if call == "" {
			return fmt.Errorf("gateway: a station needs its callsign")
		}
	default:
		return fmt.Errorf("gateway: unknown mode %q", mode)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode, c.call, c.auto = mode, call, ""
	return nil
}

// Get returns the mode and, for ModeStation, the chosen callsign.
func (c *Choice) Get() (Mode, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode, c.call
}

// Upstream returns the proxy of the gateway to use now.
func (c *Choice) Upstream() (netip.AddrPort, string, bool) {
	cands := c.candidates()
	c.mu.Lock()
	defer c.mu.Unlock()
	find := func(call string) (Candidate, bool) {
		for _, g := range cands {
			if g.Call == call {
				return g, true
			}
		}
		return Candidate{}, false
	}
	switch c.mode {
	case ModeStation:
		if g, ok := find(c.call); ok {
			return g.Proxy, g.Call, true
		}
	case ModeAuto:
		if g, ok := find(c.auto); ok && c.auto != "" {
			return g.Proxy, g.Call, true // stay with it while it is heard
		}
		var best Candidate
		for _, g := range cands {
			if best.Call == "" || g.Heard.After(best.Heard) {
				best = g
			}
		}
		if best.Call != "" {
			c.auto = best.Call
			return best.Proxy, best.Call, true
		}
	}
	return netip.AddrPort{}, "", false
}

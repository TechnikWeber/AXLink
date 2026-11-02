// SPDX-License-Identifier: GPL-3.0-or-later

// Package tun opens AXLink's virtual network adapter: ordinary programs send
// IP packets to it, AXLink carries them over the radio. One packet per read
// or write. Linux (TUN) and Windows (Wintun) are implemented; macOS follows.
package tun

import (
	"errors"
	"fmt"
	"net/netip"
)

// Config describes the adapter.
type Config struct {
	Name    string       // interface name, e.g. "axl0"
	MTU     int          // requested MTU; raised to the system minimum (Windows: 576)
	Address netip.Prefix // own address and subnet, e.g. 10.73.0.1/24
	Routes  []netip.Prefix
}

// Device is an open adapter.
type Device interface {
	Name() string
	// MTU is the MTU the operating system uses on the adapter.
	MTU() int
	// ReadPacket reads one IP packet into buf and returns its length.
	ReadPacket(buf []byte) (int, error)
	// WritePacket hands one IP packet to the operating system.
	WritePacket(pkt []byte) error
	Close() error
}

// ErrUnsupported is returned on systems without an implementation yet.
var ErrUnsupported = errors.New("tun: not implemented on this operating system yet")

// MinMTU is the smallest IPv4 MTU the operating system accepts on the
// adapter. AXLink keeps radio packets smaller itself (MSS clamping and
// fragmentation in package ipbridge) when the adapter MTU is larger.
const MinMTU = minMTU

// Open creates and configures the adapter. It needs the right to configure
// network interfaces (Linux: CAP_NET_ADMIN; Windows: administrator).
func Open(cfg Config) (Device, error) {
	if cfg.MTU < 68 || cfg.MTU > 65535 {
		return nil, fmt.Errorf("tun: MTU %d out of range", cfg.MTU)
	}
	if !cfg.Address.IsValid() || !cfg.Address.Addr().Is4() {
		return nil, errors.New("tun: an IPv4 address with prefix length is required")
	}
	cfg.MTU = max(cfg.MTU, minMTU)
	if cfg.Name == "" {
		cfg.Name = "axl0"
	}
	return open(cfg)
}

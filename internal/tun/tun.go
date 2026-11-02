// SPDX-License-Identifier: GPL-3.0-or-later

// Package tun opens AXLink's virtual network adapter: ordinary programs send
// IP packets to it, AXLink carries them over the radio. One packet per read
// or write. Linux is implemented; Windows (Wintun) and macOS (utun) follow.
package tun

import (
	"errors"
	"fmt"
	"net/netip"
)

// Config describes the adapter.
type Config struct {
	Name    string       // interface name, e.g. "axl0" (Linux)
	MTU     int          // 256 matches the AX.25 default frame size
	Address netip.Prefix // own address and subnet, e.g. 10.73.0.1/24
	Routes  []netip.Prefix
}

// Device is an open adapter.
type Device interface {
	Name() string
	// ReadPacket reads one IP packet into buf and returns its length.
	ReadPacket(buf []byte) (int, error)
	// WritePacket hands one IP packet to the operating system.
	WritePacket(pkt []byte) error
	Close() error
}

// ErrUnsupported is returned on systems without an implementation yet.
var ErrUnsupported = errors.New("tun: not implemented on this operating system yet")

// Open creates and configures the adapter. It needs the right to configure
// network interfaces (Linux: CAP_NET_ADMIN).
func Open(cfg Config) (Device, error) {
	if cfg.MTU < 68 || cfg.MTU > 65535 {
		return nil, fmt.Errorf("tun: MTU %d out of range", cfg.MTU)
	}
	if !cfg.Address.IsValid() || !cfg.Address.Addr().Is4() {
		return nil, errors.New("tun: an IPv4 address with prefix length is required")
	}
	return open(cfg)
}

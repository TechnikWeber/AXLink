// SPDX-License-Identifier: GPL-3.0-or-later

// Package ipbridge carries IPv4 packets between the virtual network adapter
// and the radio as AX.25 UI frames with PID 0xCC, the classic framing of IP
// over AX.25 that Linux, JNOS, BPQ and Packet.NET also use.
package ipbridge

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"sync/atomic"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/txqueue"
)

// Route sends packets for Prefix to the station Call.
type Route struct {
	Prefix netip.Prefix
	Call   ax25.Address
}

// Config sets up a bridge.
type Config struct {
	MyCall ax25.Address
	Local  netip.Prefix // own address and subnet on the adapter
	Routes []Route
	// AllowBroadcast lets broadcast and multicast packets onto the radio.
	// Off by default: they are mostly chatter (mDNS, LLMNR, SSDP, NetBIOS)
	// that would fill a 1200 bit/s channel. A technical filter, not a rule.
	AllowBroadcast bool
	// LinkMTU is the largest IP packet on the radio; 0 means 256. When the
	// adapter's MTU is larger (Windows: at least 576), the bridge lowers the
	// TCP MSS in both directions and fragments larger packets without DF.
	LinkMTU int
	// Learned returns the station that announced an address (station
	// discovery); nil for none. A configured route for exactly that address
	// comes first, a learned address before configured shorter prefixes.
	Learned func(addr netip.Addr) (ax25.Address, bool)
	// Filter decides about packets from the radio (the firewall); nil lets
	// every packet for the own address through.
	Filter Filter
}

// Filter decides about packets from the radio and learns from the station's
// own packets which answers to expect.
type Filter interface {
	Outbound(pkt []byte)
	Inbound(pkt []byte, from ax25.Address) bool
}

// Stats counts packets.
type Stats struct {
	ToRadio, FromRadio            uint64
	NoRoute, Filtered, NotIPv4    uint64 // dropped on the way out
	QueueFull                     uint64
	NotForUs, BadPacket, OtherPID uint64 // dropped on the way in
	DeliverErrors                 uint64
	Clamped, Fragmented, TooBig   uint64 // MSS lowered; packets split; dropped with DF set
	WrongAddress, Blocked         uint64 // from the radio: not for the own address; refused by the filter
}

// Bridge moves packets in both directions.
type Bridge struct {
	cfg     Config
	send    func(frame []byte, m txqueue.Meta) bool
	deliver func(pkt []byte) error

	toRadio, fromRadio, noRoute, filtered, notIPv4, queueFull atomic.Uint64
	notForUs, badPacket, otherPID, deliverErrors              atomic.Uint64
	clamped, fragmented, tooBig                               atomic.Uint64
	wrongAddress, blocked                                     atomic.Uint64
}

// New returns a bridge. send hands an AX.25 frame and what the transmit
// queue needs to know about it to the radio port and reports false if it was
// dropped; deliver hands an IP packet to the adapter.
func New(cfg Config, send func(frame []byte, m txqueue.Meta) bool, deliver func(pkt []byte) error) (*Bridge, error) {
	if _, err := ax25.ParseAddress(cfg.MyCall.String()); err != nil {
		return nil, fmt.Errorf("ipbridge: own callsign: %w", err)
	}
	routes := make([]Route, 0, len(cfg.Routes))
	for _, r := range cfg.Routes {
		if !r.Prefix.IsValid() || !r.Prefix.Addr().Is4() {
			return nil, fmt.Errorf("ipbridge: route %v is not an IPv4 prefix", r.Prefix)
		}
		routes = append(routes, Route{Prefix: r.Prefix.Masked(), Call: r.Call})
	}
	cfg.Routes = routes
	if cfg.LinkMTU == 0 {
		cfg.LinkMTU = DefaultLinkMTU
	}
	if cfg.LinkMTU < 68 {
		return nil, fmt.Errorf("ipbridge: link MTU %d below the IPv4 minimum of 68", cfg.LinkMTU)
	}
	return &Bridge{cfg: cfg, send: send, deliver: deliver}, nil
}

// Stats returns a snapshot of the counters.
func (b *Bridge) Stats() Stats {
	return Stats{
		ToRadio: b.toRadio.Load(), FromRadio: b.fromRadio.Load(),
		NoRoute: b.noRoute.Load(), Filtered: b.filtered.Load(), NotIPv4: b.notIPv4.Load(),
		QueueFull: b.queueFull.Load(),
		NotForUs:  b.notForUs.Load(), BadPacket: b.badPacket.Load(), OtherPID: b.otherPID.Load(),
		DeliverErrors: b.deliverErrors.Load(),
		Clamped:       b.clamped.Load(), Fragmented: b.fragmented.Load(), TooBig: b.tooBig.Load(),
		WrongAddress: b.wrongAddress.Load(), Blocked: b.blocked.Load(),
	}
}

// FromAdapter takes a packet the operating system sent to the adapter. The
// packet may be reused by the caller after the call.
func (b *Bridge) FromAdapter(pkt []byte) {
	h, err := parseIPv4(pkt, false)
	if err != nil {
		if errors.Is(err, errNotIPv4) {
			b.notIPv4.Add(1)
		} else {
			b.badPacket.Add(1)
		}
		return
	}
	if !b.cfg.AllowBroadcast && b.isBroadcast(h.dst) {
		b.filtered.Add(1)
		return
	}
	call, ok := b.route(h.dst)
	if !ok {
		b.noRoute.Add(1)
		return
	}
	pkt = pkt[:h.totalLen]
	if clampMSS(pkt, b.cfg.LinkMTU-40) {
		b.clamped.Add(1)
	}
	if b.cfg.Filter != nil {
		b.cfg.Filter.Outbound(pkt)
	}
	packets := [][]byte{pkt}
	if len(pkt) > b.cfg.LinkMTU {
		if binary.BigEndian.Uint16(pkt[6:8])&flagDF != 0 {
			b.tooBig.Add(1)
			return
		}
		if packets, err = fragment(pkt, b.cfg.LinkMTU); err != nil {
			b.badPacket.Add(1)
			return
		}
		b.fragmented.Add(1)
	}
	for _, p := range packets {
		frame, err := ax25.NewUI(call, b.cfg.MyCall, nil, ax25.PIDIPv4, p).Encode()
		if err != nil {
			b.badPacket.Add(1)
			return
		}
		if !b.send(frame, classify(p)) {
			b.queueFull.Add(1)
			return
		}
		b.toRadio.Add(1)
	}
}

// FromRadio takes an AX.25 frame (without FCS) heard on the radio.
func (b *Bridge) FromRadio(frame []byte) {
	f, err := ax25.Decode(frame)
	if err != nil {
		b.badPacket.Add(1)
		return
	}
	if !f.IsUI() || f.PID != ax25.PIDIPv4 {
		b.otherPID.Add(1)
		return
	}
	// The radio hears everything on the channel: only frames addressed to
	// this station go to the adapter.
	if !f.Dest.SameStation(b.cfg.MyCall) {
		b.notForUs.Add(1)
		return
	}
	h, err := parseIPv4(f.Info, true)
	if err != nil {
		b.badPacket.Add(1)
		return
	}
	// Only packets for the own address reach the system: a station must not
	// use this computer to get into other networks, even when the system
	// forwards packets (Docker, for example, switches that on).
	if h.dst != b.cfg.Local.Addr() && !(b.cfg.AllowBroadcast && b.isBroadcast(h.dst)) {
		b.wrongAddress.Add(1)
		return
	}
	if b.cfg.Filter != nil && !b.cfg.Filter.Inbound(f.Info[:h.totalLen], f.Src) {
		b.blocked.Add(1)
		return
	}
	pkt := append([]byte(nil), f.Info[:h.totalLen]...)
	// Keep the other side's TCP segments within the radio's MTU too.
	if clampMSS(pkt, b.cfg.LinkMTU-40) {
		b.clamped.Add(1)
	}
	if err := b.deliver(pkt); err != nil {
		b.deliverErrors.Add(1)
		return
	}
	b.fromRadio.Add(1)
}

func (b *Bridge) route(dst netip.Addr) (ax25.Address, bool) {
	best := -1
	var call ax25.Address
	for _, r := range b.cfg.Routes {
		if r.Prefix.Bits() > best && r.Prefix.Contains(dst) {
			best, call = r.Prefix.Bits(), r.Call
		}
	}
	if best < 32 && b.cfg.Learned != nil {
		if c, ok := b.cfg.Learned(dst); ok {
			return c, true
		}
	}
	return call, best >= 0
}

func (b *Bridge) isBroadcast(a netip.Addr) bool {
	if a.IsMulticast() || a == netip.AddrFrom4([4]byte{255, 255, 255, 255}) {
		return true
	}
	if p := b.cfg.Local; p.IsValid() && p.Bits() < 31 {
		return a == broadcastOf(p)
	}
	return false
}

func broadcastOf(p netip.Prefix) netip.Addr {
	a := p.Masked().Addr().As4()
	host := ^uint32(0) >> p.Bits()
	v := binary.BigEndian.Uint32(a[:]) | host
	binary.BigEndian.PutUint32(a[:], v)
	return netip.AddrFrom4(a)
}

var (
	errNotIPv4  = errors.New("not IPv4")
	errMalform  = errors.New("malformed IPv4 header")
	errChecksum = errors.New("IPv4 header checksum")
)

type ipv4Header struct {
	totalLen int
	src, dst netip.Addr
	proto    byte
}

// parseIPv4 checks the header of an IPv4 packet; trailing bytes beyond the
// total length are allowed (link padding) and cut off by the caller.
func parseIPv4(p []byte, verifyChecksum bool) (ipv4Header, error) {
	if len(p) < 1 || p[0]>>4 != 4 {
		return ipv4Header{}, errNotIPv4
	}
	if len(p) < 20 {
		return ipv4Header{}, errMalform
	}
	ihl := int(p[0]&0x0F) * 4
	total := int(binary.BigEndian.Uint16(p[2:4]))
	if ihl < 20 || total < ihl || total > len(p) {
		return ipv4Header{}, errMalform
	}
	if verifyChecksum && Checksum(p[:ihl]) != 0 {
		return ipv4Header{}, errChecksum
	}
	return ipv4Header{
		totalLen: total,
		src:      netip.AddrFrom4([4]byte(p[12:16])),
		dst:      netip.AddrFrom4([4]byte(p[16:20])),
		proto:    p[9],
	}, nil
}

// Checksum returns the Internet checksum (RFC 1071) of b. Over a header that
// contains its own checksum the result is 0.
func Checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xFFFF + sum>>16
	}
	return ^uint16(sum)
}

// SPDX-License-Identifier: GPL-3.0-or-later

package firewall

import (
	"encoding/binary"
	"net/netip"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// Answers to dropped packets are limited: one per station every refuseGap,
// and refuseBurst in any minute for all stations together.
const (
	refuseGap   = 2 * time.Second
	refuseBurst = 10
)

// Refusal returns the answer to a packet that Inbound dropped: a TCP reset
// for TCP, ICMP "communication administratively prohibited" for the rest.
// The other station's program then fails at once ("connection refused")
// instead of repeating its attempt over the air for a minute or two, which
// also costs less airtime than staying silent. It returns nil when the
// firewall is silent, when the packet must not be answered (an ICMP message
// other than an echo request, a TCP reset, a later fragment, a source that
// is not a single station) or when the station got an answer moments ago.
// The caller sends the answer only for packets to the own address.
func (f *Firewall) Refusal(pkt []byte, from ax25.Address) []byte {
	if f.cfg.Load().Silent {
		return nil
	}
	h, ok := header(pkt)
	if !ok || h.offset != 0 || !h.src.Is4() || h.src.IsUnspecified() || h.src.IsMulticast() || h.src.IsLoopback() ||
		h.src == broadcastAddr {
		return nil
	}
	var answer []byte
	switch h.proto {
	case 6:
		answer = reset(pkt, h)
	case 1:
		if len(h.l4) >= 8 && h.l4[0] == 8 { // only an echo request; never answer an ICMP error
			answer = prohibited(pkt, h)
		}
	default:
		answer = prohibited(pkt, h)
	}
	if answer == nil {
		return nil
	}
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	key := from.String()
	if t, ok := f.answered[key]; ok && now.Sub(t) < refuseGap {
		return nil
	}
	if len(f.answers) == refuseBurst && now.Sub(f.answers[0]) < time.Minute {
		return nil
	}
	if len(f.answered) >= 1024 {
		for k, t := range f.answered {
			if now.Sub(t) >= refuseGap {
				delete(f.answered, k)
			}
		}
	}
	f.answered[key] = now
	if len(f.answers) == refuseBurst {
		f.answers = append(f.answers[:0], f.answers[1:]...)
	}
	f.answers = append(f.answers, now)
	f.refused.Add(1)
	return answer
}

var broadcastAddr = netip.AddrFrom4([4]byte{255, 255, 255, 255})

// reset answers a TCP segment as a closed port does (RFC 9293, 3.10.7.1).
func reset(pkt []byte, h ipv4) []byte {
	if len(h.l4) < 20 {
		return nil
	}
	in := h.l4
	flags := in[13]
	if flags&0x04 != 0 { // never answer a reset
		return nil
	}
	seg := make([]byte, 20)
	copy(seg[0:2], in[2:4]) // ports swapped
	copy(seg[2:4], in[0:2])
	seg[12] = 5 << 4
	if flags&0x10 != 0 { // ACK set: the reset takes its acknowledgment number as sequence
		copy(seg[4:8], in[8:12])
		seg[13] = 0x04
	} else {
		dataOff := int(in[12]>>4) * 4
		n := uint32(0)
		if dataOff >= 20 && dataOff <= len(in) {
			n = uint32(len(in) - dataOff)
		}
		if flags&0x02 != 0 { // SYN
			n++
		}
		if flags&0x01 != 0 { // FIN
			n++
		}
		binary.BigEndian.PutUint32(seg[8:12], binary.BigEndian.Uint32(in[4:8])+n)
		seg[13] = 0x14 // RST, ACK
	}
	out := ipPacket(h.dst.As4(), h.src.As4(), 6, seg)
	binary.BigEndian.PutUint16(out[36:38], transportSum(out, 6))
	return out
}

// prohibited is ICMP destination unreachable, code 13 (RFC 1812, 5.2.7.1),
// with the dropped packet's header and its first 8 bytes of data.
func prohibited(pkt []byte, h ipv4) []byte {
	ihl := int(pkt[0]&0x0F) * 4
	quote := min(len(pkt), ihl+8, int(be16(pkt[2:])))
	msg := make([]byte, 8+quote)
	msg[0], msg[1] = 3, 13
	copy(msg[8:], pkt[:quote])
	binary.BigEndian.PutUint16(msg[2:4], checksum(msg))
	return ipPacket(h.dst.As4(), h.src.As4(), 1, msg)
}

// ipPacket wraps a transport segment in an IPv4 header with TTL 64.
func ipPacket(src, dst [4]byte, proto uint8, l4 []byte) []byte {
	p := make([]byte, 20+len(l4))
	p[0], p[8], p[9] = 0x45, 64, proto
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
	copy(p[12:16], src[:])
	copy(p[16:20], dst[:])
	binary.BigEndian.PutUint16(p[10:12], checksum(p[:20]))
	copy(p[20:], l4)
	return p
}

// transportSum is the TCP or UDP checksum over the pseudo-header.
func transportSum(p []byte, proto uint8) uint16 {
	l4 := p[20:]
	ph := make([]byte, 12, 12+len(l4))
	copy(ph[0:8], p[12:20])
	ph[9] = proto
	binary.BigEndian.PutUint16(ph[10:12], uint16(len(l4)))
	return checksum(append(ph, l4...))
}

// checksum is the Internet checksum (RFC 1071).
func checksum(b []byte) uint16 {
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

// SPDX-License-Identifier: GPL-3.0-or-later

package ipbridge

import (
	"encoding/binary"
	"errors"
)

// DefaultLinkMTU is the largest IP packet on the radio: the classic AX.25
// information field of 256 bytes.
const DefaultLinkMTU = 256

const (
	flagDF     = 0x4000
	flagMF     = 0x2000
	offsetMask = 0x1FFF
)

var errTooSmall = errors.New("MTU too small for this header")

// clampMSS lowers the MSS option of a TCP SYN to limit and corrects the TCP
// checksum, like a router in front of a link with a small MTU. It reports
// whether it changed the packet. pkt must hold one IPv4 packet.
func clampMSS(pkt []byte, limit int) bool {
	if len(pkt) < 20 || pkt[9] != 6 || binary.BigEndian.Uint16(pkt[6:8])&offsetMask != 0 {
		return false // not TCP, or not the first fragment
	}
	ihl := int(pkt[0]&0x0F) * 4
	total := int(binary.BigEndian.Uint16(pkt[2:4]))
	if ihl < 20 || total > len(pkt) || total < ihl+20 {
		return false
	}
	tcp := pkt[ihl:total]
	if tcp[13]&0x02 == 0 { // SYN
		return false
	}
	dataOff := int(tcp[12]>>4) * 4
	if dataOff < 20 || dataOff > len(tcp) {
		return false
	}
	opts := tcp[20:dataOff]
	for i := 0; i < len(opts); {
		switch opts[i] {
		case 0: // end of options
			return false
		case 1: // no-operation
			i++
			continue
		}
		if i+1 >= len(opts) {
			return false
		}
		l := int(opts[i+1])
		if l < 2 || i+l > len(opts) {
			return false
		}
		if opts[i] == 2 && l == 4 { // MSS
			if int(binary.BigEndian.Uint16(opts[i+2:])) <= limit {
				return false
			}
			binary.BigEndian.PutUint16(opts[i+2:], uint16(limit))
			tcp[16], tcp[17] = 0, 0
			binary.BigEndian.PutUint16(tcp[16:], transportChecksum(pkt[12:16], pkt[16:20], 6, tcp))
			return true
		}
		i += l
	}
	return false
}

// transportChecksum is the TCP/UDP checksum over the IPv4 pseudo-header and
// the segment (with its checksum field zeroed by the caller).
func transportChecksum(src, dst []byte, proto byte, seg []byte) uint16 {
	pseudo := make([]byte, 12, 12+len(seg))
	copy(pseudo[0:4], src)
	copy(pseudo[4:8], dst)
	pseudo[9] = proto
	binary.BigEndian.PutUint16(pseudo[10:], uint16(len(seg)))
	return Checksum(append(pseudo, seg...))
}

// fragment splits an IPv4 packet whose DF bit is clear into fragments of at
// most mtu bytes (RFC 791). An existing fragment keeps its offset and its
// MF bit on the last piece. Options with the copy flag go into every
// fragment, all options into the first.
func fragment(pkt []byte, mtu int) ([][]byte, error) {
	ihl := int(pkt[0]&0x0F) * 4
	total := int(binary.BigEndian.Uint16(pkt[2:4]))
	payload := pkt[ihl:total]
	field := binary.BigEndian.Uint16(pkt[6:8])
	baseOff := int(field & offsetMask)
	moreAfter := field&flagMF != 0
	if baseOff*8+len(payload) > 0xFFFF {
		return nil, errMalform // would not fit into a datagram; offsets would overflow
	}
	first := pkt[:ihl]
	later := copiedHeader(first)

	var out [][]byte
	for off := 0; off < len(payload); {
		hdr := first
		if off > 0 {
			hdr = later
		}
		room := (mtu - len(hdr)) &^ 7
		if room <= 0 {
			return nil, errTooSmall
		}
		n := min(room, len(payload)-off)
		last := off+n == len(payload)
		f := make([]byte, len(hdr)+n)
		copy(f, hdr)
		copy(f[len(hdr):], payload[off:off+n])
		f[0] = 0x40 | byte(len(hdr)/4)
		binary.BigEndian.PutUint16(f[2:], uint16(len(f)))
		v := uint16(baseOff + off/8)
		if !last || moreAfter {
			v |= flagMF
		}
		binary.BigEndian.PutUint16(f[6:], v)
		f[10], f[11] = 0, 0
		binary.BigEndian.PutUint16(f[10:], Checksum(f[:len(hdr)]))
		out = append(out, f)
		off += n
	}
	return out, nil
}

// copiedHeader returns the header for the second and later fragments: the
// fixed part plus the options whose copy flag is set, padded to 32 bits.
func copiedHeader(hdr []byte) []byte {
	out := append([]byte(nil), hdr[:20]...)
	opts := hdr[20:]
	for i := 0; i < len(opts); {
		t := opts[i]
		if t == 0 {
			break
		}
		if t == 1 {
			i++
			continue
		}
		if i+1 >= len(opts) {
			break
		}
		l := int(opts[i+1])
		if l < 2 || i+l > len(opts) {
			break
		}
		if t&0x80 != 0 {
			out = append(out, opts[i:i+l]...)
		}
		i += l
	}
	for len(out)%4 != 0 {
		out = append(out, 0)
	}
	return out
}

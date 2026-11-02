// SPDX-License-Identifier: GPL-3.0-or-later

package ipbridge

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"sort"
	"testing"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// ipv4 builds an IPv4 header (with options, if any) around payload.
func ipv4(src, dst string, proto byte, options, payload []byte, flags uint16) []byte {
	ihl := 20 + len(options)
	p := make([]byte, ihl+len(payload))
	p[0] = 0x40 | byte(ihl/4)
	binary.BigEndian.PutUint16(p[2:], uint16(len(p)))
	binary.BigEndian.PutUint16(p[4:], 0x1234)
	binary.BigEndian.PutUint16(p[6:], flags)
	p[8], p[9] = 64, proto
	s, d := netip.MustParseAddr(src).As4(), netip.MustParseAddr(dst).As4()
	copy(p[12:], s[:])
	copy(p[16:], d[:])
	copy(p[20:], options)
	copy(p[ihl:], payload)
	binary.BigEndian.PutUint16(p[10:], Checksum(p[:ihl]))
	return p
}

// tcpSegment builds a TCP segment with the given flags and options and a valid
// checksum for src/dst.
func tcpSegment(src, dst string, flags byte, opts []byte) []byte {
	seg := make([]byte, 20+len(opts))
	binary.BigEndian.PutUint16(seg[0:], 40000)
	binary.BigEndian.PutUint16(seg[2:], 80)
	seg[12] = byte((20+len(opts))/4) << 4
	seg[13] = flags
	binary.BigEndian.PutUint16(seg[14:], 64240)
	copy(seg[20:], opts)
	s, d := netip.MustParseAddr(src).As4(), netip.MustParseAddr(dst).As4()
	binary.BigEndian.PutUint16(seg[16:], transportChecksum(s[:], d[:], 6, seg))
	return seg
}

// synOptions: MSS, NOP, window scale, SACK permitted, timestamps, as Linux sends them.
func synOptions(mss uint16) []byte {
	return []byte{2, 4, byte(mss >> 8), byte(mss), 1, 3, 3, 7, 4, 2, 8, 10, 0, 0, 0, 1, 0, 0, 0, 0}
}

func tcpOK(t *testing.T, pkt []byte) {
	t.Helper()
	ihl := int(pkt[0]&0x0F) * 4
	if transportChecksum(pkt[12:16], pkt[16:20], 6, pkt[ihl:]) != 0 {
		t.Fatal("TCP checksum invalid")
	}
}

func mssOf(pkt []byte) int {
	ihl := int(pkt[0]&0x0F) * 4
	return int(binary.BigEndian.Uint16(pkt[ihl+22:]))
}

func TestClampMSSLowersItAndFixesTheChecksum(t *testing.T) {
	syn := ipv4("10.73.0.1", "10.73.0.2", 6, nil, tcpSegment("10.73.0.1", "10.73.0.2", 0x02, synOptions(536)), flagDF)
	tcpOK(t, syn)
	if !clampMSS(syn, 216) || mssOf(syn) != 216 {
		t.Fatalf("MSS %d", mssOf(syn))
	}
	tcpOK(t, syn)
	if Checksum(syn[:20]) != 0 {
		t.Fatal("the IPv4 header checksum must stay valid")
	}
	if clampMSS(syn, 216) {
		t.Fatal("a second clamp must not change anything")
	}
}

func TestClampMSSLeavesOtherPacketsAlone(t *testing.T) {
	ack := ipv4("10.73.0.1", "10.73.0.2", 6, nil, tcpSegment("10.73.0.1", "10.73.0.2", 0x10, nil), flagDF)
	small := ipv4("10.73.0.1", "10.73.0.2", 6, nil, tcpSegment("10.73.0.1", "10.73.0.2", 0x02, synOptions(200)), flagDF)
	udp := udpPacket("10.73.0.1", "10.73.0.2", "x")
	broken := ipv4("10.73.0.1", "10.73.0.2", 6, nil, tcpSegment("10.73.0.1", "10.73.0.2", 0x02, []byte{2, 9, 1, 0}), flagDF)
	for name, p := range map[string][]byte{"ACK": ack, "small MSS": small, "UDP": udp, "broken option": broken} {
		before := append([]byte(nil), p...)
		if clampMSS(p, 216) || !bytes.Equal(p, before) {
			t.Errorf("%s changed", name)
		}
	}
}

// reassemble checks the fragments and returns the payload they carry.
func reassemble(t *testing.T, frags [][]byte, mtu int) (payload []byte, lastMF bool) {
	t.Helper()
	sort.Slice(frags, func(i, j int) bool {
		return binary.BigEndian.Uint16(frags[i][6:])&offsetMask < binary.BigEndian.Uint16(frags[j][6:])&offsetMask
	})
	next := -1
	for i, f := range frags {
		if len(f) > mtu {
			t.Fatalf("fragment %d has %d bytes", i, len(f))
		}
		ihl := int(f[0]&0x0F) * 4
		if Checksum(f[:ihl]) != 0 || int(binary.BigEndian.Uint16(f[2:])) != len(f) {
			t.Fatalf("fragment %d: bad header", i)
		}
		v := binary.BigEndian.Uint16(f[6:])
		off := int(v&offsetMask) * 8
		if next >= 0 && off != next {
			t.Fatalf("fragment %d at %d, want %d", i, off, next)
		}
		if i < len(frags)-1 && v&flagMF == 0 {
			t.Fatalf("fragment %d without MF", i)
		}
		if i < len(frags)-1 && (len(f)-ihl)%8 != 0 {
			t.Fatalf("fragment %d payload not a multiple of 8", i)
		}
		payload = append(payload, f[ihl:]...)
		next = off + len(f) - ihl
		lastMF = v&flagMF != 0
	}
	return payload, lastMF
}

func TestFragmentRoundTrip(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 40) // 640 bytes
	pkt := ipv4("10.73.0.1", "10.73.0.2", 17, nil, data, 0)
	frags, err := fragment(pkt, 256)
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 3 {
		t.Fatalf("%d fragments", len(frags))
	}
	got, lastMF := reassemble(t, frags, 256)
	if !bytes.Equal(got, data) || lastMF {
		t.Fatalf("reassembled %d bytes, last MF %v", len(got), lastMF)
	}
}

func TestFragmentOfAFragmentKeepsOffsetAndMF(t *testing.T) {
	data := bytes.Repeat([]byte{0xAB}, 400)
	pkt := ipv4("10.73.0.1", "10.73.0.2", 17, nil, data, flagMF|10) // offset 80 bytes, more follow
	frags, err := fragment(pkt, 256)
	if err != nil {
		t.Fatal(err)
	}
	if off := binary.BigEndian.Uint16(frags[0][6:]) & offsetMask; off != 10 {
		t.Fatalf("first offset %d, want 10", off)
	}
	if _, lastMF := reassemble(t, frags, 256); !lastMF {
		t.Fatal("the last piece must keep MF: the original fragment was not the last")
	}
}

func TestFragmentRefusesOffsetsBeyondTheDatagram(t *testing.T) {
	pkt := ipv4("10.73.0.1", "10.73.0.2", 17, nil, bytes.Repeat([]byte{1}, 400), offsetMask) // offset 65528
	if _, err := fragment(pkt, 256); err == nil {
		t.Fatal("accepted a fragment that ends beyond 65535 bytes")
	}
}

func TestFragmentCopiesOnlyCopiedOptions(t *testing.T) {
	copied := []byte{0x83, 7, 4, 10, 73, 0, 9}                                    // loose source route: copy flag set
	notCopied := []byte{0x07, 7, 4, 0, 0, 0, 0}                                   // record route: copy flag clear
	options := append(append(append([]byte(nil), copied...), notCopied...), 0, 0) // 16 bytes
	pkt := ipv4("10.73.0.1", "10.73.0.2", 17, options, bytes.Repeat([]byte{1}, 500), 0)
	frags, err := fragment(pkt, 256)
	if err != nil {
		t.Fatal(err)
	}
	if ihl := int(frags[0][0]&0x0F) * 4; ihl != 36 || !bytes.Equal(frags[0][20:36], options) {
		t.Fatalf("first fragment header %d bytes", ihl)
	}
	for _, f := range frags[1:] {
		ihl := int(f[0]&0x0F) * 4
		if ihl != 28 || !bytes.Equal(f[20:27], copied) {
			t.Fatalf("later fragment options % X", f[20:ihl])
		}
	}
	if got, _ := reassemble(t, frags, 256); len(got) != 500 {
		t.Fatalf("payload %d bytes", len(got))
	}
}

func TestBridgeFragmentsLargePacketsWithoutDF(t *testing.T) {
	h := newHarness(t, stationA())
	pkt := ipv4("10.73.0.1", "10.73.0.2", 17, nil, bytes.Repeat([]byte{7}, 600), 0)
	h.b.FromAdapter(pkt)
	if len(h.sent) != 3 || h.b.Stats().Fragmented != 1 {
		t.Fatalf("%d frames, stats %+v", len(h.sent), h.b.Stats())
	}
	var frags [][]byte
	for _, fr := range h.sent {
		f, err := ax25.Decode(fr)
		if err != nil || f.PID != ax25.PIDIPv4 || len(f.Info) > DefaultLinkMTU {
			t.Fatalf("frame %v %+v", err, f)
		}
		frags = append(frags, f.Info)
	}
	if got, _ := reassemble(t, frags, DefaultLinkMTU); len(got) != 600 {
		t.Fatalf("payload %d bytes", len(got))
	}
}

func TestBridgeDropsLargePacketsWithDF(t *testing.T) {
	h := newHarness(t, stationA())
	h.b.FromAdapter(ipv4("10.73.0.1", "10.73.0.2", 17, nil, bytes.Repeat([]byte{7}, 600), flagDF))
	if len(h.sent) != 0 || h.b.Stats().TooBig != 1 {
		t.Fatalf("%d frames, stats %+v", len(h.sent), h.b.Stats())
	}
}

func TestBridgeClampsTheMSSBothWays(t *testing.T) {
	h := newHarness(t, stationA())
	// Out: the local system (e.g. Windows with adapter MTU 576) offers MSS 536.
	h.b.FromAdapter(ipv4("10.73.0.1", "10.73.0.2", 6, nil, tcpSegment("10.73.0.1", "10.73.0.2", 0x02, synOptions(536)), flagDF))
	f, err := ax25.Decode(h.sent[0])
	if err != nil {
		t.Fatal(err)
	}
	if mssOf(f.Info) != DefaultLinkMTU-40 {
		t.Fatalf("outgoing MSS %d", mssOf(f.Info))
	}
	tcpOK(t, f.Info)
	// In: the other station offers MSS 1460.
	in := ipv4("10.73.0.2", "10.73.0.1", 6, nil, tcpSegment("10.73.0.2", "10.73.0.1", 0x12, synOptions(1460)), flagDF)
	h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, in))
	if len(h.delivered) != 1 || mssOf(h.delivered[0]) != DefaultLinkMTU-40 {
		t.Fatalf("delivered %v", h.delivered)
	}
	tcpOK(t, h.delivered[0])
	if s := h.b.Stats(); s.Clamped != 2 {
		t.Fatalf("stats %+v", s)
	}
}

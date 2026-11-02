// SPDX-License-Identifier: GPL-3.0-or-later

package firewall

import (
	"encoding/binary"
	"testing"
	"time"
)

// syn is a TCP SYN with sequence number seq.
func syn(src, dst string, sport, dport uint16, seq uint32) []byte {
	b := ports(sport, dport)
	binary.BigEndian.PutUint32(b[4:], seq)
	b[12], b[13] = 5<<4, 0x02
	return packet(src, dst, 6, 7, 0, b)
}

func TestARefusedConnectionGetsAReset(t *testing.T) {
	f := protect()
	in := syn(peer, me, 40000, 22, 1000)
	if f.Inbound(in, peerCall) {
		t.Fatal("passed")
	}
	out := f.Refusal(in, peerCall)
	h, ok := header(out)
	if !ok || h.proto != 6 || h.src.String() != me || h.dst.String() != peer || checksum(out[:20]) != 0 {
		t.Fatalf("not an IPv4 TCP packet from us to the sender: % x", out)
	}
	seg := h.l4
	if be16(seg[0:]) != 22 || be16(seg[2:]) != 40000 || seg[13] != 0x14 ||
		binary.BigEndian.Uint32(seg[8:]) != 1001 || transportSum(out, 6) != 0 {
		t.Fatalf("not RST+ACK for seq 1000 with a good checksum: % x", seg)
	}
	if f.Stats().Refused != 1 {
		t.Fatalf("%+v", f.Stats())
	}
}

func TestASegmentWithAckGetsAResetAtThatNumber(t *testing.T) {
	f := protect()
	b := ports(40000, 22)
	binary.BigEndian.PutUint32(b[8:], 5555)
	b[12], b[13] = 5<<4, 0x10
	h, _ := header(f.Refusal(packet(peer, me, 6, 7, 0, b), peerCall))
	if h.l4[13] != 0x04 || binary.BigEndian.Uint32(h.l4[4:]) != 5555 {
		t.Fatalf("% x", h.l4)
	}
}

func TestUDPAndPingGetICMPProhibited(t *testing.T) {
	f := New(Config{Mode: ModeProtect}, nil)
	for i, in := range [][]byte{udp(peer, me, 5000, 161), icmp(peer, me, 8, 9)} {
		f.now = func() time.Time { return time.Unix(int64(100*i), 0) }
		out := f.Refusal(in, peerCall)
		h, ok := header(out)
		if !ok || h.proto != 1 || h.l4[0] != 3 || h.l4[1] != 13 || checksum(h.l4) != 0 {
			t.Fatalf("%d: not ICMP 3/13: % x", i, out)
		}
		if string(h.l4[8:]) != string(in[:28]) {
			t.Fatalf("%d: the dropped packet's header and 8 bytes are not quoted", i)
		}
	}
}

func TestNotAnswered(t *testing.T) {
	f := protect()
	rst := ports(40000, 22)
	rst[12], rst[13] = 5<<4, 0x04
	for name, in := range map[string][]byte{
		"a reset":          packet(peer, me, 6, 7, 0, rst),
		"an ICMP error":    icmp(peer, me, 3, 0),
		"an echo reply":    icmp(peer, me, 0, 9),
		"a later fragment": packet(peer, me, 17, 7, 100, ports(1, 2)[:8]),
		"from a broadcast": udp("255.255.255.255", me, 5000, 161),
		"from 0.0.0.0":     udp("0.0.0.0", me, 68, 67),
		"from multicast":   udp("224.0.0.251", me, 5353, 5353),
		"not an IP packet": []byte{0x60, 0, 0, 0},
	} {
		if out := f.Refusal(in, peerCall); out != nil {
			t.Errorf("%s was answered", name)
		}
	}
	if f.Stats().Refused != 0 {
		t.Fatalf("%+v", f.Stats())
	}
}

func TestSilentAnswersNothing(t *testing.T) {
	f := New(Config{Mode: ModeProtect, Silent: true}, nil)
	if f.Refusal(syn(peer, me, 40000, 22, 1), peerCall) != nil {
		t.Fatal("a silent firewall answered")
	}
}

func TestAnswersAreRateLimited(t *testing.T) {
	f := protect()
	now := time.Unix(1000, 0)
	f.now = func() time.Time { return now }
	if f.Refusal(syn(peer, me, 40000, 22, 1), peerCall) == nil {
		t.Fatal("the first was not answered")
	}
	if f.Refusal(syn(peer, me, 40001, 22, 1), peerCall) != nil {
		t.Fatal("a second within 2 s from the same station was answered")
	}
	if f.Refusal(syn(third, me, 40000, 22, 1), otherCall) == nil {
		t.Fatal("another station was not answered")
	}
	now = now.Add(refuseGap)
	if f.Refusal(syn(peer, me, 40002, 22, 1), peerCall) == nil {
		t.Fatal("not answered after the gap")
	}
	// A scan from many callsigns: at most refuseBurst answers a minute.
	g := protect()
	g.now = func() time.Time { return now }
	n := 0
	for i := 0; i < 3*refuseBurst; i++ {
		call := peerCall
		call.SSID = uint8(i % 16)
		call.Call = []string{"TESTA", "TESTB", "TESTC"}[i/16%3]
		if g.Refusal(syn(peer, me, uint16(40000+i), 22, 1), call) != nil {
			n++
		}
	}
	if n != refuseBurst {
		t.Fatalf("%d answers in one minute, want %d", n, refuseBurst)
	}
	now = now.Add(time.Minute)
	if g.Refusal(syn(peer, me, 1, 22, 1), otherCall) == nil {
		t.Fatal("still silent a minute later")
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package firewall

import (
	"encoding/binary"
	"net/netip"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

const me, peer, third = "10.73.0.1", "10.73.0.2", "10.73.0.3"

// packet builds an IPv4 packet with the given transport bytes.
func packet(src, dst string, proto uint8, id, frag uint16, l4 []byte) []byte {
	p := make([]byte, 20+len(l4))
	p[0], p[8], p[9] = 0x45, 64, proto
	binary.BigEndian.PutUint16(p[2:], uint16(len(p)))
	binary.BigEndian.PutUint16(p[4:], id)
	binary.BigEndian.PutUint16(p[6:], frag)
	s, d := netip.MustParseAddr(src).As4(), netip.MustParseAddr(dst).As4()
	copy(p[12:], s[:])
	copy(p[16:], d[:])
	copy(p[20:], l4)
	return p
}

func ports(sport, dport uint16) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint16(b, sport)
	binary.BigEndian.PutUint16(b[2:], dport)
	return b
}

func tcp(src, dst string, sport, dport uint16) []byte {
	return packet(src, dst, 6, 1, 0, ports(sport, dport))
}
func udp(src, dst string, sport, dport uint16) []byte {
	return packet(src, dst, 17, 1, 0, ports(sport, dport)[:8])
}

func icmp(src, dst string, typ uint8, ident uint16) []byte {
	b := make([]byte, 8)
	b[0] = typ
	binary.BigEndian.PutUint16(b[4:], ident)
	return packet(src, dst, 1, 1, 0, b)
}

var (
	peerCall  = ax25.MustAddress("TESTB-1")
	otherCall = ax25.MustAddress("TESTC-1")
)

func protect(rules ...Rule) *Firewall {
	return New(Config{Mode: ModeProtect, Ping: true, Rules: rules}, nil)
}

func TestAnswersToOwnConnectionsPass(t *testing.T) {
	f := protect()
	f.Outbound(tcp(me, peer, 40000, 80))
	if !f.Inbound(tcp(peer, me, 80, 40000), peerCall) {
		t.Fatal("the answer to our connection was dropped")
	}
	if f.Inbound(tcp(peer, me, 80, 40001), peerCall) || f.Inbound(tcp(peer, me, 22, 40000), peerCall) {
		t.Fatal("a packet that answers nothing passed")
	}
	if f.Inbound(tcp(third, me, 80, 40000), otherCall) {
		t.Fatal("another address answered our connection")
	}
	f.Outbound(udp(me, peer, 5353, 53))
	if !f.Inbound(udp(peer, me, 53, 5353), peerCall) {
		t.Fatal("the UDP answer was dropped")
	}
}

func TestUnsolicitedTrafficIsBlockedAndListed(t *testing.T) {
	f := protect()
	if f.Inbound(tcp(peer, me, 50000, 22), peerCall) || f.Inbound(udp(peer, me, 50000, 161), peerCall) {
		t.Fatal("unsolicited traffic passed")
	}
	r := f.Recent()
	if len(r) != 2 || r[0].Proto != "udp" || r[0].Port != 161 || r[1].Call != "TESTB-1" || r[1].Port != 22 {
		t.Fatalf("recent %+v", r)
	}
	if s := f.Stats(); s.Blocked != 2 || s.Passed != 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestRulesOpenPortsForEveryoneOrSomeCallsigns(t *testing.T) {
	f := protect(Rule{Proto: "tcp", Port: 80}, Rule{Proto: "tcp", Port: 22, Calls: []string{"TESTB"}},
		Rule{Proto: "udp", Port: 7070, Calls: []string{"TESTC-2"}})
	if !f.Inbound(tcp(peer, me, 50000, 80), otherCall) {
		t.Fatal("an open port was blocked")
	}
	if !f.Inbound(tcp(peer, me, 50000, 22), peerCall) || !f.Inbound(tcp(peer, me, 50000, 22), ax25.MustAddress("TESTB-7")) {
		t.Fatal("a callsign without SSID must admit all its SSIDs")
	}
	if f.Inbound(tcp(peer, me, 50000, 22), otherCall) {
		t.Fatal("a station not in the rule passed")
	}
	if f.Inbound(udp(peer, me, 50000, 7070), otherCall) || !f.Inbound(udp(peer, me, 50000, 7070), ax25.MustAddress("TESTC-2")) {
		t.Fatal("a callsign with SSID must admit only that SSID")
	}
	if f.Inbound(udp(peer, me, 50000, 80), otherCall) {
		t.Fatal("a TCP rule opened UDP")
	}
}

func TestPortsTheProgramServes(t *testing.T) {
	f := New(Config{Mode: ModeProtect}, func() []Rule { return []Rule{{Proto: "tcp", Port: 8080}} })
	if !f.Inbound(tcp(peer, me, 50000, 8080), peerCall) {
		t.Fatal("the gateway port was blocked")
	}
}

func TestPingAndICMP(t *testing.T) {
	f := protect()
	if !f.Inbound(icmp(peer, me, 8, 7), peerCall) {
		t.Fatal("ping blocked")
	}
	if f.Inbound(icmp(peer, me, 0, 9), peerCall) {
		t.Fatal("an echo reply to nothing passed")
	}
	f.Outbound(icmp(me, peer, 8, 9))
	if !f.Inbound(icmp(peer, me, 0, 9), peerCall) {
		t.Fatal("the reply to our ping was blocked")
	}
	if !f.Inbound(icmp(peer, me, 3, 0), peerCall) || !f.Inbound(icmp(peer, me, 11, 0), peerCall) {
		t.Fatal("ICMP errors are needed and must pass")
	}
	if f.Inbound(icmp(peer, me, 13, 0), peerCall) { // timestamp request
		t.Fatal("other ICMP passed")
	}
	f.Set(Config{Mode: ModeProtect, Ping: false})
	if f.Inbound(icmp(peer, me, 8, 7), peerCall) {
		t.Fatal("ping passed although it is switched off")
	}
}

func TestLaterFragmentsFollowTheirFirst(t *testing.T) {
	f := protect()
	first := icmp(peer, me, 8, 7)
	binary.BigEndian.PutUint16(first[4:], 4242)   // IP ID
	binary.BigEndian.PutUint16(first[6:], 0x2000) // more fragments
	later := packet(peer, me, 1, 4242, 29, make([]byte, 40))
	stray := packet(peer, me, 1, 999, 29, make([]byte, 40))
	if !f.Inbound(first, peerCall) || !f.Inbound(later, peerCall) {
		t.Fatal("the fragments of a ping were blocked")
	}
	if f.Inbound(stray, peerCall) {
		t.Fatal("a fragment without its first one passed")
	}
}

func TestConnectionsExpire(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	f := protect()
	f.now = func() time.Time { return now }
	f.Outbound(udp(me, peer, 5000, 7070))
	now = now.Add(udpIdle + time.Second)
	if f.Inbound(udp(peer, me, 7070, 5000), peerCall) {
		t.Fatal("an answer after the idle time passed")
	}
}

func TestOpenModePassesEverything(t *testing.T) {
	f := New(Config{Mode: ModeOpen}, nil)
	if !f.Inbound(tcp(peer, me, 50000, 22), peerCall) || !f.Inbound(packet(peer, me, 47, 1, 0, nil), peerCall) {
		t.Fatal("open mode blocked")
	}
}

func TestParseRuleAndCheck(t *testing.T) {
	r, err := ParseRule("22/tcp=testb,TESTC-2")
	if err != nil || r.Proto != "tcp" || r.Port != 22 || len(r.Calls) != 2 || r.Calls[0] != "TESTB" {
		t.Fatalf("%+v %v", r, err)
	}
	for _, bad := range []string{"22", "22/icmp", "0/tcp", "x/tcp", "22/tcp=not a call"} {
		if _, err := ParseRule(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if (Config{Mode: "sometimes"}).Check() == nil {
		t.Error("a bad mode accepted")
	}
}

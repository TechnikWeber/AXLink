// SPDX-License-Identifier: GPL-3.0-or-later

package ipbridge

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"testing"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/txqueue"
)

// udpPacket builds an IPv4/UDP packet with a valid header checksum.
func udpPacket(src, dst string, payload string) []byte {
	p := make([]byte, 28+len(payload))
	p[0], p[8], p[9] = 0x45, 64, 17
	binary.BigEndian.PutUint16(p[2:], uint16(len(p)))
	s, d := netip.MustParseAddr(src).As4(), netip.MustParseAddr(dst).As4()
	copy(p[12:], s[:])
	copy(p[16:], d[:])
	binary.BigEndian.PutUint16(p[10:], Checksum(p[:20]))
	binary.BigEndian.PutUint16(p[20:], 12345)
	binary.BigEndian.PutUint16(p[22:], 7070)
	binary.BigEndian.PutUint16(p[24:], uint16(8+len(payload)))
	copy(p[28:], payload)
	return p
}

type harness struct {
	b         *Bridge
	sent      [][]byte
	meta      []txqueue.Meta
	delivered [][]byte
}

func newHarness(t *testing.T, cfg Config) *harness {
	t.Helper()
	h := &harness{}
	b, err := New(cfg,
		func(f []byte, m txqueue.Meta) bool {
			h.sent, h.meta = append(h.sent, f), append(h.meta, m)
			return true
		},
		func(p []byte) error { h.delivered = append(h.delivered, p); return nil })
	if err != nil {
		t.Fatal(err)
	}
	h.b = b
	return h
}

func stationA() Config {
	return Config{
		MyCall: ax25.MustAddress("TESTA-1"),
		Local:  netip.MustParsePrefix("10.73.0.1/24"),
		Routes: []Route{{netip.MustParsePrefix("10.73.0.2/32"), ax25.MustAddress("TESTB-1")}},
	}
}

func TestLearnedAddressesComeBetweenExactAndShorterRoutes(t *testing.T) {
	cfg := stationA()
	cfg.Routes = []Route{
		{netip.MustParsePrefix("10.73.0.0/24"), ax25.MustAddress("TESTGW")},
		{netip.MustParsePrefix("10.73.0.9/32"), ax25.MustAddress("TESTD-1")},
	}
	cfg.Learned = func(a netip.Addr) (ax25.Address, []ax25.Address, bool) {
		if a == netip.MustParseAddr("10.73.0.2") || a == netip.MustParseAddr("10.73.0.9") {
			return ax25.MustAddress("TESTB-1"), nil, true
		}
		return ax25.Address{}, nil, false
	}
	h := newHarness(t, cfg)
	for dst, want := range map[string]string{
		"10.73.0.2": "TESTB-1", // learned, before the /24
		"10.73.0.9": "TESTD-1", // the configured /32 comes first
		"10.73.0.7": "TESTGW",  // nothing learned: the /24
	} {
		h.sent = nil
		h.b.FromAdapter(udpPacket("10.73.0.1", dst, "x"))
		if len(h.sent) != 1 {
			t.Fatalf("%s: nothing sent", dst)
		}
		f, _ := ax25.Decode(h.sent[0])
		if f.Dest.String() != want {
			t.Errorf("%s went to %s, want %s", dst, f.Dest, want)
		}
	}
}

func TestPacketsForOtherAddressesNeverReachTheSystem(t *testing.T) {
	h := newHarness(t, stationA())
	for _, dst := range []string{"192.168.1.5", "10.73.0.9", "10.73.0.255"} {
		h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, udpPacket("10.73.0.2", dst, "x")))
	}
	if len(h.delivered) != 0 || h.b.Stats().WrongAddress != 3 {
		t.Fatalf("delivered %d, stats %+v", len(h.delivered), h.b.Stats())
	}
	cfg := stationA()
	cfg.AllowBroadcast = true
	h = newHarness(t, cfg)
	h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, udpPacket("10.73.0.2", "10.73.0.255", "x")))
	if len(h.delivered) != 1 {
		t.Fatal("with broadcasts allowed, a subnet broadcast must pass")
	}
}

type fakeFilter struct {
	out  int
	pass bool
	from string
}

func (f *fakeFilter) Outbound([]byte) { f.out++ }
func (f *fakeFilter) Inbound(_ []byte, from ax25.Address) bool {
	f.from = from.String()
	return f.pass
}

func TestTheFilterSeesBothDirections(t *testing.T) {
	ff := &fakeFilter{}
	cfg := stationA()
	cfg.Filter = ff
	h := newHarness(t, cfg)
	h.b.FromAdapter(udpPacket("10.73.0.1", "10.73.0.2", "out"))
	h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, udpPacket("10.73.0.2", "10.73.0.1", "in")))
	if ff.out != 1 || ff.from != "TESTB-1" || len(h.delivered) != 0 || h.b.Stats().Blocked != 1 {
		t.Fatalf("filter %+v, delivered %d, stats %+v", ff, len(h.delivered), h.b.Stats())
	}
	ff.pass = true
	h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, udpPacket("10.73.0.2", "10.73.0.1", "in")))
	if len(h.delivered) != 1 {
		t.Fatal("a packet the filter passes must be delivered")
	}
}

func TestKnownGoodPacketHasValidChecksum(t *testing.T) {
	// The IPv4/UDP packet from the v0.1 destination-filter tests (built independently).
	p := []byte{0x45, 0x00, 0x00, 0x20, 0x00, 0x01, 0x00, 0x00, 0x40, 0x11, 0x66, 0x38,
		0x0A, 0x49, 0x00, 0x01, 0x0A, 0x49, 0x00, 0x02}
	if Checksum(p) != 0 {
		t.Fatalf("checksum over a valid header = %#x", Checksum(p))
	}
}

func TestOutboundBecomesUIFrameWithPIDCC(t *testing.T) {
	h := newHarness(t, stationA())
	pkt := udpPacket("10.73.0.1", "10.73.0.2", "AXLink")
	h.b.FromAdapter(pkt)
	if len(h.sent) != 1 {
		t.Fatalf("stats %+v", h.b.Stats())
	}
	f, err := ax25.Decode(h.sent[0])
	if err != nil {
		t.Fatal(err)
	}
	if f.Dest.String() != "TESTB-1" || f.Src.String() != "TESTA-1" || !f.IsUI() || !f.IsCommand() ||
		f.PID != ax25.PIDIPv4 || !bytes.Equal(f.Info, pkt) {
		t.Fatalf("frame %+v", f)
	}
}

func TestOutboundFrameDoesNotAliasTheReadBuffer(t *testing.T) {
	h := newHarness(t, stationA())
	pkt := udpPacket("10.73.0.1", "10.73.0.2", "AXLink")
	h.b.FromAdapter(pkt)
	pkt[30] = 'X' // the adapter reuses its buffer for the next read
	f, _ := ax25.Decode(h.sent[0])
	if f.Info[30] == 'X' {
		t.Fatal("queued frame changed with the read buffer")
	}
}

func TestLongestPrefixWins(t *testing.T) {
	cfg := stationA()
	cfg.Routes = []Route{
		{netip.MustParsePrefix("10.73.0.0/24"), ax25.MustAddress("TESTGW")},
		{netip.MustParsePrefix("10.73.0.5/32"), ax25.MustAddress("TESTC-1")},
	}
	h := newHarness(t, cfg)
	h.b.FromAdapter(udpPacket("10.73.0.1", "10.73.0.5", "x"))
	h.b.FromAdapter(udpPacket("10.73.0.1", "10.73.0.7", "x"))
	h.b.FromAdapter(udpPacket("10.73.0.1", "10.74.0.1", "x")) // no route
	if len(h.sent) != 2 {
		t.Fatalf("sent %d, stats %+v", len(h.sent), h.b.Stats())
	}
	for i, want := range []string{"TESTC-1", "TESTGW"} {
		f, _ := ax25.Decode(h.sent[i])
		if f.Dest.String() != want {
			t.Errorf("packet %d went to %s, want %s", i, f.Dest, want)
		}
	}
	if h.b.Stats().NoRoute != 1 {
		t.Errorf("stats %+v", h.b.Stats())
	}
}

func TestBroadcastFilterIsOnByDefaultAndSwitchable(t *testing.T) {
	cfg := stationA()
	cfg.Routes = append(cfg.Routes, Route{netip.MustParsePrefix("0.0.0.0/0"), ax25.MustAddress("QST")})
	h := newHarness(t, cfg)
	for _, dst := range []string{"224.0.0.251", "239.255.255.250", "255.255.255.255", "10.73.0.255"} {
		h.b.FromAdapter(udpPacket("10.73.0.1", dst, "chatter"))
	}
	if len(h.sent) != 0 || h.b.Stats().Filtered != 4 {
		t.Fatalf("sent %d, stats %+v", len(h.sent), h.b.Stats())
	}
	cfg.AllowBroadcast = true
	h = newHarness(t, cfg)
	h.b.FromAdapter(udpPacket("10.73.0.1", "224.0.0.251", "wanted"))
	if len(h.sent) != 1 {
		t.Fatalf("with the filter off: sent %d", len(h.sent))
	}
}

func TestOutboundDropsIPv6AndJunk(t *testing.T) {
	h := newHarness(t, stationA())
	h.b.FromAdapter([]byte{0x60, 0, 0, 0}) // IPv6
	h.b.FromAdapter([]byte{0x45, 0, 0})    // truncated IPv4
	s := h.b.Stats()
	if len(h.sent) != 0 || s.NotIPv4 != 1 || s.BadPacket != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func frameTo(t *testing.T, dest string, pid byte, info []byte) []byte {
	t.Helper()
	b, err := ax25.NewUI(ax25.MustAddress(dest), ax25.MustAddress("TESTB-1"), nil, pid, info).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInboundOnlyForThisStation(t *testing.T) {
	h := newHarness(t, stationA())
	pkt := udpPacket("10.73.0.2", "10.73.0.1", "AXLink")
	h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, pkt))
	h.b.FromRadio(frameTo(t, "TESTC-1", ax25.PIDIPv4, pkt)) // heard, but for someone else
	h.b.FromRadio(frameTo(t, "TESTA", ax25.PIDIPv4, pkt))   // other SSID
	h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDNoL3, []byte(">APRS status")))
	if len(h.delivered) != 1 || !bytes.Equal(h.delivered[0], pkt) {
		t.Fatalf("delivered %d, stats %+v", len(h.delivered), h.b.Stats())
	}
	if s := h.b.Stats(); s.NotForUs != 2 || s.OtherPID != 1 || s.FromRadio != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func TestInboundValidatesThePacket(t *testing.T) {
	h := newHarness(t, stationA())
	good := udpPacket("10.73.0.2", "10.73.0.1", "AXLink")

	badSum := append([]byte(nil), good...)
	badSum[10] ^= 0xFF
	short := good[:len(good)-3] // total length says more than the frame carries
	padded := append(append([]byte(nil), good...), 0, 0)

	for _, p := range [][]byte{badSum, short, padded} {
		h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, p))
	}
	if len(h.delivered) != 1 || !bytes.Equal(h.delivered[0], good) {
		t.Fatalf("delivered %v, stats %+v", h.delivered, h.b.Stats())
	}
	if h.b.Stats().BadPacket != 2 {
		t.Fatalf("stats %+v", h.b.Stats())
	}
}

func TestBroadcastAddress(t *testing.T) {
	for in, want := range map[string]string{
		"10.73.0.1/24": "10.73.0.255",
		"10.73.4.9/22": "10.73.7.255",
		"10.0.0.1/8":   "10.255.255.255",
	} {
		if got := broadcastOf(netip.MustParsePrefix(in)); got.String() != want {
			t.Errorf("broadcastOf(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestOverARelay(t *testing.T) {
	cfg := stationA()
	cfg.Routes = nil
	cfg.Learned = func(a netip.Addr) (ax25.Address, []ax25.Address, bool) {
		return ax25.MustAddress("TESTC-1"), []ax25.Address{ax25.MustAddress("TESTB-1")}, a == netip.MustParseAddr("10.73.0.3")
	}
	h := newHarness(t, cfg)
	h.b.FromAdapter(udpPacket("10.73.0.1", "10.73.0.3", "to C"))
	f, err := ax25.Decode(h.sent[0])
	if err != nil || f.Dest.String() != "TESTC-1" || len(f.Path) != 1 || f.Path[0].String() != "TESTB-1" || f.Path[0].Bit7 {
		t.Fatalf("%v %+v", err, f)
	}
	// From C over B: taken only once B has repeated it.
	in := udpPacket("10.73.0.3", "10.73.0.1", "from C")
	frame := func(used bool) []byte {
		b := ax25.MustAddress("TESTB-1")
		b.Bit7 = used
		raw, _ := ax25.NewUI(ax25.MustAddress("TESTA-1"), ax25.MustAddress("TESTC-1"), []ax25.Address{b}, ax25.PIDIPv4, in).Encode()
		return raw
	}
	h.b.FromRadio(frame(false))
	if len(h.delivered) != 0 || h.b.Stats().InTransit != 1 {
		t.Fatalf("taken before the relay: %+v", h.b.Stats())
	}
	h.b.FromRadio(frame(true))
	if len(h.delivered) != 1 {
		t.Fatal("the relay's copy was not taken")
	}
}

type refusingFilter struct{ fakeFilter }

func (f *refusingFilter) Refusal(pkt []byte, from ax25.Address) []byte {
	return udpPacket("10.73.0.1", "10.73.0.2", "refused:"+from.String())
}

func TestARefusedPacketIsAnsweredToItsSender(t *testing.T) {
	cfg := stationA()
	cfg.AllowBroadcast = true
	cfg.Filter = &refusingFilter{}
	h := newHarness(t, cfg)
	h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, udpPacket("10.73.0.2", "10.73.0.1", "in")))
	if len(h.sent) != 1 || h.b.Stats().Refused != 1 || h.b.Stats().Blocked != 1 {
		t.Fatalf("sent %d, stats %+v", len(h.sent), h.b.Stats())
	}
	f, err := ax25.Decode(h.sent[0])
	if err != nil || f.Dest.String() != "TESTB-1" || f.Src.String() != "TESTA-1" || f.PID != ax25.PIDIPv4 ||
		!bytes.HasSuffix(f.Info, []byte("refused:TESTB-1")) {
		t.Fatalf("%v %+v", err, f)
	}
	// A refused broadcast is not answered.
	h.b.FromRadio(frameTo(t, "TESTA-1", ax25.PIDIPv4, udpPacket("10.73.0.2", "10.73.0.255", "in")))
	if len(h.sent) != 1 {
		t.Fatal("a broadcast was answered")
	}
}

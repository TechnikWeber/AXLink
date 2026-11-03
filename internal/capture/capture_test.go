// SPDX-License-Identifier: GPL-3.0-or-later

package capture

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

func ui(t *testing.T, dest, src string, path []ax25.Address, pid byte, info []byte) []byte {
	t.Helper()
	b, err := ax25.NewUI(ax25.MustAddress(dest), ax25.MustAddress(src), path, pid, info).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func syn() []byte {
	p := make([]byte, 40)
	p[0], p[9] = 0x45, 6
	binary.BigEndian.PutUint16(p[2:], 40)
	a, b := netip.MustParseAddr("10.73.0.1").As4(), netip.MustParseAddr("10.73.0.2").As4()
	copy(p[12:], a[:])
	copy(p[16:], b[:])
	binary.BigEndian.PutUint16(p[20:], 40000)
	binary.BigEndian.PutUint16(p[22:], 80)
	p[32], p[33] = 5<<4, 0x02
	return p
}

func TestEveryFrameIsDescribed(t *testing.T) {
	c, _ := New(Config{Call: ax25.MustAddress("TESTA-1")})
	used := ax25.MustAddress("TESTB-1")
	used.Bit7 = true
	c.Add(ui(t, "TESTB-1", "TESTA-1", nil, ax25.PIDIPv4, syn()), true)
	c.Add(ui(t, "TESTC-1", "TESTD-1", []ax25.Address{used}, ax25.PIDIPv4, syn()), false)
	c.Add(ui(t, "AXLINK", "TESTB-1", []ax25.Address{{Call: "AXHOP", SSID: 2}}, ax25.PIDNoL3, []byte("AXL1 ip=10.73.0.2/24 tags=RELAY")), false)
	c.Add(ui(t, "APZAXL", "TESTB-1", nil, ax25.PIDNoL3, []byte(":TESTA-1  :Hello{7")), false)
	c.Add(ui(t, "APZAXL", "TESTB-1", nil, ax25.PIDNoL3, []byte(":TESTC-1  :ack3")), false)
	c.Add(ui(t, "APZK5M", "TESTE-7", nil, ax25.PIDNoL3, []byte("!5230.00N/01323.00E-\x01")), false)
	c.Add(ui(t, "TESTA-1", "TESTB-1", nil, 0xCF, []byte("netrom")), false)
	c.Add([]byte{1, 2, 3}, false)
	got := c.Since(0)
	want := []struct {
		kind, text string
		mine       bool
	}{
		{KindIP, "10.73.0.1 → 10.73.0.2 TCP 40000 → 80 SYN, 0 B", true},
		{KindIP, "10.73.0.1 → 10.73.0.2 TCP 40000 → 80 SYN, 0 B", false},
		{KindBeacon, "AXL1 ip=10.73.0.2/24 tags=RELAY", false},
		{KindMessage, "to TESTA-1: Hello {7}", true},
		{KindMessage, "ack 3 to TESTC-1", false},
		{KindAPRS, "!5230.00N/01323.00E-·", false},
		{KindOther, "PID 0xCF, 6 bytes", true},
		{KindOther, "", false},
	}
	if len(got) != len(want) {
		t.Fatalf("%d entries", len(got))
	}
	for i, w := range want {
		g := got[i]
		if g.Kind != w.kind || (w.text != "" && g.Text != w.text) || g.Mine != w.mine || g.N != uint64(i+1) {
			t.Errorf("%d: %+v, want %+v", i, g, w)
		}
	}
	if !got[0].Out || got[1].Out || strings.Join(got[1].Via, ",") != "TESTB-1*" || got[2].Via[0] != "AXHOP-2" {
		t.Fatalf("%+v %+v %+v", got[0], got[1], got[2])
	}
}

func TestTheRingKeepsTheNewest(t *testing.T) {
	c, _ := New(Config{Call: ax25.MustAddress("TESTA-1")})
	f := ui(t, "TESTB-1", "TESTA-1", nil, ax25.PIDNoL3, []byte("x"))
	for range Keep + 20 {
		c.Add(f, true)
	}
	all := c.Since(0)
	if len(all) != Keep || all[0].N != 21 || all[Keep-1].N != Keep+20 {
		t.Fatalf("%d entries, %d..%d", len(all), all[0].N, all[len(all)-1].N)
	}
	if after := c.Since(Keep + 18); len(after) != 2 || after[0].N != Keep+19 {
		t.Fatalf("%+v", after)
	}
	if none := c.Since(Keep + 20); len(none) != 0 {
		t.Fatal("frames after the newest")
	}
}

func TestPcap(t *testing.T) {
	var buf bytes.Buffer
	at := time.Unix(1_800_000_000, 123456000)
	c, err := New(Config{Call: ax25.MustAddress("TESTA-1"), Pcap: &buf, Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	f := ui(t, "TESTB-1", "TESTA-1", nil, ax25.PIDIPv4, syn())
	c.Add(f, true)
	b := buf.Bytes()
	if binary.LittleEndian.Uint32(b[0:]) != 0xa1b2c3d4 || binary.LittleEndian.Uint32(b[20:]) != 3 || len(b) != 24+16+len(f) {
		t.Fatalf("% x", b[:24])
	}
	r := b[24:]
	if binary.LittleEndian.Uint32(r[0:]) != 1_800_000_000 || binary.LittleEndian.Uint32(r[4:]) != 123456 ||
		binary.LittleEndian.Uint32(r[8:]) != uint32(len(f)) || !bytes.Equal(r[16:], f) {
		t.Fatalf("% x", r[:16])
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

func TestMarshalAndParseRoundTrip(t *testing.T) {
	for want, b := range map[string]Beacon{
		"AXL1 ip=10.73.0.2/24":                             {Addr: netip.MustParsePrefix("10.73.0.2/24"), Every: DefaultEvery},
		"AXL1 ip=10.73.0.2/24 tags=INET,AUTO":              {Addr: netip.MustParsePrefix("10.73.0.2/24"), Every: DefaultEvery, Tags: []string{"INET", "AUTO"}},
		"AXL1 ip=44.128.1.7/32 every=300 tags=RELAY ask=1": {Addr: netip.MustParsePrefix("44.128.1.7/32"), Every: 5 * time.Minute, Tags: []string{"RELAY"}, Ask: true},
	} {
		got, err := b.Marshal()
		if err != nil || string(got) != want {
			t.Fatalf("Marshal %+v = %q, %v; want %q", b, got, err, want)
		}
		back, err := Parse(got)
		if err != nil || back.Addr != b.Addr || back.Every != b.Every || back.Ask != b.Ask || !slices.Equal(back.Tags, b.Tags) {
			t.Fatalf("Parse(%q) = %+v, %v", got, back, err)
		}
	}
}

func TestParseKeepsWhatLaterVersionsMayAdd(t *testing.T) {
	b, err := Parse([]byte("AXL1 tags=INET,MAIL9 note=hello=world ip=10.73.0.5/24"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.Tags, []string{"INET", "MAIL9"}) || b.Addr.String() != "10.73.0.5/24" {
		t.Fatalf("%+v", b)
	}
}

func TestParseRejects(t *testing.T) {
	for _, info := range []string{
		"",
		"AXL1",
		"AXL2 ip=10.73.0.2/24",
		"axl1 ip=10.73.0.2/24",
		"AXL1  ip=10.73.0.2/24",
		"AXL1 ip=10.73.0.2/24 ",
		"AXL1 ip=10.73.0.2",
		"AXL1 ip=10.73.0.02/24",
		"AXL1 ip=10.73.0.2/33",
		"AXL1 ip=0.0.0.0/0",
		"AXL1 ip=fd00::1/64",
		"AXL1 ip=10.73.0.2/24 ip=10.73.0.3/24",
		"AXL1 ip=10.73.0.2/24 every=59",
		"AXL1 ip=10.73.0.2/24 every=86401",
		"AXL1 ip=10.73.0.2/24 every=+600",
		"AXL1 ip=10.73.0.2/24 every=0600",
		"AXL1 ip=10.73.0.2/24 tags=",
		"AXL1 ip=10.73.0.2/24 tags=INET,",
		"AXL1 ip=10.73.0.2/24 tags=inet",
		"AXL1 ip=10.73.0.2/24 tags=INET,INET",
		"AXL1 ip=10.73.0.2/24 tags=TOOLONGTAG",
		"AXL1 ip=10.73.0.2/24 tags=A,B,C,D,E,F,G,H,I",
		"AXL1 ip=10.73.0.2/24 ask=0",
		"AXL1 ip=10.73.0.2/24 Ask=1",
		"AXL1 ip=10.73.0.2/24 x",
		"AXL1 ip=10.73.0.2/24 note=\x01",
		"AXL1 ip=10.73.0.2/24 note=" + strings.Repeat("x", 120),
	} {
		if b, err := Parse([]byte(info)); err == nil {
			t.Errorf("accepted %q as %+v", info, b)
		}
	}
}

func TestMarshalRejectsWhatParseWouldReject(t *testing.T) {
	addr := netip.MustParsePrefix("10.73.0.2/24")
	for _, b := range []Beacon{
		{Every: DefaultEvery},
		{Addr: addr, Every: 30 * time.Second},
		{Addr: addr, Every: 90*time.Second + 500*time.Millisecond}, // not whole seconds
		{Addr: addr, Every: DefaultEvery, Tags: []string{"inet"}},
	} {
		if _, err := b.Marshal(); err == nil {
			t.Errorf("Marshal accepted %+v", b)
		}
	}
}

func TestParseTagsFromTheCommandLine(t *testing.T) {
	if tags, err := ParseTags("inet,auto"); err != nil || !slices.Equal(tags, []string{"INET", "AUTO"}) {
		t.Fatalf("%v %v", tags, err)
	}
	if tags, err := ParseTags(""); err != nil || tags != nil {
		t.Fatalf("%v %v", tags, err)
	}
	if _, err := ParseTags("INET,,AUTO"); err == nil {
		t.Fatal("accepted an empty tag")
	}
}

func TestFromFrame(t *testing.T) {
	b := Beacon{Addr: netip.MustParsePrefix("10.73.0.2/24"), Every: DefaultEvery, Tags: []string{"INET"}}
	raw, err := b.Frame(ax25.MustAddress("TESTB-1"))
	if err != nil {
		t.Fatal(err)
	}
	f, _ := ax25.Decode(raw)
	call, got, ok, err := FromFrame(f)
	if !ok || err != nil || call.String() != "TESTB-1" || got.Addr != b.Addr || !slices.Equal(got.Tags, b.Tags) {
		t.Fatalf("%v %+v %v %v", call, got, ok, err)
	}

	ip, _ := ax25.NewUI(ax25.MustAddress("TESTA-1"), ax25.MustAddress("TESTB-1"), nil, ax25.PIDIPv4, []byte{0x45}).Encode()
	other, _ := ax25.NewUI(Dest, ax25.MustAddress("TESTB-1"), nil, ax25.PIDIPv4, []byte("AXL1 ip=10.73.0.2/24")).Encode()
	for _, raw := range [][]byte{ip, other} {
		f, _ := ax25.Decode(raw)
		if _, _, ok, _ := FromFrame(f); ok {
			t.Fatalf("% X taken for a beacon", raw)
		}
	}

	relayed, _ := ax25.NewUI(Dest, ax25.MustAddress("TESTB-1"), []ax25.Address{ax25.MustAddress("WIDE1-1")}, ax25.PIDNoL3, []byte("AXL1 ip=10.73.0.2/24")).Encode()
	f, _ = ax25.Decode(relayed)
	if _, _, ok, err := FromFrame(f); !ok || !errors.Is(err, errRelayed) {
		t.Fatalf("relayed: %v %v", ok, err)
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"AXL1 ip=10.73.0.2/24", "AXL1 ip=10.73.0.2/24 every=300 tags=INET,AUTO ask=1", "AXL1 x=y ip=1.2.3.4/8"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, info []byte) {
		b, err := Parse(info)
		if err != nil {
			return
		}
		// What we accept, we can write again, and it reads back the same.
		out, err := b.Marshal()
		if err != nil {
			t.Fatalf("accepted %q but cannot write %+v: %v", info, b, err)
		}
		back, err := Parse(out)
		if err != nil || back.Addr != b.Addr || back.Every != b.Every || back.Ask != b.Ask || !slices.Equal(back.Tags, b.Tags) {
			t.Fatalf("%q -> %q -> %+v, %v", info, out, back, err)
		}
	})
}

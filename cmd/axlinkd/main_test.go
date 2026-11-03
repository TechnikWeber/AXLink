// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/discovery"
	"github.com/TechnikWeber/AXLink/internal/forward"
	"github.com/TechnikWeber/AXLink/internal/gateway"
	"github.com/TechnikWeber/AXLink/internal/ipbridge"
	"github.com/TechnikWeber/AXLink/internal/kiss"
	"github.com/TechnikWeber/AXLink/internal/mesh"
	"github.com/TechnikWeber/AXLink/internal/settings"
	"github.com/TechnikWeber/AXLink/internal/tnc"
	"github.com/TechnikWeber/AXLink/internal/ui"
)

func TestVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"version"}, nil, &out, &out); code != 0 || !strings.HasPrefix(out.String(), "axlinkd ") {
		t.Fatalf("code %d, output %q", code, out.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"nope"}, nil, &out, &out); code != 2 {
		t.Fatalf("code %d", code)
	}
}

func TestDecodeHex(t *testing.T) {
	path := []ax25.Address{ax25.MustAddress("WIDE1-1")}
	path[0].Bit7 = true
	frame, _ := ax25.NewUI(ax25.MustAddress("QST"), ax25.MustAddress("N0CALL-7"), path, ax25.PIDNoL3, []byte("hi")).Encode()
	var stream []byte
	stream, _ = kiss.Append(stream, kiss.Param(0, kiss.CmdTxDelay, 30))
	stream, _ = kiss.Append(stream, kiss.Data(0, frame))
	in := strings.NewReader(hex.EncodeToString(stream))

	var out, errs bytes.Buffer
	if code := run([]string{"decode", "-hex"}, in, &out, &errs); code != 0 {
		t.Fatalf("code %d: %s", code, errs.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output %q", out.String())
	}
	var param, data frameJSON
	if err := json.Unmarshal([]byte(lines[0]), &param); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &data); err != nil {
		t.Fatal(err)
	}
	if param.Command != 1 || param.Payload != "1e" {
		t.Fatalf("parameter frame %+v", param)
	}
	if data.Dest != "QST" || data.Src != "N0CALL-7" || len(data.Path) != 1 || data.Path[0] != "WIDE1-1*" ||
		data.PID == nil || *data.PID != ax25.PIDNoL3 || data.Info != hex.EncodeToString([]byte("hi")) {
		t.Fatalf("data frame %+v", data)
	}
}

func TestRunFlags(t *testing.T) {
	var errs bytes.Buffer
	cfg, err := parseNodeFlags([]string{"-call", "testa-1", "-kiss-unix", "/tmp/a.sock", "-ip", "10.73.0.1/24",
		"-route", "10.73.0.2=TESTB-1", "-route", "10.74.0.0/16=TESTGW"}, &errs)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.call.String() != "TESTA-1" || cfg.profile.Name != "generic-kiss" || len(cfg.routes) != 2 {
		t.Fatalf("config %+v", cfg)
	}
	if cfg.routes[0].Prefix.String() != "10.73.0.2/32" {
		t.Fatalf("plain address route %v", cfg.routes[0].Prefix)
	}
	// Only the route outside the own /24 needs an operating-system route.
	if len(cfg.tun.Routes) != 1 || cfg.tun.Routes[0].String() != "10.74.0.0/16" {
		t.Fatalf("adapter routes %v", cfg.tun.Routes)
	}

	if cfg.linkMTU != 256 || cfg.tun.MTU != 256 {
		t.Fatalf("MTUs radio %d adapter %d", cfg.linkMTU, cfg.tun.MTU)
	}
	if cfg.beacon != 10*time.Minute || cfg.tags != nil {
		t.Fatalf("beacon %v, tags %v", cfg.beacon, cfg.tags)
	}
	gw, err := parseNodeFlags([]string{"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-tags", "inet,auto", "-beacon", "0"}, &errs)
	if err != nil || strings.Join(gw.tags, ",") != "INET,AUTO" || gw.beacon != 0 {
		t.Fatalf("gateway flags: %+v %v", gw, err)
	}
	if cfg.ui != "127.0.0.1:7373" || cfg.localProxy != "127.0.0.1:7374" || len(cfg.overrides) != 0 {
		t.Fatalf("web defaults: %q %q, %d overrides", cfg.ui, cfg.localProxy, len(cfg.overrides))
	}
	// Options that change stored settings become overrides of those settings.
	opts, err := parseNodeFlags([]string{"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-proxy", "3128", "-proxy-lan",
		"-gateway", "testb-1", "-open", "8080/tcp", "-open", "22/tcp=TESTB", "-firewall", "open", "-config", "", "-ui", "",
		"-forward", "2222=192.168.1.10:22=TESTB"}, &errs)
	if err != nil {
		t.Fatal(err)
	}
	s := settings.Defaults()
	for _, o := range opts.overrides {
		o(&s)
	}
	if !s.Share.Enabled || s.Share.Port != 3128 || !s.Share.LAN || s.Gateway.Mode != "station" || s.Gateway.Call != "TESTB-1" ||
		len(s.Firewall.Rules) != 2 || s.Firewall.Rules[1].Calls[0] != "TESTB" || s.Firewall.Mode != "open" ||
		len(s.Forwards) != 1 || s.Forwards[0].Target != "192.168.1.10:22" || s.Forwards[0].Calls[0] != "TESTB" ||
		opts.configPath != "" || opts.ui != "" {
		t.Fatalf("overrides gave %+v", s)
	}
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	stop, _ := parseNodeFlags([]string{"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-proxy", "0"}, &errs)
	s = settings.Defaults()
	s.Share.Enabled = true
	stop.overrides[0](&s)
	if s.Share.Enabled {
		t.Fatal("-proxy 0 must stop sharing")
	}
	big, err := parseNodeFlags([]string{"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-adapter-mtu", "576"}, &errs)
	if err != nil || big.linkMTU != 256 || big.tun.MTU != 576 {
		t.Fatalf("adapter MTU: %+v %v", big, err)
	}

	serial, err := parseNodeFlags([]string{"-call", "TESTA-1", "-ip", "10.73.0.1/24"}, &errs)
	if err != nil || serial.profile.Name != "f4hwn-kisstnc" || serial.dialer.String() != "serial:auto" {
		t.Fatalf("serial default: %+v %v", serial, err)
	}

	for name, args := range map[string][]string{
		"placeholder call": {"-call", "N0CALL-1", "-ip", "10.73.0.1/24"},
		"no call":          {"-ip", "10.73.0.1/24"},
		"no address":       {"-call", "TESTA-1"},
		"IPv6":             {"-call", "TESTA-1", "-ip", "fd00::1/64"},
		"two transports":   {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-port", "auto", "-kiss-tcp", "x:1"},
		"unknown profile":  {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-profile", "nope"},
		"bad route":        {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-route", "10.73.0.2"},
		"tiny MTU":         {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-mtu", "40"},
		"bad tag":          {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-tags", "IN-ET"},
		"short beacon":     {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-beacon", "10s"},
		"no port":          {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-proxy", "70000"},
		"bad gateway":      {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-gateway", "not a call"},
		"bad open":         {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-open", "22"},
		"bad forward":      {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-forward", "2222=192.168.1.10"},
		"bad firewall":     {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-firewall", "sometimes"},
		"bad ui address":   {"-call", "TESTA-1", "-ip", "10.73.0.1/24", "-ui", "7373"},
	} {
		if _, err := parseNodeFlags(args, &errs); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestEveryCounterHasATextInBothLanguages(t *testing.T) {
	all := append(counters(tnc.Stats{}, ipbridge.Stats{}, discovery.Stats{}, &gateway.LocalStats{}, &gateway.Stats{}),
		append(forwardCounters(forward.Stats{}), relayCounters(mesh.Stats{})...)...)
	for _, c := range all {
		if !ui.HasText("c."+c.Key) || !ui.HasText("group."+c.Group) {
			t.Errorf("no text for counter %s/%s", c.Group, c.Key)
		}
	}
}

func TestDecodeShowsBeacons(t *testing.T) {
	frame, _ := ax25.NewUI(ax25.MustAddress("AXLINK"), ax25.MustAddress("TESTB-1"), nil, ax25.PIDNoL3,
		[]byte("AXL1 ip=10.73.0.2/24 tags=INET")).Encode()
	stream, _ := kiss.Append(nil, kiss.Data(0, frame))
	var out, errs bytes.Buffer
	if code := run([]string{"decode"}, bytes.NewReader(stream), &out, &errs); code != 0 {
		t.Fatalf("code %d: %s", code, errs.String())
	}
	var f frameJSON
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &f); err != nil || f.Beacon != "AXL1 ip=10.73.0.2/24 tags=INET" || f.Error != "" {
		t.Fatalf("%q %v", out.String(), err)
	}
}

func TestDecodeRawStreamReportsBadFrames(t *testing.T) {
	in := bytes.NewReader([]byte{kiss.FEND, 0x00, 0x01, 0x02, kiss.FEND, 0x00, kiss.FESC, 0x41, kiss.FEND})
	var out, errs bytes.Buffer
	if code := run([]string{"decode"}, in, &out, &errs); code != 0 {
		t.Fatalf("code %d", code)
	}
	var f frameJSON
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &f); err != nil || f.Error == "" {
		t.Fatalf("expected an AX.25 error for the short frame, got %q (%v)", out.String(), err)
	}
	if !strings.Contains(errs.String(), "1 invalid KISS frames dropped") {
		t.Fatalf("stderr %q", errs.String())
	}
}

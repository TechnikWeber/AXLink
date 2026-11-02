// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/kiss"
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
	} {
		if _, err := parseNodeFlags(args, &errs); err == nil {
			t.Errorf("%s: accepted", name)
		}
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

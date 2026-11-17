// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/TechnikWeber/AXLink/internal/settings"
)

// TestMain points the user's configuration folder at a temporary one, so
// that no test reads or writes the developer's own settings.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "axlinkd-test-")
	if err != nil {
		panic(err)
	}
	for _, v := range []string{"XDG_CONFIG_HOME", "HOME", "APPDATA"} {
		os.Setenv(v, dir)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestRunUsesTheStoredStation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, _ := settings.Open(path)
	if _, err := store.Update(func(s *settings.Settings) {
		s.Station = settings.Station{Call: "testa-1", Port: "/dev/ttyACM7", IP: "10.73.0.9/24", Profile: "f4hwn-kisstnc-ackmode"}
	}); err != nil {
		t.Fatal(err)
	}
	var errs bytes.Buffer
	cfg, err := parseNodeFlags([]string{"-config", path}, &errs)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.call.String() != "TESTA-1" || cfg.tun.Address.String() != "10.73.0.9/24" ||
		cfg.dialer.String() != "serial:/dev/ttyACM7" || cfg.profile.Name != "f4hwn-kisstnc-ackmode" {
		t.Fatalf("stored station not used: %+v", cfg)
	}
	// Flags win; a TNC given by flag does not take the stored profile.
	cfg, err = parseNodeFlags([]string{"-config", path, "-call", "TESTB-2", "-ip", "10.73.0.20/24", "-kiss-tcp", "x:8001"}, &errs)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.call.String() != "TESTB-2" || cfg.tun.Address.String() != "10.73.0.20/24" ||
		cfg.dialer.String() != "tcp:x:8001" || cfg.profile.Name != "generic-kiss" {
		t.Fatalf("flags must override the stored station: %+v", cfg)
	}
	// Nothing stored, nothing given: the error points to the setup.
	_, err = parseNodeFlags([]string{"-config", filepath.Join(t.TempDir(), "none.json")}, &errs)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("axlinkd setup")) {
		t.Fatalf("want a hint to axlinkd setup, got %v", err)
	}
}

func TestShellQuote(t *testing.T) {
	got := shellQuote("/opt/ax link/axlinkd", "run", "-config", "/home/o'brien/settings.json")
	want := `'/opt/ax link/axlinkd' run -config '/home/o'\''brien/settings.json'`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

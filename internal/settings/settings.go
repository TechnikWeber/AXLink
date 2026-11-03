// SPDX-License-Identifier: GPL-3.0-or-later

// Package settings keeps what the web interface changes across restarts: the
// choice of an internet gateway, sharing the own internet connection, the
// station's welcome page, the firewall and the port forwarding. They live in a JSON file,
// settings.json in the user's configuration folder by default, written
// atomically and readable by the owner only.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/firewall"
	"github.com/TechnikWeber/AXLink/internal/forward"
	"github.com/TechnikWeber/AXLink/internal/locator"
)

// Settings are the stored settings.
type Settings struct {
	Gateway  Gateway         `json:"gateway"`
	Share    Share           `json:"share"`
	Welcome  Welcome         `json:"welcome"`
	Firewall firewall.Config `json:"firewall"`
	// Forwards publish services of the own network on ports of the radio
	// address.
	Forwards []forward.Rule `json:"forwards,omitempty"`
	// Relay repeats frames for other stations (mesh, role RELAY).
	Relay bool `json:"relay,omitempty"`
}

// Gateway is the choice of the internet gateway this station uses.
type Gateway struct {
	Mode string `json:"mode"`           // off, auto, station
	Call string `json:"call,omitempty"` // for station
}

// Share is this station's own internet connection, shared with the radio
// network as an HTTP proxy.
type Share struct {
	Enabled bool   `json:"enabled"`
	Port    uint16 `json:"port"`
	LAN     bool   `json:"lan"` // radio users may reach the own network too
	// Only lists the stations that may use the gateway; empty: every station.
	// A callsign without SSID stands for all its SSIDs.
	Only []string `json:"only,omitempty"`
}

// Welcome is the station's own page on its radio address: who runs it and
// what it offers. It is kept short: every byte goes over the air.
type Welcome struct {
	Enabled bool   `json:"enabled"`
	Port    uint16 `json:"port"`
	Name    string `json:"name,omitempty"`
	Locator string `json:"locator,omitempty"` // Maidenhead, e.g. JO62qm
	// LocatorInBeacon announces the locator in the station beacon, so that
	// other stations see the distance; off unless the operator wants it.
	LocatorInBeacon bool   `json:"locator_in_beacon,omitempty"`
	QTH             string `json:"qth,omitempty"`
	Equipment       string `json:"equipment,omitempty"`
	Text            string `json:"text,omitempty"`
}

// Defaults are the settings before anything was changed.
func Defaults() Settings {
	return Settings{
		Gateway:  Gateway{Mode: "off"},
		Share:    Share{Port: 8080},
		Welcome:  Welcome{Enabled: true, Port: 80},
		Firewall: firewall.Config{Mode: firewall.ModeProtect, Ping: true},
	}
}

// Check reports the first thing that is wrong.
func (s Settings) Check() error {
	switch s.Gateway.Mode {
	case "off", "auto":
	case "station":
		if _, err := ax25.ParseAddress(s.Gateway.Call); err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
	default:
		return fmt.Errorf("gateway: mode %q (off, auto or station)", s.Gateway.Mode)
	}
	if s.Share.Port == 0 {
		return errors.New("share: port 0")
	}
	if len(s.Share.Only) > 64 {
		return errors.New("share: more than 64 stations")
	}
	for _, c := range s.Share.Only {
		if _, err := ax25.ParseAddress(c); err != nil {
			return fmt.Errorf("share: %q: %w", c, err)
		}
	}
	if s.Welcome.Port == 0 {
		return errors.New("welcome: port 0")
	}
	if s.Welcome.Locator != "" {
		if _, err := locator.Normalize(s.Welcome.Locator); err != nil {
			return fmt.Errorf("welcome: %w", err)
		}
	} else if s.Welcome.LocatorInBeacon {
		return errors.New("welcome: no locator to announce")
	}
	for _, f := range []struct {
		name, value string
		max         int
		lines       bool
	}{
		{"name", s.Welcome.Name, 60, false}, {"locator", s.Welcome.Locator, 10, false},
		{"QTH", s.Welcome.QTH, 60, false}, {"equipment", s.Welcome.Equipment, 200, false},
		{"text", s.Welcome.Text, 1000, true},
	} {
		if err := checkText(f.value, f.max, f.lines); err != nil {
			return fmt.Errorf("welcome %s: %w", f.name, err)
		}
	}
	if err := s.Firewall.Check(); err != nil {
		return err
	}
	if len(s.Forwards) > 32 {
		return errors.New("forward: more than 32 rules")
	}
	used := map[uint16]string{}
	if s.Welcome.Enabled {
		used[s.Welcome.Port] = "the welcome page"
	}
	if s.Share.Enabled {
		used[s.Share.Port] = "the gateway"
	}
	for _, f := range s.Forwards {
		if err := f.Check(); err != nil {
			return err
		}
		if what, ok := used[f.Port]; ok {
			return fmt.Errorf("forward: port %d is already used by %s", f.Port, what)
		}
		used[f.Port] = "another forwarding"
	}
	return nil
}

func checkText(s string, max int, lines bool) error {
	if !utf8.ValidString(s) {
		return errors.New("not UTF-8")
	}
	if n := utf8.RuneCountInString(s); n > max {
		return fmt.Errorf("%d characters, at most %d", n, max)
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(lines && (r == '\n' || r == '\r')) {
			return errors.New("contains control characters")
		}
	}
	return nil
}

// DefaultPath is settings.json in the user's configuration folder, e.g.
// ~/.config/axlink on Linux and %AppData%\AXLink on Windows.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	name := "axlink"
	if filepath.Separator == '\\' {
		name = "AXLink"
	}
	return filepath.Join(dir, name, "settings.json")
}

// Store holds the settings and writes every change to its file.
type Store struct {
	path string // "" keeps them in memory only
	mu   sync.Mutex
	cur  Settings
}

// Open reads the file at path; a missing file gives the defaults. An empty
// path keeps the settings in memory.
func Open(path string) (*Store, error) {
	st := &Store{path: path, cur: Defaults()}
	if path == "" {
		return st, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	s := Defaults()
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("settings: %s: %w", path, err)
	}
	if err := s.Check(); err != nil {
		return nil, fmt.Errorf("settings: %s: %w", path, err)
	}
	st.cur = s
	return st, nil
}

// Path is the file the settings are written to; "" for none.
func (st *Store) Path() string { return st.path }

// Get returns a copy of the settings.
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return clone(st.cur)
}

// Update applies change to a copy, checks it, writes it and only then makes
// it the current settings. It returns the new settings.
func (st *Store) Update(change func(*Settings)) (Settings, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	next := clone(st.cur)
	change(&next)
	normalise(&next)
	if err := next.Check(); err != nil {
		return clone(st.cur), err
	}
	if err := st.write(next); err != nil {
		return clone(st.cur), err
	}
	st.cur = next
	return clone(next), nil
}

func (st *Store) write(s Settings) error {
	if st.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o700); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(st.path), ".settings-*.json")
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	defer os.Remove(tmp.Name()) // gone after the rename; cleans up after an error
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := os.Rename(tmp.Name(), st.path); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}

// normalise writes callsigns in capitals and trims texts.
func normalise(s *Settings) {
	up := func(calls []string) []string {
		var out []string
		for _, c := range calls {
			if c = strings.ToUpper(strings.TrimSpace(c)); c != "" {
				out = append(out, c)
			}
		}
		return out
	}
	s.Gateway.Call = strings.ToUpper(strings.TrimSpace(s.Gateway.Call))
	s.Share.Only = up(s.Share.Only)
	for i := range s.Firewall.Rules {
		s.Firewall.Rules[i].Proto = strings.ToLower(strings.TrimSpace(s.Firewall.Rules[i].Proto))
		s.Firewall.Rules[i].Calls = up(s.Firewall.Rules[i].Calls)
		s.Firewall.Rules[i].Note = strings.TrimSpace(s.Firewall.Rules[i].Note)
	}
	for i := range s.Forwards {
		f := &s.Forwards[i]
		f.Target, f.Note, f.Calls = strings.TrimSpace(f.Target), strings.TrimSpace(f.Note), up(f.Calls)
	}
	w := &s.Welcome
	w.Name, w.Locator, w.QTH = strings.TrimSpace(w.Name), strings.TrimSpace(w.Locator), strings.TrimSpace(w.QTH)
	w.Equipment, w.Text = strings.TrimSpace(w.Equipment), strings.TrimSpace(w.Text)
	if loc, err := locator.Normalize(w.Locator); err == nil {
		w.Locator = loc
	}
}

func clone(s Settings) Settings {
	data, _ := json.Marshal(s)
	var out Settings
	_ = json.Unmarshal(data, &out)
	return out
}

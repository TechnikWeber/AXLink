// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TechnikWeber/AXLink/internal/firewall"
	"github.com/TechnikWeber/AXLink/internal/forward"
)

func TestDefaultsAreValidAndSafe(t *testing.T) {
	d := Defaults()
	if err := d.Check(); err != nil {
		t.Fatal(err)
	}
	if d.Share.Enabled || d.Share.LAN || d.Firewall.Mode != firewall.ModeProtect || !d.Welcome.Enabled || d.Gateway.Mode != "off" {
		t.Fatalf("defaults %+v", d)
	}
}

func TestChangesSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "settings.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Update(func(s *Settings) {
		s.Gateway = Gateway{Mode: "station", Call: "testb-1"}
		s.Share = Share{Enabled: true, Port: 3128, Only: []string{" testa ", ""}}
		s.Welcome.Text = "Hello from the lab.\nSecond line."
		s.Firewall.Rules = []firewall.Rule{{Proto: "TCP", Port: 22, Calls: []string{"testb"}}}
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := again.Get()
	if s.Gateway.Call != "TESTB-1" || s.Share.Port != 3128 || len(s.Share.Only) != 1 || s.Share.Only[0] != "TESTA" ||
		!strings.Contains(s.Welcome.Text, "\n") || s.Firewall.Rules[0].Proto != "tcp" || s.Firewall.Rules[0].Calls[0] != "TESTB" {
		t.Fatalf("read back %+v", s)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("file mode %v %v", info.Mode(), err)
		}
	}
}

func TestAnInvalidChangeIsNotWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, _ := Open(path)
	for name, change := range map[string]func(*Settings){
		"bad mode":        func(s *Settings) { s.Gateway.Mode = "sometimes" },
		"no call":         func(s *Settings) { s.Gateway = Gateway{Mode: "station"} },
		"long text":       func(s *Settings) { s.Welcome.Text = strings.Repeat("x", 1001) },
		"control chars":   func(s *Settings) { s.Welcome.Name = "a\x1bb" },
		"bad rule":        func(s *Settings) { s.Firewall.Rules = []firewall.Rule{{Proto: "icmp", Port: 1}} },
		"bad share call":  func(s *Settings) { s.Share.Only = []string{"not a call"} },
		"firewall mode":   func(s *Settings) { s.Firewall.Mode = "" },
		"zero share port": func(s *Settings) { s.Share.Port = 0 },
	} {
		if _, err := st.Update(change); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("a refused change was written")
	}
	if st.Get().Gateway.Mode != "off" {
		t.Fatal("a refused change became current")
	}
}

func TestTheLocatorIsCheckedAndWrittenAsUsual(t *testing.T) {
	st, _ := Open("")
	s, err := st.Update(func(s *Settings) { s.Welcome.Locator = " jo62QM " })
	if err != nil || s.Welcome.Locator != "JO62qm" {
		t.Fatalf("%q %v", s.Welcome.Locator, err)
	}
	if _, err := st.Update(func(s *Settings) { s.Welcome.Locator = "near Berlin" }); err == nil {
		t.Fatal("not a locator, but accepted")
	}
	if _, err := st.Update(func(s *Settings) { s.Welcome.Locator, s.Welcome.LocatorInBeacon = "", true }); err == nil {
		t.Fatal("announcing no locator was accepted")
	}
	if s, err := st.Update(func(s *Settings) { s.Welcome.LocatorInBeacon = true }); err != nil || !s.Welcome.LocatorInBeacon {
		t.Fatal(err)
	}
}

func TestForwardsAreCheckedAgainstTheOtherPorts(t *testing.T) {
	st, _ := Open("")
	s, err := st.Update(func(s *Settings) {
		s.Forwards = []forward.Rule{{Port: 2222, Target: " 192.168.1.10:22 ", Calls: []string{"testb"}, Note: " NAS "}}
	})
	if err != nil || s.Forwards[0].Target != "192.168.1.10:22" || s.Forwards[0].Calls[0] != "TESTB" || s.Forwards[0].Note != "NAS" {
		t.Fatalf("%+v %v", s.Forwards, err)
	}
	for name, f := range map[string][]forward.Rule{
		"the welcome page's port": {{Port: 80, Target: "192.168.1.10:80"}},
		"twice the same port":     {{Port: 2222, Target: "a:22"}, {Port: 2222, Target: "b:22"}},
		"no target port":          {{Port: 2222, Target: "192.168.1.10"}},
	} {
		if _, err := st.Update(func(s *Settings) { s.Forwards = f }); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// The gateway's port only while it shares.
	if _, err := st.Update(func(s *Settings) { s.Forwards = []forward.Rule{{Port: 8080, Target: "a:80"}} }); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *Settings) { s.Share.Enabled = true }); err == nil {
		t.Fatal("sharing on a forwarded port was accepted")
	}
}

func TestABrokenFileIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte("{not json"), 0o600)
	if _, err := Open(path); err == nil {
		t.Fatal("a broken file was accepted")
	}
}

func TestMissingFieldsKeepTheirDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"gateway":{"mode":"auto"}}`), 0o600)
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := st.Get(); s.Gateway.Mode != "auto" || !s.Welcome.Enabled || s.Welcome.Port != 80 || s.Firewall.Mode != firewall.ModeProtect {
		t.Fatalf("%+v", s)
	}
}

func TestInMemoryOnly(t *testing.T) {
	st, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *Settings) { s.Gateway.Mode = "auto" }); err != nil || st.Get().Gateway.Mode != "auto" {
		t.Fatal(err)
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package setup

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TechnikWeber/AXLink/internal/settings"
	"github.com/TechnikWeber/AXLink/internal/tnc"
)

func newServer(t *testing.T) (*Server, *settings.Store, *[]settings.Station) {
	t.Helper()
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var started []settings.Station
	s := New(Config{Store: store, UIURL: "http://127.0.0.1:7373/",
		Ports: func() ([]tnc.PortInfo, error) {
			return []tnc.PortInfo{{Name: "/dev/ttyACM0", USB: true, VID: tnc.F4HWNVendorID, PID: tnc.F4HWNProductID, Product: "UV-K1"}}, nil
		},
		Start: func(st settings.Station) (Result, error) {
			started = append(started, st)
			return Result{Wait: true}, nil
		},
	})
	return s, store, &started
}

func post(s *Server, body, token, host, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(body))
	r.Host = host
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("X-Setup-Token", token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestSaveStoresTheStationAndStarts(t *testing.T) {
	s, store, started := newServer(t)
	w := post(s, `{"call":" testa-1 ","port":"auto","ip":"10.73.0.5/24","profile":"f4hwn-kisstnc-ackmode"}`,
		s.token, "127.0.0.1:7375", "http://127.0.0.1:7375")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"wait":true`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := store.Get().Station
	want := settings.Station{Call: "TESTA-1", Port: "auto", IP: "10.73.0.5/24", Profile: "f4hwn-kisstnc-ackmode"}
	if got != want || len(*started) != 1 || (*started)[0] != want {
		t.Fatalf("stored %+v, started %+v", got, *started)
	}
}

func TestSaveRefusesBadInput(t *testing.T) {
	s, store, started := newServer(t)
	for name, body := range map[string]string{
		"placeholder":     `{"call":"N0CALL-1","port":"auto","ip":"10.73.0.5/24"}`,
		"no call":         `{"call":"","port":"auto","ip":"10.73.0.5/24"}`,
		"no address":      `{"call":"TESTA-1","port":"auto","ip":""}`,
		"IPv6":            `{"call":"TESTA-1","port":"auto","ip":"fd00::1/64"}`,
		"network address": `{"call":"TESTA-1","port":"auto","ip":"10.73.0.0/24"}`,
		"bad profile":     `{"call":"TESTA-1","port":"auto","ip":"10.73.0.5/24","profile":"nope"}`,
		"control chars":   `{"call":"TESTA-1","port":"a\u0007","ip":"10.73.0.5/24"}`,
	} {
		if w := post(s, body, s.token, "127.0.0.1:7375", ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if store.Get().Station != (settings.Station{}) || len(*started) != 0 {
		t.Fatalf("bad input changed something: %+v %+v", store.Get().Station, *started)
	}
}

func TestSaveNeedsTokenOwnOriginAndLoopback(t *testing.T) {
	s, _, started := newServer(t)
	body := `{"call":"TESTA-1","port":"auto","ip":"10.73.0.5/24"}`
	for name, w := range map[string]*httptest.ResponseRecorder{
		"no token":       post(s, body, "", "127.0.0.1:7375", ""),
		"wrong token":    post(s, body, "00", "127.0.0.1:7375", ""),
		"foreign origin": post(s, body, s.token, "127.0.0.1:7375", "http://evil.example"),
		"rebinding":      post(s, body, s.token, "evil.example:7375", ""),
	} {
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if len(*started) != 0 {
		t.Fatal("started without a valid request")
	}
}

func TestPageShowsRadioAndLanguage(t *testing.T) {
	s, _, _ := newServer(t)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "localhost:7375"
	r.Header.Set("Accept-Language", "de-DE,de;q=0.9,en;q=0.8")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	body := w.Body.String()
	for _, want := range []string{"AXLink einrichten", "/dev/ttyACM0 – UV-K1 – Funkgerät mit F4HWN-Firmware",
		`data-token="` + s.token + `"`, `value="auto" selected`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(body, "Gerade ist kein Funkgerät") {
		t.Error("says no radio although one is connected")
	}
}

func TestSuggestIP(t *testing.T) {
	a, b := SuggestIP("TESTA-1"), SuggestIP("testa-7")
	if a != b {
		t.Fatalf("the SSID must not change the suggestion: %s %s", a, b)
	}
	for _, c := range []string{"TESTA", "TESTB", "TESTC", "N0CALL", "A"} {
		ip := SuggestIP(c)
		if !strings.HasPrefix(ip, "10.73.0.") || strings.HasSuffix(ip, ".0/24") || strings.HasSuffix(ip, ".1/24") ||
			strings.HasSuffix(ip, ".255/24") {
			t.Errorf("%s: %s", c, ip)
		}
	}
}

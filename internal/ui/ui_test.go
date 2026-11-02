// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func testServer() (*Server, *[]string) {
	var chosen []string
	s := New(Config{
		Status: func() Status {
			return Status{Version: "0.2.0-test", Call: "TESTA-1", Addr: "10.73.0.1/24",
				Stations: []Station{{Call: "TESTB-1", Addr: "10.73.0.2/24", Tags: []string{"INET"}, Proxy: "10.73.0.2:8080"}},
				Internet: Internet{Mode: "off", LocalProxy: "127.0.0.1:7374"},
				Counters: []Counter{{Group: "radio", Key: "frames_out", Value: 7}}}
		},
		SetGateway: func(mode, call string) error {
			if mode == "nonsense" {
				return errors.New("unknown mode")
			}
			chosen = append(chosen, mode+" "+call)
			return nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	s.loopback = true
	return s, &chosen
}

func do(s *Server, method, target, body string, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Host = "127.0.0.1:7373"
	for k, v := range header {
		if k == "Host" {
			r.Host = v
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestBothLanguagesHaveTheSameTexts(t *testing.T) {
	en, de := slices.Sorted(maps.Keys(texts["en"])), slices.Sorted(maps.Keys(texts["de"]))
	if !slices.Equal(en, de) {
		t.Fatalf("keys differ:\nen %v\nde %v", en, de)
	}
	for k, v := range texts["de"] {
		if strings.TrimSpace(v) == "" {
			t.Errorf("de %s is empty", k)
		}
	}
}

func TestPageAndScriptUseKnownTexts(t *testing.T) {
	page, _ := assets.ReadFile("assets/index.html")
	script, _ := assets.ReadFile("assets/app.js")
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`index \.T "([^"]+)"`).FindAllStringSubmatch(string(page), -1) {
		keys[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`\bt\("([a-z_.]+)"`).FindAllStringSubmatch(string(script), -1) {
		if !strings.HasSuffix(m[1], ".") { // "c." + key: the counters are checked where they are made
			keys[m[1]] = true
		}
	}
	for _, g := range regexp.MustCompile(`"(radio|ip|beacons|internet|gateway)"`).FindAllStringSubmatch(string(script), -1) {
		keys["group."+g[1]] = true
	}
	if len(keys) < 30 {
		t.Fatalf("found only %d keys; the patterns no longer match", len(keys))
	}
	for k := range keys {
		if !HasText(k) {
			t.Errorf("no text for %q", k)
		}
	}
}

func TestLanguageFollowsTheChoiceThenTheBrowser(t *testing.T) {
	for _, tc := range []struct {
		query, cookie, accept, want string
		chosen                      bool
	}{
		{"", "", "", "en", false},
		{"", "", "de-DE,de;q=0.9,en;q=0.8", "de", false},
		{"", "", "fr-FR, en;q=0.5", "en", false},
		{"", "", "fr-FR", "en", false},
		{"", "de", "en-US", "de", false},
		{"lang=de", "en", "en-US", "de", true},
		{"lang=xx", "", "de", "de", false},
	} {
		r := httptest.NewRequest("GET", "/?"+tc.query, nil)
		if tc.cookie != "" {
			r.AddCookie(&http.Cookie{Name: langCookie, Value: tc.cookie})
		}
		if tc.accept != "" {
			r.Header.Set("Accept-Language", tc.accept)
		}
		if got, chosen := language(r); got != tc.want || chosen != tc.chosen {
			t.Errorf("%+v: got %s %v", tc, got, chosen)
		}
	}
}

func TestPageInGerman(t *testing.T) {
	s, _ := testServer()
	w := do(s, "GET", "/", "", map[string]string{"Accept-Language": "de"})
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `<html lang="de">`) || !strings.Contains(body, "Gehörte Stationen") {
		t.Fatalf("%d %s", w.Code, body)
	}
	m := regexp.MustCompile(`(?s)<script id="texts" type="application/json">(.*?)</script>`).FindStringSubmatch(body)
	var got map[string]string
	if m == nil || json.Unmarshal([]byte(m[1]), &got) != nil || got["stations.title"] != "Gehörte Stationen" {
		t.Fatalf("texts for the script: %v", m)
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("no content security policy")
	}
}

func TestStatusAndStatic(t *testing.T) {
	s, _ := testServer()
	w := do(s, "GET", "/api/status", "", nil)
	var st Status
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &st) != nil || st.Stations[0].Proxy != "10.73.0.2:8080" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, f := range []string{"/static/app.css", "/static/app.js"} {
		if w := do(s, "GET", f, "", nil); w.Code != 200 || w.Body.Len() < 500 {
			t.Errorf("%s: %d, %d bytes", f, w.Code, w.Body.Len())
		}
	}
	if w := do(s, "GET", "/nothing", "", nil); w.Code != 404 {
		t.Errorf("unknown path: %d", w.Code)
	}
}

func TestChoosingAGateway(t *testing.T) {
	s, chosen := testServer()
	w := do(s, "POST", "/api/gateway", `{"mode":"station","call":"TESTB-1"}`,
		map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:7373"})
	if w.Code != http.StatusNoContent || len(*chosen) != 1 || (*chosen)[0] != "station TESTB-1" {
		t.Fatalf("%d %v", w.Code, *chosen)
	}
	if w := do(s, "POST", "/api/gateway", `{"mode":"nonsense"}`, map[string]string{"Content-Type": "application/json"}); w.Code != 400 {
		t.Fatalf("a bad mode: %d", w.Code)
	}
}

func TestOtherSitesCannotUseTheInterface(t *testing.T) {
	s, chosen := testServer()
	body := `{"mode":"auto"}`
	for name, h := range map[string]map[string]string{
		"foreign origin": {"Content-Type": "application/json", "Origin": "http://evil.example"},
		"form post":      {"Content-Type": "application/x-www-form-urlencoded"},
		"rebound name":   {"Content-Type": "application/json", "Host": "evil.example:7373"},
	} {
		if w := do(s, "POST", "/api/gateway", body, h); w.Code < 400 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if w := do(s, "GET", "/api/status", "", map[string]string{"Host": "evil.example"}); w.Code != http.StatusForbidden {
		t.Errorf("status under a foreign name: %d", w.Code)
	}
	if len(*chosen) != 0 {
		t.Fatalf("a refused request changed the choice: %v", *chosen)
	}
	for _, host := range []string{"localhost:7373", "127.0.0.1:7373", "[::1]:7373"} {
		if w := do(s, "GET", "/api/status", "", map[string]string{"Host": host}); w.Code != 200 {
			t.Errorf("%s: %d", host, w.Code)
		}
	}
}

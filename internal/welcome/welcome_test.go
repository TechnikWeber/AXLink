// SPDX-License-Identifier: GPL-3.0-or-later

package welcome

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func lab() Page {
	return Page{Call: "TESTB-1", Addr: "10.73.0.2", Tags: []string{"INET"}, Gateway: "10.73.0.2:8080",
		Name: "Lab station B", QTH: "a network namespace", Version: "0.2.0-test",
		Text: "First paragraph with <b>no</b> markup.\n\nSecond paragraph."}
}

func get(t *testing.T, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	Handler(lab).ServeHTTP(w, r)
	return w
}

func TestThePageIsSmallAndEscapesTheText(t *testing.T) {
	w := get(t, "/", nil)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "<h1>TESTB-1</h1>") || !strings.Contains(body, "Second paragraph.") {
		t.Fatalf("%d %s", w.Code, body)
	}
	if strings.Contains(body, "<b>no</b>") {
		t.Fatal("the operator's text must be escaped")
	}
	if len(body) > 1500 {
		t.Fatalf("%d bytes: too much for the radio", len(body))
	}
	if !strings.Contains(body, `rel="icon" href="data:,"`) {
		t.Fatal("without an inline icon the browser fetches /favicon.ico over the radio")
	}
}

func TestCompressedWhenTheBrowserAccepts(t *testing.T) {
	plain := get(t, "/", nil).Body.Len()
	w := get(t, "/", map[string]string{"Accept-Encoding": "gzip, deflate"})
	if w.Header().Get("Content-Encoding") != "gzip" || w.Body.Len() >= plain {
		t.Fatalf("%q, %d bytes compressed, %d plain", w.Header().Get("Content-Encoding"), w.Body.Len(), plain)
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(zr); !strings.Contains(string(b), "TESTB-1") {
		t.Fatal("the compressed page does not read back")
	}
}

func TestGermanLabels(t *testing.T) {
	if b := get(t, "/", map[string]string{"Accept-Language": "de-DE,de;q=0.9"}).Body.String(); !strings.Contains(b, "Adresse") || !strings.Contains(b, `lang="de"`) {
		t.Fatal(b)
	}
}

func TestStationJSON(t *testing.T) {
	w := get(t, "/station.json", nil)
	var p Page
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.Call != "TESTB-1" || p.Gateway != "10.73.0.2:8080" {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestOtherPathsAndMethods(t *testing.T) {
	if w := get(t, "/admin", nil); w.Code != http.StatusNotFound {
		t.Fatalf("%d", w.Code)
	}
	w := httptest.NewRecorder()
	Handler(lab).ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("%d", w.Code)
	}
}

func TestOpenPortsAreListed(t *testing.T) {
	p := lab()
	p.Services = []Service{{Proto: "tcp", Port: 22, Note: "SSH <admin>", Some: true}, {Proto: "udp", Port: 7070}}
	w := httptest.NewRecorder()
	Handler(func() Page { return p }).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	b := w.Body.String()
	if !strings.Contains(b, "<li>22/tcp SSH &lt;admin&gt; (for some stations)</li>") || !strings.Contains(b, "<li>7070/udp</li>") ||
		!strings.Contains(b, "ask the operator") {
		t.Fatal(b)
	}
	p.AllOpen = true
	w = httptest.NewRecorder()
	Handler(func() Page { return p }).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if b := w.Body.String(); !strings.Contains(b, "all ports (no firewall)") || strings.Contains(b, "<li>") {
		t.Fatal(b)
	}
}

func TestAnEmptyPageSaysSo(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(func() Page { return Page{Call: "TESTA-1", Addr: "10.73.0.1"} }).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(w.Body.String(), "No introduction yet.") {
		t.Fatal(w.Body.String())
	}
}

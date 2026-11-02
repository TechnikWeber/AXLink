// SPDX-License-Identifier: GPL-3.0-or-later

// Package welcome serves a station's own page on its radio address: who runs
// it, where, with what, and what the station offers. Other stations open it
// over the radio, so it is small (no pictures, no extra files) and goes out
// compressed when the browser accepts that. /station.json gives the same for
// programs.
package welcome

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
)

// Page is what the page shows.
type Page struct {
	Call      string   `json:"call"`
	Addr      string   `json:"ip"`
	Tags      []string `json:"tags,omitempty"`
	Gateway   string   `json:"gateway,omitempty"` // its HTTP proxy, address:port, if it shares the internet
	Name      string   `json:"name,omitempty"`
	Locator   string   `json:"locator,omitempty"`
	QTH       string   `json:"qth,omitempty"`
	Equipment string   `json:"equipment,omitempty"`
	Text      string   `json:"text,omitempty"`
	Version   string   `json:"version"`
}

var labels = map[string]map[string]string{
	"en": {"address": "Address", "roles": "Roles", "equipment": "Equipment", "internet": "Internet",
		"gateway": "gateway, HTTP proxy", "none": "No introduction yet.", "footer": "This page comes over amateur radio."},
	"de": {"address": "Adresse", "roles": "Rollen", "equipment": "Ausrüstung", "internet": "Internet",
		"gateway": "Gateway, HTTP-Proxy", "none": "Noch keine Vorstellung.", "footer": "Diese Seite kommt über Amateurfunk."},
}

var page = template.Must(template.New("p").Parse(`<!doctype html>
<html lang="{{.Lang}}"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="icon" href="data:,"><title>{{.P.Call}}</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:38em;margin:1.5em auto;padding:0 1em;color:#1f2328;background:#fff}h1{margin:0}.s,dt,footer{color:#5d6670}.s{margin:0 0 1em}dl{display:grid;grid-template-columns:max-content 1fr;gap:.2em 1em}dd{margin:0}footer{font-size:.85em;margin-top:2em}</style>
<h1>{{.P.Call}}</h1>{{if .P.Name}}<p class="s">{{.P.Name}}</p>{{end}}
<dl>{{if .P.Locator}}<dt>Locator</dt><dd>{{.P.Locator}}</dd>{{end}}{{if .P.QTH}}<dt>QTH</dt><dd>{{.P.QTH}}</dd>{{end}}{{if .P.Equipment}}<dt>{{.L.equipment}}</dt><dd>{{.P.Equipment}}</dd>{{end}}<dt>{{.L.address}}</dt><dd>{{.P.Addr}}</dd>{{if .P.Tags}}<dt>{{.L.roles}}</dt><dd>{{.Tags}}</dd>{{end}}{{if .P.Gateway}}<dt>{{.L.internet}}</dt><dd>{{.L.gateway}} {{.P.Gateway}}</dd>{{end}}</dl>
{{range .Text}}<p>{{.}}</p>{{else}}<p class="s">{{.L.none}}</p>{{end}}
<footer>AXLink {{.P.Version}} · {{.L.footer}}</footer>
`))

// Handler serves the page; page is called for every request.
func Handler(page func() Page) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		p := page()
		var body bytes.Buffer
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			render(&body, p, lang(r))
		case "/station.json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(&body).Encode(p)
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("Vary", "Accept-Encoding, Accept-Language")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		out := body.Bytes()
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			var z bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&z, gzip.BestCompression)
			_, _ = zw.Write(out)
			_ = zw.Close()
			if z.Len() < len(out) {
				out = z.Bytes()
				w.Header().Set("Content-Encoding", "gzip")
			}
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write(out)
		}
	})
}

func render(b *bytes.Buffer, p Page, lang string) {
	var paragraphs []string
	for _, para := range strings.Split(strings.ReplaceAll(p.Text, "\r", ""), "\n\n") {
		if para = strings.TrimSpace(para); para != "" {
			paragraphs = append(paragraphs, para)
		}
	}
	_ = page.Execute(b, struct {
		Lang string
		P    Page
		L    map[string]string
		Tags string
		Text []string
	}{lang, p, labels[lang], strings.Join(p.Tags, ", "), paragraphs})
}

// lang picks German for browsers that prefer it, else English.
func lang(r *http.Request) string {
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if labels[base] != nil {
			return base
		}
	}
	return "en"
}

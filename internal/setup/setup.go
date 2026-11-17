// SPDX-License-Identifier: GPL-3.0-or-later

// Package setup serves the setup page: a small page in the browser where a
// new operator enters the callsign, picks the radio and the address, and
// starts AXLink, without the command line. It stores the station in the
// settings file, so that "axlinkd run" needs no flags afterwards, and then
// calls Start, which on Linux installs the service (one password prompt).
//
// Like the web interface it answers loopback host names only and accepts
// JSON posts from its own page only; a token in the page guards the one
// request that changes anything.
package setup

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/TechnikWeber/AXLink/internal/pacing"
	"github.com/TechnikWeber/AXLink/internal/settings"
	"github.com/TechnikWeber/AXLink/internal/tnc"
)

// DefaultAddr is where the setup page listens: next to the web interface
// (7373) and the local proxy (7374).
const DefaultAddr = "127.0.0.1:7375"

// Result is what Start reports to the page.
type Result struct {
	// Manual is a command the operator runs instead, when AXLink cannot start
	// itself here (e.g. no pkexec); "" when it started.
	Manual string `json:"manual,omitempty"`
	// Wait says the page should wait for the web interface to come up.
	Wait bool `json:"wait"`
}

// Config wires the page to the program.
type Config struct {
	Store   *settings.Store
	Ports   func() ([]tnc.PortInfo, error) // the serial ports; tnc.ListPorts
	Start   func(settings.Station) (Result, error)
	Running func() bool // the web interface answers
	UIURL   string      // the web interface, e.g. http://127.0.0.1:7373/
	Version string
	Logger  *slog.Logger
	// Done is called once the page has seen the web interface come up; the
	// program may end then.
	Done func()
}

// Server is the setup page.
type Server struct {
	cfg   Config
	token string
	once  sync.Once
}

// New returns the page with a fresh token.
func New(cfg Config) *Server {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{cfg: cfg, token: hex.EncodeToString(b)}
}

// ServeHTTP routes a request after the safety checks.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	if !loopbackName(r.Host) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if ct, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); strings.TrimSpace(ct) != "application/json" {
			http.Error(w, "JSON only", http.StatusUnsupportedMediaType)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Setup-Token")), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	switch {
	case r.URL.Path == "/" && r.Method == http.MethodGet:
		s.index(w, r)
	case r.URL.Path == "/setup.js" && r.Method == http.MethodGet:
		h.Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = io.WriteString(w, script)
	case r.URL.Path == "/setup.css" && r.Method == http.MethodGet:
		h.Set("Content-Type", "text/css; charset=utf-8")
		_, _ = io.WriteString(w, style)
	case r.URL.Path == "/api/running" && r.Method == http.MethodGet:
		running := s.cfg.Running != nil && s.cfg.Running()
		writeJSON(w, http.StatusOK, map[string]any{"running": running, "url": s.cfg.UIURL})
		if running && r.URL.Query().Get("done") == "1" && s.cfg.Done != nil {
			s.once.Do(func() { time.AfterFunc(2*time.Second, s.cfg.Done) })
		}
	case r.URL.Path == "/api/save" && r.Method == http.MethodPost:
		s.save(w, r)
	default:
		http.NotFound(w, r)
	}
}

type saveRequest struct {
	Call    string `json:"call"`
	Port    string `json:"port"`
	IP      string `json:"ip"`
	Profile string `json:"profile"`
}

func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	lang := language(r)
	var req saveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	st := settings.Station{Call: req.Call, Port: req.Port, IP: req.IP, Profile: req.Profile}
	if msg := missing(st, lang); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	next, err := s.cfg.Store.Update(func(x *settings.Settings) { x.Station = st })
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.cfg.Logger.Info("station stored", "call", next.Station.Call, "port", next.Station.Port, "ip", next.Station.IP,
		"file", s.cfg.Store.Path())
	if s.cfg.Start == nil {
		writeJSON(w, http.StatusOK, Result{})
		return
	}
	res, err := s.cfg.Start(next.Station)
	if err != nil {
		s.cfg.Logger.Warn("start", "err", err)
		writeJSON(w, http.StatusOK, map[string]any{"error": tr(lang, "startfail") + " " + err.Error(), "manual": res.Manual})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// missing names a required field that is empty; the rest is checked by the
// settings.
func missing(st settings.Station, lang string) string {
	switch {
	case strings.TrimSpace(st.Call) == "":
		return tr(lang, "needcall")
	case strings.TrimSpace(st.IP) == "":
		return tr(lang, "needip")
	case strings.TrimSpace(st.Port) == "":
		return tr(lang, "needport")
	}
	if _, ok := pacing.ByName(st.Profile); st.Profile != "" && !ok {
		return "unknown profile"
	}
	return ""
}

type portOption struct {
	Value, Label string
	Selected     bool
}

type pageData struct {
	Lang, Other string
	T           map[string]string
	Token       string
	Call, IP    string
	Ports       []portOption
	Profiles    []portOption
	Radio       bool // an F4HWN radio is connected now
	PortErr     string
	Running     bool
	UIURL       string
	Version     string
	File        string
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	lang := language(r)
	st := s.cfg.Store.Get().Station
	d := pageData{Lang: lang, Other: map[string]string{"en": "de", "de": "en"}[lang], T: texts[lang],
		Token: s.token, Call: st.Call, IP: st.IP, UIURL: s.cfg.UIURL, Version: s.cfg.Version,
		File: s.cfg.Store.Path()}
	d.Running = s.cfg.Running != nil && s.cfg.Running()
	chosen := st.Port
	if chosen == "" {
		chosen = "auto"
	}
	d.Ports = append(d.Ports, portOption{Value: "auto", Label: tr(lang, "auto"), Selected: chosen == "auto"})
	found := chosen == "auto"
	if s.cfg.Ports != nil {
		ports, err := s.cfg.Ports()
		if err != nil {
			d.PortErr = err.Error()
		}
		for _, p := range ports {
			label := p.Name
			if p.Product != "" {
				label += " – " + p.Product
			}
			if p.IsF4HWN() {
				label += " – " + tr(lang, "f4hwn")
				d.Radio = true
			}
			d.Ports = append(d.Ports, portOption{Value: p.Name, Label: label, Selected: p.Name == chosen})
			found = found || p.Name == chosen
		}
	}
	if !found { // stored, but not plugged in now
		d.Ports = append(d.Ports, portOption{Value: chosen, Label: chosen + " – " + tr(lang, "absent"), Selected: true})
	}
	for _, p := range []struct{ name, key string }{
		{pacing.F4HWNKissTNC.Name, "p.f4hwn"}, {pacing.F4HWNKissTNCAck.Name, "p.ack"}, {pacing.Generic.Name, "p.generic"},
	} {
		d.Profiles = append(d.Profiles, portOption{Value: p.name, Label: tr(lang, p.key),
			Selected: p.name == st.Profile || (st.Profile == "" && p.name == pacing.F4HWNKissTNC.Name)})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(w, d); err != nil {
		s.cfg.Logger.Warn("setup page", "err", err)
	}
}

// SuggestIP proposes an address in 10.73.0.0/24 from the callsign, so that two
// new stations rarely start on the same one. The page computes the same.
func SuggestIP(call string) string {
	base, _, _ := strings.Cut(strings.ToUpper(strings.TrimSpace(call)), "-")
	sum := 0
	for _, c := range base {
		sum = (sum*31 + int(c)) % 1000003
	}
	return netip.AddrFrom4([4]byte{10, 73, 0, byte(2 + sum%252)}).String() + "/24"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func loopbackName(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	name = strings.Trim(name, "[]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	a, err := netip.ParseAddr(name)
	return err == nil && a.IsLoopback()
}

func language(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); texts[l] != nil {
		return l
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if texts[base] != nil {
			return base
		}
	}
	return "en"
}

func tr(lang, key string) string {
	if t, ok := texts[lang][key]; ok {
		return t
	}
	return texts["en"][key]
}

var texts = map[string]map[string]string{
	"en": {
		"title":     "Set up AXLink",
		"intro":     "Three things, then AXLink runs: your callsign, your radio and your address on the radio network.",
		"call":      "Your callsign",
		"callhint":  "With an SSID for this computer, e.g. N0CALL-1. AXLink does not transmit without your own callsign.",
		"radio":     "Radio",
		"auto":      "Find it automatically (UV-K1 / UV-K5 V3, F4HWN)",
		"f4hwn":     "radio with F4HWN firmware",
		"absent":    "not connected now",
		"noradio":   "No radio with F4HWN firmware is connected right now. That is fine: AXLink waits for it.",
		"kind":      "What runs on the radio",
		"p.f4hwn":   "F4HWN KISS TNC app",
		"p.ack":     "AXLink TNC app (faster)",
		"p.generic": "another KISS TNC",
		"ip":        "Address on the radio network",
		"iphint":    "Every station needs its own address in the same network. Agree on it with the stations you want to reach; the suggestion comes from your callsign.",
		"save":      "Save and start AXLink",
		"saving":    "Starting AXLink …",
		"password":  "Your computer may ask for your password once: AXLink needs the right to create a network adapter.",
		"waiting":   "Waiting for AXLink to come up …",
		"manual":    "AXLink could not start itself here. Run this once in a terminal:",
		"running":   "AXLink is running.",
		"open":      "Open AXLink",
		"change":    "To change the station, edit it below and save again: AXLink restarts.",
		"needcall":  "Enter your callsign.",
		"needip":    "Enter an address, e.g. 10.73.0.1/24.",
		"needport":  "Choose the radio.",
		"startfail": "AXLink did not start:",
		"file":      "Settings file",
	},
	"de": {
		"title":     "AXLink einrichten",
		"intro":     "Drei Angaben, dann läuft AXLink: dein Rufzeichen, dein Funkgerät und deine Adresse im Funknetz.",
		"call":      "Dein Rufzeichen",
		"callhint":  "Mit einer SSID für diesen Computer, z. B. N0CALL-1. Ohne eigenes Rufzeichen sendet AXLink nicht.",
		"radio":     "Funkgerät",
		"auto":      "Automatisch finden (UV-K1 / UV-K5 V3, F4HWN)",
		"f4hwn":     "Funkgerät mit F4HWN-Firmware",
		"absent":    "gerade nicht angeschlossen",
		"noradio":   "Gerade ist kein Funkgerät mit F4HWN-Firmware angeschlossen. Das macht nichts: AXLink wartet darauf.",
		"kind":      "Was auf dem Funkgerät läuft",
		"p.f4hwn":   "F4HWN-App KISS TNC",
		"p.ack":     "App AXLink TNC (schneller)",
		"p.generic": "ein anderer KISS-TNC",
		"ip":        "Adresse im Funknetz",
		"iphint":    "Jede Station braucht eine eigene Adresse im selben Netz. Sprich sie mit den Stationen ab, die du erreichen willst; der Vorschlag kommt aus deinem Rufzeichen.",
		"save":      "Speichern und AXLink starten",
		"saving":    "AXLink startet …",
		"password":  "Dein Computer fragt eventuell einmal nach deinem Passwort: AXLink braucht das Recht, einen Netzwerkadapter anzulegen.",
		"waiting":   "Warte, bis AXLink läuft …",
		"manual":    "AXLink konnte sich hier nicht selbst starten. Führe das einmal im Terminal aus:",
		"running":   "AXLink läuft.",
		"open":      "AXLink öffnen",
		"change":    "Um die Station zu ändern, unten anpassen und erneut speichern: AXLink startet neu.",
		"needcall":  "Gib dein Rufzeichen ein.",
		"needip":    "Gib eine Adresse ein, z. B. 10.73.0.1/24.",
		"needport":  "Wähle das Funkgerät.",
		"startfail": "AXLink ist nicht gestartet:",
		"file":      "Einstellungsdatei",
	},
}

var page = template.Must(template.New("setup").Parse(`<!doctype html>
<html lang="{{.Lang}}"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="icon" href="data:,"><link rel="stylesheet" href="/setup.css"><title>{{.T.title}}</title>
<body data-token="{{.Token}}" data-ui="{{.UIURL}}" data-lang="{{.Lang}}"
 data-saving="{{.T.saving}}" data-waiting="{{.T.waiting}}" data-manual="{{.T.manual}}">
<header><span class="brand">AXLink</span><span class="ver">{{.Version}}</span>
<a class="lang" href="/?lang={{.Other}}">{{if eq .Lang "de"}}English{{else}}Deutsch{{end}}</a></header>
<main>
<h1>{{.T.title}}</h1>
{{if .Running}}<p class="ok">{{.T.running}} <a class="button" href="{{.UIURL}}">{{.T.open}}</a></p><p class="muted">{{.T.change}}</p>
{{else}}<p>{{.T.intro}}</p>{{end}}
<form id="f" novalidate>
<label for="call">{{.T.call}}</label>
<input id="call" name="call" value="{{.Call}}" autocomplete="off" autocapitalize="characters" spellcheck="false" required>
<p class="hint">{{.T.callhint}}</p>

<label for="port">{{.T.radio}}</label>
<select id="port" name="port">{{range .Ports}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}</select>
{{if not .Radio}}<p class="hint">{{.T.noradio}}</p>{{end}}{{if .PortErr}}<p class="hint bad">{{.PortErr}}</p>{{end}}

<label for="profile">{{.T.kind}}</label>
<select id="profile" name="profile">{{range .Profiles}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}</select>

<label for="ip">{{.T.ip}}</label>
<input id="ip" name="ip" value="{{.IP}}" placeholder="10.73.0.1/24" autocomplete="off" spellcheck="false" required{{if .IP}} data-edited="1"{{end}}>
<p class="hint">{{.T.iphint}}</p>

<p class="hint">{{.T.password}}</p>
<button id="save" type="submit">{{.T.save}}</button>
<p id="msg" role="status" aria-live="polite"></p>
<pre id="cmd" hidden></pre>
</form>
{{if .File}}<p class="foot">{{.T.file}}: <code>{{.File}}</code></p>{{end}}
</main>
<script src="/setup.js"></script>
`))

const style = `:root{--bg:#f4f6f8;--card:#fff;--text:#1f2328;--muted:#5d6670;--line:#d8dee4;--accent:#0b62c4;--ok:#1a7f37;--bad:#c62f2f}
@media (prefers-color-scheme:dark){:root{--bg:#0f1419;--card:#171c22;--text:#e6edf3;--muted:#9aa5b1;--line:#2d333b;--accent:#4c9aff;--ok:#3fb950;--bad:#f85149}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
header{display:flex;align-items:baseline;gap:10px;padding:12px 16px;background:var(--card);border-bottom:1px solid var(--line)}
.brand{font-weight:700;font-size:18px;color:var(--accent)}.ver{color:var(--muted);font-size:13px}.lang{margin-left:auto;color:var(--muted);font-size:14px}
main{max-width:36em;margin:24px auto;padding:20px;background:var(--card);border:1px solid var(--line);border-radius:8px}
@media (max-width:40em){main{margin:12px 16px}}
h1{font-size:22px;margin:0 0 8px}label{display:block;font-weight:600;margin:18px 0 4px}
input,select{width:100%;font:inherit;padding:8px 10px;border:1px solid var(--line);border-radius:6px;background:var(--card);color:var(--text)}
#call,#ip{font-family:ui-monospace,Menlo,Consolas,monospace}
.hint,.muted,.foot{color:var(--muted);font-size:14px;margin:4px 0 0}.bad{color:var(--bad)}.ok{color:var(--ok);font-weight:600}
button,.button{display:inline-block;margin-top:20px;font:inherit;font-weight:600;padding:10px 18px;border:0;border-radius:6px;background:var(--accent);color:#fff;cursor:pointer;text-decoration:none}
.ok .button{margin:0 0 0 10px}button:disabled{opacity:.6;cursor:wait}
#msg{min-height:1.5em}pre{white-space:pre-wrap;word-break:break-all;background:var(--bg);border:1px solid var(--line);border-radius:6px;padding:10px;font-size:14px}
.foot{margin-top:24px;border-top:1px solid var(--line);padding-top:12px}code{font-size:13px}`

const script = `(function(){
var b=document.body,f=document.getElementById('f'),call=document.getElementById('call'),ip=document.getElementById('ip'),
 msg=document.getElementById('msg'),cmd=document.getElementById('cmd'),btn=document.getElementById('save');
function suggest(c){c=c.trim().toUpperCase().split('-')[0];var s=0;for(var i=0;i<c.length;i++){s=(s*31+c.charCodeAt(i))%1000003}return '10.73.0.'+(2+s%252)+'/24'}
call.addEventListener('input',function(){if(!ip.dataset.edited&&call.value.trim()){ip.value=suggest(call.value)}});
ip.addEventListener('input',function(){ip.dataset.edited='1'});
function say(t,bad){msg.textContent=t;msg.className=bad?'bad':''}
function wait(){say(b.dataset.waiting);var tries=0;(function poll(){fetch('/api/running?done=1').then(function(r){return r.json()}).then(function(j){
 if(j.running){location.href=j.url;return}if(++tries<120){setTimeout(poll,1000)}}).catch(function(){setTimeout(poll,1000)})})()}
f.addEventListener('submit',function(e){e.preventDefault();btn.disabled=true;cmd.hidden=true;say(b.dataset.saving);
 fetch('/api/save?lang='+b.dataset.lang,{method:'POST',headers:{'Content-Type':'application/json','X-Setup-Token':b.dataset.token},
  body:JSON.stringify({call:call.value,port:f.port.value,profile:f.profile.value,ip:ip.value})})
 .then(function(r){return r.json()}).then(function(j){btn.disabled=false;
  if(j.manual){cmd.textContent=j.manual;cmd.hidden=false;say((j.error?j.error+' ':'')+b.dataset.manual,!!j.error);if(j.wait!==false){wait()}return}
  if(j.error){say(j.error,true);return}
  if(j.wait){btn.disabled=true;wait()}})
 .catch(function(err){btn.disabled=false;say(String(err),true)})});
})();`

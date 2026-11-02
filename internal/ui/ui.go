// SPDX-License-Identifier: GPL-3.0-or-later

// Package ui serves AXLink's web interface: this station, the stations heard,
// the choice of an internet gateway and the traffic counters, in English and
// German. It needs no internet: page, style and script are built in. Station
// data comes from the radio and is shown as text only.
package ui

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/TechnikWeber/AXLink/internal/firewall"
	"github.com/TechnikWeber/AXLink/internal/monitor"
	"github.com/TechnikWeber/AXLink/internal/settings"
)

//go:embed assets
var assets embed.FS

// Status is what the page shows. The node fills it in.
type Status struct {
	Version  string    `json:"version"`
	Call     string    `json:"call"`
	Addr     string    `json:"ip"`
	Tags     []string  `json:"tags"`
	Beacon   int       `json:"beacon_s"` // 0: no beacons, listening only
	Uptime   int64     `json:"uptime_s"`
	Adapter  Adapter   `json:"adapter"`
	TNC      TNC       `json:"tnc"`
	Stations []Station `json:"stations"`
	Internet Internet  `json:"internet"`
	Gateway  *Gateway  `json:"gateway,omitempty"` // nil unless this station is a gateway
	Counters []Counter `json:"counters"`
	Monitor  Monitor   `json:"monitor"`
	Firewall Firewall  `json:"firewall"`
	Welcome  string    `json:"welcome,omitempty"` // the own welcome page on the radio address
	Settings string    `json:"settings_file"`     // where the settings are stored; "" for nowhere
}

// Monitor is the live view: one sample every Interval seconds.
type Monitor struct {
	Interval int              `json:"interval_s"`
	QueueCap int              `json:"queue_cap"`
	Samples  []monitor.Sample `json:"samples"`
}

// Firewall is what the firewall did.
type Firewall struct {
	Mode     string             `json:"mode"`
	Ping     bool               `json:"ping"`
	Silent   bool               `json:"silent"` // refused packets get no answer
	Passed   uint64             `json:"passed"`
	Blocked  uint64             `json:"blocked"`
	Answered uint64             `json:"answered"` // blocked packets answered with a reset or ICMP
	Recent   []firewall.Blocked `json:"recent"`
}

// Adapter is the virtual network adapter.
type Adapter struct {
	Name     string `json:"name"`
	MTU      int    `json:"mtu"`
	RadioMTU int    `json:"radio_mtu"`
}

// TNC is the connection to the radio or TNC.
type TNC struct {
	Name      string `json:"name"`
	Profile   string `json:"profile"`
	Connected bool   `json:"connected"`
	For       int64  `json:"for_s"` // seconds since it connected
}

// Station is one station heard.
type Station struct {
	Call     string   `json:"call"`
	Addr     string   `json:"ip"`
	Tags     []string `json:"tags"`
	Proxy    string   `json:"proxy,omitempty"` // its internet gateway, address:port
	Locator  string   `json:"locator,omitempty"`
	Km       *float64 `json:"km,omitempty"`      // distance from the own locator
	Bearing  string   `json:"bearing,omitempty"` // N, NE, ... from the own locator
	HeardAgo int64    `json:"heard_s"`
	Page     string   `json:"page"` // its welcome page
}

// Internet is the client's way to the internet.
type Internet struct {
	Mode       string `json:"mode"`                  // off, auto, station
	Call       string `json:"call,omitempty"`        // the chosen station
	Using      string `json:"using,omitempty"`       // the gateway in use now
	Upstream   string `json:"upstream,omitempty"`    // its proxy
	LocalProxy string `json:"local_proxy,omitempty"` // "" when the local proxy is off
}

// Gateway is this station's own gateway.
type Gateway struct {
	Listen string   `json:"listen"`
	LAN    bool     `json:"lan"`
	Only   []string `json:"only,omitempty"`
}

// Counter is one number in the traffic section. Key names it ("c."+Key in
// the texts), Group places it.
type Counter struct {
	Group string `json:"group"`
	Key   string `json:"key"`
	Value uint64 `json:"value"`
	Bytes bool   `json:"bytes,omitempty"`
}

// Config sets up the server.
type Config struct {
	Status     func() Status
	SetGateway func(mode, call string) error
	// Settings returns the stored settings; Save checks, applies and stores
	// new ones and returns what is now in effect.
	Settings func() settings.Settings
	Save     func(settings.Settings) (settings.Settings, error)
	// Welcome serves the own welcome page for the preview under /welcome/.
	Welcome http.Handler
	Logger  *slog.Logger
}

// Server is the web interface.
type Server struct {
	cfg      Config
	page     *template.Template
	static   http.Handler
	loopback bool // listening on a loopback address: only loopback host names are accepted
}

// New prepares the server.
func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	sub, _ := fs.Sub(assets, "assets")
	return &Server{
		cfg:    cfg,
		page:   template.Must(template.ParseFS(assets, "assets/index.html")),
		static: http.StripPrefix("/static/", http.FileServer(http.FS(sub))),
	}
}

// Serve answers on ln until ctx ends.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if ap, err := netip.ParseAddrPort(ln.Addr().String()); err == nil {
		s.loopback = ap.Addr().IsLoopback()
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second,
		ErrorLog: slog.NewLogLogger(s.cfg.Logger.Handler(), slog.LevelDebug)}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	err := srv.Serve(ln)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// ServeHTTP routes a request after the safety checks.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
	// A web page elsewhere must not use the interface: a foreign host name
	// means DNS rebinding, a foreign Origin a cross-site request.
	if s.loopback && !loopbackName(r.Host) {
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
	}
	switch {
	case r.URL.Path == "/" && r.Method == http.MethodGet:
		s.index(w, r)
	case r.URL.Path == "/api/status" && r.Method == http.MethodGet:
		h.Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, s.cfg.Status())
	case r.URL.Path == "/api/gateway" && r.Method == http.MethodPost:
		s.gateway(w, r)
	case r.URL.Path == "/api/settings" && r.Method == http.MethodGet && s.cfg.Settings != nil:
		h.Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, s.cfg.Settings())
	case r.URL.Path == "/api/settings" && r.Method == http.MethodPost && s.cfg.Save != nil:
		s.save(w, r)
	case strings.HasPrefix(r.URL.Path, "/welcome/") && r.Method == http.MethodGet && s.cfg.Welcome != nil:
		http.StripPrefix("/welcome", s.cfg.Welcome).ServeHTTP(w, r)
	case strings.HasPrefix(r.URL.Path, "/static/") && r.Method == http.MethodGet:
		h.Set("Cache-Control", "no-cache")
		s.static.ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	lang, chosen := language(r)
	if chosen {
		http.SetCookie(w, &http.Cookie{Name: langCookie, Value: lang, Path: "/", MaxAge: 365 * 24 * 3600,
			SameSite: http.SameSiteStrictMode})
	}
	st := s.cfg.Status()
	data := struct {
		Lang, Call, Version string
		T                   map[string]string
	}{lang, st.Call, st.Version, texts[lang]}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.page.Execute(w, data); err != nil {
		s.cfg.Logger.Debug("ui: page", "err", err)
	}
}

func (s *Server) gateway(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
		Call string `json:"call"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	if err := s.cfg.SetGateway(req.Mode, req.Call); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.cfg.Logger.Info("ui: internet gateway chosen", "mode", req.Mode, "call", req.Call)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	var next settings.Settings
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&next); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request: " + err.Error()})
		return
	}
	saved, err := s.cfg.Save(next)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "settings": saved})
		return
	}
	s.cfg.Logger.Info("ui: settings saved")
	writeJSON(w, http.StatusOK, saved)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// loopbackName reports whether a Host header names this computer.
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

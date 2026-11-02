// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TechnikWeber/AXLink/internal/discovery"
	"github.com/TechnikWeber/AXLink/internal/firewall"
	"github.com/TechnikWeber/AXLink/internal/gateway"
	"github.com/TechnikWeber/AXLink/internal/ipbridge"
	"github.com/TechnikWeber/AXLink/internal/monitor"
	"github.com/TechnikWeber/AXLink/internal/settings"
	"github.com/TechnikWeber/AXLink/internal/tnc"
	"github.com/TechnikWeber/AXLink/internal/tun"
	"github.com/TechnikWeber/AXLink/internal/ui"
	"github.com/TechnikWeber/AXLink/internal/welcome"
)

// node holds the running parts. The web interface shows them and changes the
// settings; a change takes effect at once and is stored.
type node struct {
	cfg       nodeConfig
	ctx       context.Context
	log       *slog.Logger
	started   time.Time
	dev       tun.Device
	port      *tnc.Port
	bridge    *ipbridge.Bridge
	disc      *discovery.Service
	fw        *firewall.Firewall
	mon       *monitor.Monitor
	store     *settings.Store
	choice    *gateway.Choice
	gw        *gateway.Proxy // made once; its listener follows the settings
	local     *gateway.Local // nil while the local proxy is off
	localAddr string

	system atomic.Pointer[[]firewall.Rule] // ports the program serves itself

	mu      sync.Mutex // guards the services below and applying settings
	gwAddr  string     // "" while the internet connection is not shared
	gwStop  context.CancelFunc
	welAddr string // "" while the welcome page is off
	welStop context.CancelFunc
}

// systemRules opens the ports of the welcome page and the gateway.
func (n *node) systemRules() []firewall.Rule {
	if r := n.system.Load(); r != nil {
		return *r
	}
	return nil
}

func (n *node) updateSystemRules() {
	var rules []firewall.Rule
	for _, addr := range []string{n.welAddr, n.gwAddr} {
		if ap, err := netip.ParseAddrPort(addr); err == nil {
			rules = append(rules, firewall.Rule{Proto: "tcp", Port: ap.Port()})
		}
	}
	n.system.Store(&rules)
}

// serve runs handler on a new listener at addr until the returned stop is called.
func (n *node) serve(addr string, handler http.Handler) (context.CancelFunc, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(n.ctx)
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute,
		ErrorLog: slog.NewLogLogger(n.log.Handler(), slog.LevelDebug)}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	go srv.Serve(ln)
	return cancel, nil
}

// applyShare starts or stops sharing the internet connection and announces it.
func (n *node) applyShare(s settings.Share) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.gw.SetAccess(s.LAN, s.Only)
	addr := ""
	if s.Enabled {
		addr = netip.AddrPortFrom(n.cfg.tun.Address.Addr(), s.Port).String()
	}
	if addr != n.gwAddr {
		if n.gwStop != nil {
			n.gwStop()
			n.gwStop, n.gwAddr = nil, ""
		}
		if addr != "" {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				n.log.Warn("gateway not started", "listen", addr, "err", err)
			} else {
				ctx, cancel := context.WithCancel(n.ctx)
				go n.gw.Serve(ctx, ln)
				n.gwStop, n.gwAddr = cancel, addr
			}
		}
	}
	tags, proxy := n.cfg.tags, uint16(0)
	if n.gwAddr != "" {
		proxy = s.Port
		if !slices.Contains(tags, discovery.TagInet) {
			tags = append(slices.Clone(tags), discovery.TagInet)
		}
		n.log.Info("gateway: sharing the internet connection", "listen", n.gwAddr, "lan", s.LAN, "only", s.Only)
	}
	if err := n.disc.SetAnnounce(tags, proxy); err != nil {
		n.log.Warn("beacon not changed", "err", err)
	}
	n.updateSystemRules()
}

// applyWelcome starts, moves or stops the welcome page.
func (n *node) applyWelcome(w settings.Welcome) {
	n.mu.Lock()
	defer n.mu.Unlock()
	addr := ""
	if w.Enabled {
		addr = netip.AddrPortFrom(n.cfg.tun.Address.Addr(), w.Port).String()
	}
	if addr == n.welAddr {
		return
	}
	if n.welStop != nil {
		n.welStop()
		n.welStop, n.welAddr = nil, ""
	}
	if addr != "" {
		stop, err := n.serve(addr, welcome.Handler(n.welcomePage))
		if err != nil {
			n.log.Warn("welcome page not started", "listen", addr, "err", err)
		} else {
			n.welStop, n.welAddr = stop, addr
			n.log.Info("welcome page", "url", "http://"+addr+"/")
		}
	}
	n.updateSystemRules()
}

func (n *node) welcomePage() welcome.Page {
	s := n.store.Get().Welcome
	tags, proxy := n.disc.Announce()
	p := welcome.Page{Call: n.cfg.call.String(), Addr: n.cfg.tun.Address.Addr().String(), Tags: tags,
		Name: s.Name, Locator: s.Locator, QTH: s.QTH, Equipment: s.Equipment, Text: s.Text, Version: version}
	if proxy != 0 {
		p.Gateway = netip.AddrPortFrom(n.cfg.tun.Address.Addr(), proxy).String()
	}
	return p
}

// save checks, stores and applies new settings.
func (n *node) save(next settings.Settings) (settings.Settings, error) {
	old := n.store.Get()
	saved, err := n.store.Update(func(s *settings.Settings) { *s = next })
	if err != nil {
		return saved, err
	}
	n.apply(old, saved)
	return saved, nil
}

// apply puts settings into effect; old are the ones in effect before.
func (n *node) apply(old, s settings.Settings) {
	if s.Gateway != old.Gateway {
		_ = n.choice.Set(gateway.Mode(s.Gateway.Mode), s.Gateway.Call)
	}
	if s.Share.Enabled != old.Share.Enabled || s.Share.Port != old.Share.Port || s.Share.LAN != old.Share.LAN ||
		!slices.Equal(s.Share.Only, old.Share.Only) {
		n.applyShare(s.Share)
	}
	if s.Welcome.Enabled != old.Welcome.Enabled || s.Welcome.Port != old.Welcome.Port {
		n.applyWelcome(s.Welcome)
	}
	n.fw.Set(s.Firewall)
}

// setGateway is the choice in the overview; it is stored like any setting.
func (n *node) setGateway(mode, call string) error {
	next := n.store.Get()
	next.Gateway = settings.Gateway{Mode: mode, Call: call}
	if mode != string(gateway.ModeStation) {
		next.Gateway.Call = ""
	}
	_, err := n.save(next)
	return err
}

// candidates lists the internet gateways heard: stations with the tag INET
// that announce a proxy.
func (n *node) candidates() []gateway.Candidate {
	var out []gateway.Candidate
	for _, s := range n.disc.Stations() {
		if s.Proxy != 0 && slices.Contains(s.Tags, discovery.TagInet) {
			out = append(out, gateway.Candidate{Call: s.Call.String(),
				Proxy: netip.AddrPortFrom(s.Addr.Addr(), s.Proxy), Heard: s.Heard})
		}
	}
	return out
}

// direct resolves hosts the local proxy reaches over the radio itself: an
// address in the own radio network or of a station heard, or "<call>.axl".
func (n *node) direct(host string) (netip.Addr, bool) {
	if a, err := netip.ParseAddr(host); err == nil {
		a = a.Unmap()
		if n.cfg.tun.Address.Masked().Contains(a) {
			return a, true
		}
		if _, ok := n.disc.Lookup(a); ok {
			return a, true
		}
		return netip.Addr{}, false
	}
	name, ok := strings.CutSuffix(strings.ToLower(strings.TrimSuffix(host, ".")), ".axl")
	if !ok {
		return netip.Addr{}, false
	}
	if strings.EqualFold(name, n.cfg.call.String()) {
		return n.cfg.tun.Address.Addr(), true
	}
	for _, s := range n.disc.Stations() {
		if strings.EqualFold(name, s.Call.String()) {
			return s.Addr.Addr(), true
		}
	}
	return netip.Addr{}, false
}

func (n *node) totals() monitor.Totals {
	s := n.port.Stats()
	return monitor.Totals{BytesIn: s.BytesIn, BytesOut: s.BytesOut, AirOwn: s.AirOut, AirHeard: s.AirHeard,
		Queue: n.port.QueueLen()}
}

func (n *node) status() ui.Status {
	tags, _ := n.disc.Announce()
	st := ui.Status{
		Version: version, Call: n.cfg.call.String(), Addr: n.cfg.tun.Address.String(), Tags: tags,
		Beacon: int(n.cfg.beacon / time.Second), Uptime: int64(time.Since(n.started) / time.Second),
		Adapter:  ui.Adapter{Name: n.dev.Name(), MTU: n.dev.MTU(), RadioMTU: n.cfg.linkMTU},
		Settings: n.store.Path(),
	}
	connected, since := n.port.Connected()
	st.TNC = ui.TNC{Name: n.port.Name(), Profile: n.port.Profile().Name, Connected: connected}
	if connected {
		st.TNC.For = int64(time.Since(since) / time.Second)
	}
	for _, s := range n.disc.Stations() {
		row := ui.Station{Call: s.Call.String(), Addr: s.Addr.String(), Tags: s.Tags,
			HeardAgo: int64(time.Since(s.Heard) / time.Second), Page: "http://" + s.Addr.Addr().String() + "/"}
		if s.Proxy != 0 {
			row.Proxy = netip.AddrPortFrom(s.Addr.Addr(), s.Proxy).String()
		}
		st.Stations = append(st.Stations, row)
	}
	mode, call := n.choice.Get()
	st.Internet = ui.Internet{Mode: string(mode), Call: call, LocalProxy: n.localAddr}
	if up, using, ok := n.choice.Upstream(); ok {
		st.Internet.Using, st.Internet.Upstream = using, up.String()
	}
	var ls *gateway.LocalStats
	if n.local != nil {
		s := n.local.Stats()
		ls = &s
	}
	n.mu.Lock()
	gwAddr, welAddr := n.gwAddr, n.welAddr
	n.mu.Unlock()
	var gs *gateway.Stats
	if gwAddr != "" {
		s := n.gw.Stats()
		gs = &s
		share := n.store.Get().Share
		st.Gateway = &ui.Gateway{Listen: gwAddr, LAN: share.LAN, Only: share.Only}
	}
	if welAddr != "" {
		st.Welcome = "http://" + strings.TrimSuffix(welAddr, ":80") + "/"
	}
	st.Counters = counters(n.port.Stats(), n.bridge.Stats(), n.disc.Stats(), ls, gs)
	fwc, fws := n.fw.Config(), n.fw.Stats()
	st.Firewall = ui.Firewall{Mode: fwc.Mode, Ping: fwc.Ping, Passed: fws.Passed, Blocked: fws.Blocked, Recent: n.fw.Recent()}
	samples := n.mon.Samples()
	if keep := 150; len(samples) > keep { // five minutes
		samples = samples[len(samples)-keep:]
	}
	st.Monitor = ui.Monitor{Interval: int(n.mon.Interval() / time.Second), QueueCap: n.port.QueueCap(), Samples: samples}
	return st
}

// counters are the numbers of the traffic section; every key needs a text
// "c.<key>" in package ui.
func counters(p tnc.Stats, b ipbridge.Stats, d discovery.Stats, l *gateway.LocalStats, g *gateway.Stats) []ui.Counter {
	c := func(group, key string, v uint64) ui.Counter { return ui.Counter{Group: group, Key: key, Value: v} }
	size := func(group, key string, v uint64) ui.Counter {
		return ui.Counter{Group: group, Key: key, Value: v, Bytes: true}
	}
	out := []ui.Counter{
		c("radio", "frames_out", p.FramesOut), c("radio", "frames_in", p.FramesIn),
		size("radio", "bytes_out", p.BytesOut), size("radio", "bytes_in", p.BytesIn),
		c("radio", "replaced", p.Replaced), c("radio", "repeats", p.Repeats),
		c("radio", "queue_full", p.QueueFull), c("radio", "stale", p.Stale),
		c("radio", "reconnects", p.Disconnects),
		c("ip", "to_radio", b.ToRadio), c("ip", "from_radio", b.FromRadio),
		c("ip", "no_route", b.NoRoute), c("ip", "filtered", b.Filtered),
		c("ip", "not_for_us", b.NotForUs), c("ip", "wrong_address", b.WrongAddress),
		c("ip", "blocked", b.Blocked), c("ip", "bad_packet", b.BadPacket),
		c("ip", "clamped", b.Clamped), c("ip", "fragmented", b.Fragmented), c("ip", "too_big", b.TooBig),
		c("beacons", "beacons_sent", d.Sent), c("beacons", "beacons_heard", d.Heard),
		c("beacons", "beacons_invalid", d.Invalid+d.Relayed),
	}
	if l != nil {
		out = append(out, c("internet", "requests", l.Requests), c("internet", "tunnels", l.Tunnels),
			c("internet", "direct", l.Direct),
			size("internet", "bytes_in", l.BytesIn), size("internet", "bytes_out", l.BytesOut),
			c("internet", "no_gateway", l.NoGateway), c("internet", "failed", l.Failed))
	}
	if g != nil {
		out = append(out, c("gateway", "requests", g.Requests), c("gateway", "tunnels", g.Tunnels),
			size("gateway", "gw_bytes_in", g.BytesIn), size("gateway", "gw_bytes_out", g.BytesOut),
			c("gateway", "refused", g.Refused), c("gateway", "failed", g.Failed))
	}
	return out
}

// startLocal opens the local proxy to the chosen gateway; a failure is
// reported, not fatal: the radio works without it.
func startLocal(n *node) {
	if n.cfg.localProxy == "" {
		return
	}
	ln, err := net.Listen("tcp", n.cfg.localProxy)
	if err != nil {
		n.log.Warn("local proxy not started", "listen", n.cfg.localProxy, "err", err)
		return
	}
	hint := "Choose an internet gateway in the web interface."
	if n.cfg.ui != "" {
		hint = "Choose an internet gateway at http://" + n.cfg.ui + "/."
	}
	n.local = gateway.NewLocal(gateway.LocalConfig{Upstream: n.choice.Upstream, Direct: n.direct, Hint: hint, Logger: n.log})
	n.localAddr = ln.Addr().String()
	go n.local.Serve(n.ctx, ln)
	n.log.Info("local proxy to the internet gateway and the radio stations", "listen", n.localAddr)
}

// startUI opens the web interface; a failure is reported, not fatal.
func startUI(n *node) {
	if n.cfg.ui == "" {
		return
	}
	ln, err := net.Listen("tcp", n.cfg.ui)
	if err != nil {
		n.log.Warn("web interface not started", "listen", n.cfg.ui, "err", err)
		return
	}
	srv := ui.New(ui.Config{Status: n.status, SetGateway: n.setGateway, Settings: n.store.Get, Save: n.save,
		Welcome: welcome.Handler(n.welcomePage), Logger: n.log})
	go srv.Serve(n.ctx, ln)
	n.log.Info("web interface", "url", "http://"+ln.Addr().String()+"/")
}

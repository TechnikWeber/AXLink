// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"time"

	"github.com/TechnikWeber/AXLink/internal/discovery"
	"github.com/TechnikWeber/AXLink/internal/gateway"
	"github.com/TechnikWeber/AXLink/internal/ipbridge"
	"github.com/TechnikWeber/AXLink/internal/tnc"
	"github.com/TechnikWeber/AXLink/internal/tun"
	"github.com/TechnikWeber/AXLink/internal/ui"
)

// node holds the running parts that the web interface shows.
type node struct {
	cfg       nodeConfig
	started   time.Time
	dev       tun.Device
	port      *tnc.Port
	bridge    *ipbridge.Bridge
	disc      *discovery.Service
	gw        *gateway.Proxy // nil unless this station is a gateway
	gwAddr    string
	choice    *gateway.Choice
	local     *gateway.Local // nil while the local proxy is off
	localAddr string
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

func (n *node) setGateway(mode, call string) error { return n.choice.Set(gateway.Mode(mode), call) }

func (n *node) status() ui.Status {
	st := ui.Status{
		Version: version, Call: n.cfg.call.String(), Addr: n.cfg.tun.Address.String(), Tags: n.cfg.tags,
		Beacon: int(n.cfg.beacon / time.Second), Uptime: int64(time.Since(n.started) / time.Second),
		Adapter: ui.Adapter{Name: n.dev.Name(), MTU: n.dev.MTU(), RadioMTU: n.cfg.linkMTU},
	}
	connected, since := n.port.Connected()
	st.TNC = ui.TNC{Name: n.port.Name(), Profile: n.port.Profile().Name, Connected: connected}
	if connected {
		st.TNC.For = int64(time.Since(since) / time.Second)
	}
	for _, s := range n.disc.Stations() {
		row := ui.Station{Call: s.Call.String(), Addr: s.Addr.String(), Tags: s.Tags,
			HeardAgo: int64(time.Since(s.Heard) / time.Second)}
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
	var gs *gateway.Stats
	if n.gw != nil {
		s := n.gw.Stats()
		gs = &s
		st.Gateway = &ui.Gateway{Listen: n.gwAddr, LAN: n.cfg.proxyLAN}
	}
	st.Counters = counters(n.port.Stats(), n.bridge.Stats(), n.disc.Stats(), ls, gs)
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
		c("radio", "replaced", p.Replaced), c("radio", "repeats", p.Repeats),
		c("radio", "queue_full", p.QueueFull), c("radio", "stale", p.Stale),
		c("radio", "reconnects", p.Disconnects),
		c("ip", "to_radio", b.ToRadio), c("ip", "from_radio", b.FromRadio),
		c("ip", "no_route", b.NoRoute), c("ip", "filtered", b.Filtered),
		c("ip", "not_for_us", b.NotForUs), c("ip", "bad_packet", b.BadPacket),
		c("ip", "clamped", b.Clamped), c("ip", "fragmented", b.Fragmented), c("ip", "too_big", b.TooBig),
		c("beacons", "beacons_sent", d.Sent), c("beacons", "beacons_heard", d.Heard),
		c("beacons", "beacons_invalid", d.Invalid+d.Relayed),
	}
	if l != nil {
		out = append(out, c("internet", "requests", l.Requests), c("internet", "tunnels", l.Tunnels),
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
func startLocal(ctx context.Context, n *node, log *slog.Logger) {
	if n.cfg.localProxy == "" {
		return
	}
	ln, err := net.Listen("tcp", n.cfg.localProxy)
	if err != nil {
		log.Warn("local proxy not started", "listen", n.cfg.localProxy, "err", err)
		return
	}
	hint := "Choose an internet gateway in the web interface."
	if n.cfg.ui != "" {
		hint = "Choose an internet gateway at http://" + n.cfg.ui + "/."
	}
	n.local = gateway.NewLocal(gateway.LocalConfig{Upstream: n.choice.Upstream, Hint: hint, Logger: log})
	n.localAddr = ln.Addr().String()
	go n.local.Serve(ctx, ln)
	log.Info("local proxy to the internet gateway", "listen", n.localAddr)
}

// startUI opens the web interface; a failure is reported, not fatal.
func startUI(ctx context.Context, n *node, log *slog.Logger) {
	if n.cfg.ui == "" {
		return
	}
	ln, err := net.Listen("tcp", n.cfg.ui)
	if err != nil {
		log.Warn("web interface not started", "listen", n.cfg.ui, "err", err)
		return
	}
	srv := ui.New(ui.Config{Status: n.status, SetGateway: n.setGateway, Logger: log})
	go srv.Serve(ctx, ln)
	log.Info("web interface", "url", "http://"+ln.Addr().String()+"/")
}

// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/capture"
	"github.com/TechnikWeber/AXLink/internal/chat"
	"github.com/TechnikWeber/AXLink/internal/discovery"
	"github.com/TechnikWeber/AXLink/internal/firewall"
	"github.com/TechnikWeber/AXLink/internal/forward"
	"github.com/TechnikWeber/AXLink/internal/gateway"
	"github.com/TechnikWeber/AXLink/internal/ipbridge"
	"github.com/TechnikWeber/AXLink/internal/mesh"
	"github.com/TechnikWeber/AXLink/internal/monitor"
	"github.com/TechnikWeber/AXLink/internal/pacing"
	"github.com/TechnikWeber/AXLink/internal/settings"
	"github.com/TechnikWeber/AXLink/internal/tnc"
	"github.com/TechnikWeber/AXLink/internal/tun"
	"github.com/TechnikWeber/AXLink/internal/txqueue"
)

// routeFlags collects -route IP/PREFIX=CALLSIGN.
type routeFlags []ipbridge.Route

func (r *routeFlags) String() string { return fmt.Sprint(*r) }

func (r *routeFlags) Set(v string) error {
	prefix, call, ok := strings.Cut(v, "=")
	if !ok {
		return errors.New("want IP/PREFIX=CALLSIGN")
	}
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		a, aerr := netip.ParseAddr(prefix) // a plain address means /32
		if aerr != nil {
			return err
		}
		p = netip.PrefixFrom(a, a.BitLen())
	}
	c, err := ax25.ParseAddress(call)
	if err != nil {
		return err
	}
	*r = append(*r, ipbridge.Route{Prefix: p, Call: c})
	return nil
}

type nodeConfig struct {
	call           ax25.Address
	dialer         tnc.Dialer
	profile        pacing.Profile
	tun            tun.Config
	linkMTU        int
	routes         []ipbridge.Route
	allowBroadcast bool
	tags           []string // given with -tags; INET is added while sharing, RELAY while relaying
	beacon         time.Duration
	ui             string // address of the web interface; "" for none
	localProxy     string // address of the local proxy to the chosen gateway; "" for none
	configPath     string // the settings file; "" keeps the settings in memory
	pcap           string // a file for every frame, for Wireshark; "" for none
	debug          bool   // every frame into the log
	// overrides are the options that change stored settings (-proxy,
	// -proxy-lan, -gateway, -open, -firewall); they are applied and stored at
	// the start, like a change in the web interface.
	overrides []func(*settings.Settings)
}

// forwardFlags collects -forward PORT=HOST:PORT[=CALL,CALL].
type forwardFlags []forward.Rule

func (r *forwardFlags) String() string { return fmt.Sprint(*r) }

func (r *forwardFlags) Set(v string) error {
	rule, err := forward.ParseRule(v)
	if err != nil {
		return err
	}
	*r = append(*r, rule)
	return nil
}

// ruleFlags collects -open PORT/PROTO[=CALL,CALL].
type ruleFlags []firewall.Rule

func (r *ruleFlags) String() string { return fmt.Sprint(*r) }

func (r *ruleFlags) Set(v string) error {
	rule, err := firewall.ParseRule(v)
	if err != nil {
		return err
	}
	*r = append(*r, rule)
	return nil
}

func parseNodeFlags(args []string, stderr io.Writer) (nodeConfig, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	call := fs.String("call", "", "own callsign with SSID, e.g. N0CALL-1 (required; a placeholder is refused)")
	port := fs.String("port", "", `serial port of the radio or TNC, or "auto" to find the F4HWN radio`)
	kissTCP := fs.String("kiss-tcp", "", "host:port of a KISS-over-TCP TNC (Dire Wolf, net-sim)")
	kissUnix := fs.String("kiss-unix", "", "Unix socket of a KISS stream (the lab channel)")
	profile := fs.String("profile", "", "TNC profile: f4hwn-kisstnc (default for serial), f4hwn-kisstnc-ackmode, generic-kiss (default otherwise)")
	tunName := fs.String("tun", "axl0", "name of the network adapter")
	ip := fs.String("ip", "", "own IPv4 address with prefix length, e.g. 10.73.0.1/24 (required)")
	mtu := fs.Int("mtu", ipbridge.DefaultLinkMTU, "largest IP packet on the radio (the AX.25 information field)")
	adapterMTU := fs.Int("adapter-mtu", 0, "MTU of the network adapter; 0 = the radio MTU, raised to the system minimum (Windows: 576)")
	var routes routeFlags
	fs.Var(&routes, "route", "IP/PREFIX=CALLSIGN: send packets for these addresses to that station (repeatable); stations that send beacons are found without it")
	allowBroadcast := fs.Bool("allow-broadcast", false, "let broadcast and multicast packets onto the radio")
	tagList := fs.String("tags", "", "roles to announce, comma-separated: RELAY, INET, LINK, AUTO")
	beacon := fs.Duration("beacon", discovery.DefaultEvery, "interval of the station beacon; 0 sends none, stations are still learned")
	proxy := fs.Int("proxy", 0, "share this station's internet connection: an HTTP proxy on this port of the radio address (e.g. 8080; 0 stops sharing); adds the tag INET; stored")
	proxyLAN := fs.Bool("proxy-lan", false, "let the gateway also reach private, loopback and link-local addresses (the own network); stored")
	uiAddr := fs.String("ui", "127.0.0.1:7373", `address of the web interface; "" turns it off`)
	localProxy := fs.String("local-proxy", "127.0.0.1:7374", `address of the local proxy to the chosen internet gateway; "" turns it off`)
	gwChoice := fs.String("gateway", "off", "internet gateway to use: off, auto or a station's callsign; stored, the web interface can change it")
	configPath := fs.String("config", settings.DefaultPath(), `file for the settings; "" keeps them in memory only`)
	var open ruleFlags
	fs.Var(&open, "open", "PORT/tcp or PORT/udp, optionally =CALL,CALL: let radio stations reach this port of this computer (repeatable; stored)")
	fwMode := fs.String("firewall", "", "what radio stations may send to this computer: protect (the default) or open; stored")
	relay := fs.Bool("relay", true, "repeat frames for other stations, so that they reach stations beyond their range (mesh; on by default, -relay=false turns it off); announces the tag RELAY; stored")
	pcapPath := fs.String("pcap", "", "write every frame heard and sent into this file (pcap, link type AX.25: Wireshark opens it)")
	debug := fs.Bool("debug", false, "log every frame heard and sent, those for other stations too")
	var forwards forwardFlags
	fs.Var(&forwards, "forward", "PORT=HOST:PORT, optionally =CALL,CALL: publish a TCP service of the own network on this port of the radio address (repeatable; stored)")
	if err := fs.Parse(args); err != nil {
		return nodeConfig{}, err
	}

	var cfg nodeConfig
	if *call == "" {
		return cfg, errors.New("-call is required")
	}
	c, err := ax25.ParseAddress(*call)
	if err != nil {
		return cfg, fmt.Errorf("-call: %w", err)
	}
	if c.Call == "N0CALL" || c.Call == "NOCALL" {
		return cfg, errors.New("-call: that is a placeholder; set your own callsign")
	}
	cfg.call = c

	switch n := btoi(*port != "") + btoi(*kissTCP != "") + btoi(*kissUnix != ""); {
	case n > 1:
		return cfg, errors.New("use only one of -port, -kiss-tcp and -kiss-unix")
	case *kissTCP != "":
		cfg.dialer = tnc.TCP{Addr: *kissTCP}
	case *kissUnix != "":
		cfg.dialer = tnc.Unix{Path: *kissUnix}
	default:
		cfg.dialer = tnc.Serial{Path: *port}
	}
	name := *profile
	if name == "" {
		name = pacing.Generic.Name
		if _, serial := cfg.dialer.(tnc.Serial); serial {
			name = pacing.F4HWNKissTNC.Name
		}
	}
	p, ok := pacing.ByName(name)
	if !ok {
		return cfg, fmt.Errorf("-profile: unknown profile %q", name)
	}
	cfg.profile = p

	if *ip == "" {
		return cfg, errors.New("-ip is required")
	}
	local, err := netip.ParsePrefix(*ip)
	if err != nil || !local.Addr().Is4() {
		return cfg, fmt.Errorf("-ip: want an IPv4 address with prefix length, e.g. 10.73.0.1/24")
	}
	if *mtu < 68 || *mtu > 65535 {
		return cfg, fmt.Errorf("-mtu: %d is outside 68..65535", *mtu)
	}
	cfg.linkMTU = *mtu
	osMTU := *adapterMTU
	if osMTU == 0 {
		osMTU = *mtu
	}
	cfg.tun = tun.Config{Name: *tunName, MTU: osMTU, Address: local}
	for _, r := range routes {
		if !local.Masked().Contains(r.Prefix.Addr()) || r.Prefix.Bits() < local.Bits() {
			cfg.tun.Routes = append(cfg.tun.Routes, r.Prefix.Masked()) // outside the own subnet
		}
	}
	cfg.routes = routes
	cfg.allowBroadcast = *allowBroadcast
	if cfg.tags, err = discovery.ParseTags(*tagList); err != nil {
		return cfg, fmt.Errorf("-tags: %w", err)
	}
	cfg.beacon = *beacon
	if err := cfg.discovery().Check(); err != nil {
		return cfg, fmt.Errorf("-beacon/-tags: %w", err)
	}
	for name, addr := range map[string]string{"-ui": *uiAddr, "-local-proxy": *localProxy} {
		if _, _, err := net.SplitHostPort(addr); addr != "" && err != nil {
			return cfg, fmt.Errorf("%s: want host:port, e.g. 127.0.0.1:7373", name)
		}
	}
	cfg.ui, cfg.localProxy, cfg.configPath = *uiAddr, *localProxy, *configPath

	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if given["proxy"] {
		if *proxy < 0 || *proxy > 65535 {
			return cfg, fmt.Errorf("-proxy: %d is not a port", *proxy)
		}
		port := uint16(*proxy)
		cfg.overrides = append(cfg.overrides, func(s *settings.Settings) {
			s.Share.Enabled = port != 0
			if port != 0 {
				s.Share.Port = port
			}
		})
	}
	if given["proxy-lan"] {
		lan := *proxyLAN
		cfg.overrides = append(cfg.overrides, func(s *settings.Settings) { s.Share.LAN = lan })
	}
	if given["gateway"] {
		choice := settings.Gateway{Mode: strings.ToLower(*gwChoice)}
		if choice.Mode != "off" && choice.Mode != "auto" {
			c, err := ax25.ParseAddress(*gwChoice)
			if err != nil {
				return cfg, fmt.Errorf("-gateway: off, auto or a callsign: %w", err)
			}
			choice = settings.Gateway{Mode: "station", Call: c.String()}
		}
		cfg.overrides = append(cfg.overrides, func(s *settings.Settings) { s.Gateway = choice })
	}
	if len(open) > 0 {
		rules := []firewall.Rule(open)
		cfg.overrides = append(cfg.overrides, func(s *settings.Settings) {
			for _, r := range rules {
				s.Firewall.Rules = slices.DeleteFunc(s.Firewall.Rules, func(x firewall.Rule) bool {
					return x.Proto == r.Proto && x.Port == r.Port
				})
				s.Firewall.Rules = append(s.Firewall.Rules, r)
			}
		})
	}
	cfg.pcap, cfg.debug = *pcapPath, *debug
	if given["relay"] {
		on := *relay
		cfg.overrides = append(cfg.overrides, func(s *settings.Settings) { s.Relay = on })
	}
	if len(forwards) > 0 {
		rules := []forward.Rule(forwards)
		cfg.overrides = append(cfg.overrides, func(s *settings.Settings) {
			for _, r := range rules {
				s.Forwards = slices.DeleteFunc(s.Forwards, func(x forward.Rule) bool { return x.Port == r.Port })
				s.Forwards = append(s.Forwards, r)
			}
		})
	}
	if given["firewall"] {
		mode := strings.ToLower(*fwMode)
		if mode != firewall.ModeProtect && mode != firewall.ModeOpen {
			return cfg, fmt.Errorf("-firewall: protect or open")
		}
		cfg.overrides = append(cfg.overrides, func(s *settings.Settings) { s.Firewall.Mode = mode })
	}
	return cfg, nil
}

func (cfg nodeConfig) discovery() discovery.Config {
	return discovery.Config{Call: cfg.call, Addr: cfg.tun.Address, Tags: cfg.tags, Every: cfg.beacon}
}

// relayMeta tells the transmit queue about a frame this station repeats for
// others: an IP packet as if it were its own, anything else urgent (beacons,
// messages, acknowledgments).
func relayMeta(frame []byte) txqueue.Meta {
	f, err := ax25.Decode(frame)
	if err == nil && f.PID == ax25.PIDIPv4 {
		return ipbridge.Classify(f.Info)
	}
	return txqueue.Meta{Urgent: true}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func runNode(args []string, stderr io.Writer) int {
	cfg, err := parseNodeFlags(args, stderr)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stderr, "run:", err)
		}
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := settings.Open(cfg.configPath)
	if err != nil {
		fmt.Fprintln(stderr, "run:", err)
		return 1
	}
	if len(cfg.overrides) > 0 {
		if _, err := store.Update(func(s *settings.Settings) {
			for _, o := range cfg.overrides {
				o(s)
			}
		}); err != nil {
			fmt.Fprintln(stderr, "run:", err)
			return 2
		}
	}
	st := store.Get()
	if store.Path() != "" {
		log.Info("settings", "file", store.Path())
	}

	dev, err := tun.Open(cfg.tun)
	if err != nil {
		fmt.Fprintln(stderr, "run:", err)
		return 1
	}
	log.Info("network adapter up", "name", dev.Name(), "ip", cfg.tun.Address, "mtu", dev.MTU())
	if dev.MTU() > cfg.linkMTU {
		log.Info("adapter MTU above the radio MTU: TCP MSS is lowered and larger packets are fragmented",
			"adapter", dev.MTU(), "radio", cfg.linkMTU)
	}

	n := &node{cfg: cfg, ctx: ctx, log: log, started: time.Now(), dev: dev, store: store}
	n.fw = firewall.New(st.Firewall, n.systemRules)
	var bridge *ipbridge.Bridge
	var disc *discovery.Service
	var msgs *chat.Service
	ccfg := capture.Config{Call: cfg.call}
	if cfg.debug {
		ccfg.Logger = log
	}
	if cfg.pcap != "" {
		f, err := capture.OpenPcap(cfg.pcap)
		if err != nil {
			dev.Close()
			fmt.Fprintln(stderr, "run:", err)
			return 1
		}
		defer f.Close()
		ccfg.Pcap = f
		log.Info("writing every frame into a pcap file", "file", cfg.pcap)
	}
	if n.capture, err = capture.New(ccfg); err != nil {
		dev.Close()
		fmt.Fprintln(stderr, "run:", err)
		return 1
	}
	var port *tnc.Port
	n.relay = mesh.NewRelay(mesh.Config{Call: cfg.call,
		Send:      func(frame []byte) { port.SendMeta(frame, relayMeta(frame)) },
		Neighbors: func() []ax25.Address { return disc.Neighbors() }})
	port = tnc.NewPort(tnc.Config{Dialer: cfg.dialer, Profile: cfg.profile, Logger: log,
		OnSent: func(frame []byte) { n.capture.Add(frame, true) }},
		func(frame []byte) {
			n.capture.Add(frame, false)
			if src, ok := ax25.Source(frame); ok {
				msgs.Heard(src) // its waiting messages go out again
			}
			if !disc.Handle(frame) && !msgs.Handle(frame) {
				bridge.FromRadio(frame)
			}
			n.relay.Handle(frame) // after discovery: the neighbours are up to date
		})
	msgs, err = chat.New(chat.Config{Call: cfg.call, Path: chat.DefaultPath(store.Path()), Logger: log,
		Route: func(peer ax25.Address) []ax25.Address { p, _ := disc.PathTo(peer); return p },
		Flood: func() []ax25.Address { return disc.Flood() },
		Send: func(frame []byte, replace string) bool {
			return port.SendMeta(frame, txqueue.Meta{Urgent: true, Replace: replace})
		}})
	if err != nil {
		dev.Close()
		fmt.Fprintln(stderr, "run:", err)
		return 1
	}
	n.chat = msgs
	dcfg := cfg.discovery()
	if w := st.Welcome; w.LocatorInBeacon {
		dcfg.Locator = w.Locator
	}
	dcfg.Logger = log
	dcfg.Send = func(frame []byte) bool {
		return port.SendMeta(frame, txqueue.Meta{Urgent: true, Replace: "beacon"})
	}
	disc = discovery.New(dcfg)
	bridge, err = ipbridge.New(ipbridge.Config{MyCall: cfg.call, Local: cfg.tun.Address,
		Routes: cfg.routes, AllowBroadcast: cfg.allowBroadcast, LinkMTU: cfg.linkMTU, Learned: disc.Route,
		Filter: n.fw}, port.SendMeta, dev.WritePacket)
	if err != nil {
		dev.Close()
		fmt.Fprintln(stderr, "run:", err)
		return 1
	}
	n.port, n.bridge, n.disc = port, bridge, disc
	n.gw = gateway.New(gateway.Config{AllowLAN: st.Share.LAN, Only: st.Share.Only, Who: disc.Lookup, Logger: log})
	n.choice = gateway.NewChoice(n.candidates)
	_ = n.choice.Set(gateway.Mode(st.Gateway.Mode), st.Gateway.Call) // checked when the settings were read
	n.fwd = forward.New(forward.Config{Addr: cfg.tun.Address.Addr(), Who: disc.Lookup, Logger: log})
	n.relay.SetOn(st.Relay || slices.Contains(cfg.tags, discovery.TagRelay))
	n.applyShare(st.Share) // announces the tags, RELAY too
	n.applyWelcome(st.Welcome)
	n.applyForwards(st.Forwards)
	log.Info("firewall", "mode", st.Firewall.Mode, "ping", st.Firewall.Ping, "rules", len(st.Firewall.Rules))
	n.mon = monitor.New(2*time.Second, 300, n.totals)
	go n.mon.Run(ctx)
	startLocal(n)
	startUI(n)

	portDone := make(chan error, 1)
	go func() { portDone <- port.Run(ctx) }()
	go disc.Run(ctx)
	go msgs.Run(ctx)
	adapterDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := dev.ReadPacket(buf)
			if err != nil {
				adapterDone <- err
				return
			}
			bridge.FromAdapter(buf[:n])
		}
	}()

	logStats := func() {
		args := []any{"port", fmt.Sprintf("%+v", port.Stats()), "ip", fmt.Sprintf("%+v", bridge.Stats()),
			"beacons", fmt.Sprintf("%+v", disc.Stats()), "stations", disc.Summary(),
			"firewall", fmt.Sprintf("%+v", n.fw.Stats()), "chat", fmt.Sprintf("%+v", msgs.Stats()),
			"forward", fmt.Sprintf("%+v", n.fwd.Stats()), "relay", fmt.Sprintf("on=%v %+v", n.relay.On(), n.relay.Stats())}
		n.mu.Lock()
		sharing := n.gwAddr != ""
		n.mu.Unlock()
		if sharing {
			args = append(args, "gateway", fmt.Sprintf("%+v", n.gw.Stats()))
		}
		if n.local != nil {
			args = append(args, "local", fmt.Sprintf("%+v", n.local.Stats()))
		}
		log.Info("stats", args...)
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	code := 0
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-ticker.C:
			logStats()
		case err := <-adapterDone:
			log.Error("network adapter failed", "err", err)
			code = 1
			break loop
		}
	}
	stop()
	dev.Close()
	<-portDone
	logStats()
	return code
}

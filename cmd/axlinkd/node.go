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
	"github.com/TechnikWeber/AXLink/internal/discovery"
	"github.com/TechnikWeber/AXLink/internal/gateway"
	"github.com/TechnikWeber/AXLink/internal/ipbridge"
	"github.com/TechnikWeber/AXLink/internal/pacing"
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
	tags           []string
	beacon         time.Duration
	proxy          uint16 // port of the gateway's HTTP proxy; 0 for none
	proxyLAN       bool
	ui             string // address of the web interface; "" for none
	localProxy     string // address of the local proxy to the chosen gateway; "" for none
	gwMode         gateway.Mode
	gwCall         string
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
	proxy := fs.Int("proxy", 0, "share this station's internet connection: an HTTP proxy on this port of the radio address (e.g. 8080); adds the tag INET")
	proxyLAN := fs.Bool("proxy-lan", false, "let the proxy also reach private, loopback and link-local addresses (the own network)")
	uiAddr := fs.String("ui", "127.0.0.1:7373", `address of the web interface; "" turns it off`)
	localProxy := fs.String("local-proxy", "127.0.0.1:7374", `address of the local proxy to the chosen internet gateway; "" turns it off`)
	gwChoice := fs.String("gateway", "off", "internet gateway at the start: off, auto or a station's callsign; the web interface can change it")
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
	if *proxy < 0 || *proxy > 65535 {
		return cfg, fmt.Errorf("-proxy: %d is not a port", *proxy)
	}
	cfg.proxy, cfg.proxyLAN = uint16(*proxy), *proxyLAN
	if cfg.proxy != 0 && !slices.Contains(cfg.tags, discovery.TagInet) {
		cfg.tags = append(cfg.tags, discovery.TagInet)
	}
	if err := cfg.discovery().Check(); err != nil {
		return cfg, fmt.Errorf("-beacon/-tags: %w", err)
	}
	for name, addr := range map[string]string{"-ui": *uiAddr, "-local-proxy": *localProxy} {
		if _, _, err := net.SplitHostPort(addr); addr != "" && err != nil {
			return cfg, fmt.Errorf("%s: want host:port, e.g. 127.0.0.1:7373", name)
		}
	}
	cfg.ui, cfg.localProxy = *uiAddr, *localProxy
	switch strings.ToLower(*gwChoice) {
	case "off", "":
		cfg.gwMode = gateway.ModeOff
	case "auto":
		cfg.gwMode = gateway.ModeAuto
	default:
		c, err := ax25.ParseAddress(*gwChoice)
		if err != nil {
			return cfg, fmt.Errorf("-gateway: off, auto or a callsign: %w", err)
		}
		cfg.gwMode, cfg.gwCall = gateway.ModeStation, c.String()
	}
	return cfg, nil
}

func (cfg nodeConfig) discovery() discovery.Config {
	return discovery.Config{Call: cfg.call, Addr: cfg.tun.Address, Tags: cfg.tags, Every: cfg.beacon, Proxy: cfg.proxy}
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

	var bridge *ipbridge.Bridge
	var disc *discovery.Service
	port := tnc.NewPort(tnc.Config{Dialer: cfg.dialer, Profile: cfg.profile, Logger: log},
		func(frame []byte) {
			if !disc.Handle(frame) {
				bridge.FromRadio(frame)
			}
		})
	dcfg := cfg.discovery()
	dcfg.Logger = log
	dcfg.Send = func(frame []byte) bool {
		return port.SendMeta(frame, txqueue.Meta{Urgent: true, Replace: "beacon"})
	}
	disc = discovery.New(dcfg)
	bridge, err = ipbridge.New(ipbridge.Config{MyCall: cfg.call, Local: cfg.tun.Address,
		Routes: cfg.routes, AllowBroadcast: cfg.allowBroadcast, LinkMTU: cfg.linkMTU, Learned: disc.Lookup},
		port.SendMeta, dev.WritePacket)
	if err != nil {
		dev.Close()
		fmt.Fprintln(stderr, "run:", err)
		return 1
	}

	n := &node{cfg: cfg, started: time.Now(), dev: dev, port: port, bridge: bridge, disc: disc}
	if cfg.proxy != 0 {
		n.gwAddr = netip.AddrPortFrom(cfg.tun.Address.Addr(), cfg.proxy).String()
		ln, err := net.Listen("tcp", n.gwAddr)
		if err != nil {
			dev.Close()
			fmt.Fprintln(stderr, "run: gateway:", err)
			return 1
		}
		n.gw = gateway.New(gateway.Config{AllowLAN: cfg.proxyLAN, Who: disc.Lookup, Logger: log})
		go n.gw.Serve(ctx, ln)
		log.Info("gateway: HTTP proxy for the radio network", "listen", n.gwAddr, "lan", cfg.proxyLAN)
	}
	n.choice = gateway.NewChoice(n.candidates)
	_ = n.choice.Set(cfg.gwMode, cfg.gwCall) // checked when the flags were read
	startLocal(ctx, n, log)
	startUI(ctx, n, log)

	portDone := make(chan error, 1)
	go func() { portDone <- port.Run(ctx) }()
	go disc.Run(ctx)
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
			"beacons", fmt.Sprintf("%+v", disc.Stats()), "stations", disc.Summary()}
		if n.gw != nil {
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

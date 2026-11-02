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
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/TechnikWeber/AXLink/internal/sim/channel"
	"github.com/TechnikWeber/AXLink/internal/tnc"
)

func listPorts(stdout, stderr io.Writer) int {
	ports, err := tnc.ListPorts()
	if err != nil {
		fmt.Fprintln(stderr, "ports:", err)
		return 1
	}
	if len(ports) == 0 {
		fmt.Fprintln(stdout, "no serial ports found")
		return 0
	}
	for _, p := range ports {
		line := p.Name
		if p.USB {
			line += fmt.Sprintf("  USB %s:%s", p.VID, p.PID)
			if p.Product != "" {
				line += "  " + p.Product
			}
		}
		if p.IsF4HWN() {
			line += "  <- radio with F4HWN firmware"
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// stationFlags collects -station NAME=SOCKET.
type stationFlags [][2]string

func (s *stationFlags) String() string { return fmt.Sprint(*s) }

func (s *stationFlags) Set(v string) error {
	name, path, ok := strings.Cut(v, "=")
	if !ok || name == "" || path == "" {
		return errors.New("want NAME=SOCKET")
	}
	*s = append(*s, [2]string{name, path})
	return nil
}

func simChannel(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("sim-channel", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var stations stationFlags
	fs.Var(&stations, "station", "NAME=SOCKET: a station connects to this Unix socket (repeatable)")
	mode := fs.String("mode", "f4hwn", "TNC in front of each station: f4hwn (model of the F4HWN KISS app) or ideal")
	ackMode := fs.Bool("ackmode", false, "f4hwn mode: model the proposed ACKMODE extension")
	tracePath := fs.String("trace", "", "write transmissions and receptions as JSON lines to this file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(stations) < 2 {
		fmt.Fprintln(stderr, "sim-channel: at least two -station flags are needed")
		return 2
	}
	cfg := channel.Config{AckMode: *ackMode, Logger: slog.New(slog.NewTextHandler(stderr, nil))}
	switch *mode {
	case "f4hwn":
		cfg.Mode = channel.F4HWN
	case "ideal":
		cfg.Mode = channel.Ideal
	default:
		fmt.Fprintln(stderr, "sim-channel: -mode must be f4hwn or ideal")
		return 2
	}
	if *tracePath != "" {
		f, err := os.Create(*tracePath)
		if err != nil {
			fmt.Fprintln(stderr, "sim-channel:", err)
			return 1
		}
		defer f.Close()
		cfg.Trace = f
	}
	c := channel.New(cfg)
	for _, st := range stations {
		os.Remove(st[1]) // a socket left over from an earlier run
		ln, err := net.Listen("unix", st[1])
		if err != nil {
			fmt.Fprintln(stderr, "sim-channel:", err)
			return 1
		}
		defer os.Remove(st[1])
		c.AddStation(st[0], ln)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg.Logger.Info("channel ready", "mode", *mode, "ackmode", *ackMode, "stations", len(stations))
	c.Run(ctx)
	return 0
}

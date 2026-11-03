// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin

package tun

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"

	wgtun "golang.zx2c4.com/wireguard/tun"
)

const minMTU = 68

// open creates a utun adapter; macOS chooses its name (utun4, utun5, ...),
// so cfg.Name is not used. It needs root.
func open(cfg Config) (Device, error) {
	dev, err := wgtun.CreateTUN("utun", cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("tun: create utun: %w (needs root: start axlinkd with sudo)", err)
	}
	name, err := dev.Name()
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("tun: %w", err)
	}
	if err := configure(name, cfg); err != nil {
		dev.Close()
		return nil, err
	}
	return newWGDevice(dev, name, cfg.MTU), nil
}

// configure sets the address, the MTU and the routes with ifconfig and route,
// as wg-quick does on macOS: utun is a point-to-point adapter, so the subnet
// gets a route of its own.
func configure(name string, cfg Config) error {
	cmds := [][]string{
		{"ifconfig", name, "inet", cfg.Address.String(), cfg.Address.Addr().String(), "alias"},
		{"ifconfig", name, "mtu", strconv.Itoa(cfg.MTU), "up"},
	}
	for _, r := range append([]netip.Prefix{cfg.Address.Masked()}, cfg.Routes...) {
		cmds = append(cmds, []string{"route", "-q", "-n", "add", "-inet", r.String(), "-interface", name})
	}
	for _, args := range cmds {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil && !(args[0] == "route" && strings.Contains(string(out), "File exists")) {
			return fmt.Errorf("tun: %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package tun

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
	wgtun "golang.zx2c4.com/wireguard/tun"
)

const minMTU = 68

func open(cfg Config) (Device, error) {
	// Open the device ourselves without IFF_VNET_HDR: wireguard-go then
	// disables GSO offloads. With MTU 256 a 64 KB offload packet would split
	// into more segments than one read can return.
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("tun: open /dev/net/tun: %w", err)
	}
	ifr, err := unix.NewIfreq(cfg.Name)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("tun: create %s: %w (needs CAP_NET_ADMIN)", cfg.Name, err)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	dev, err := wgtun.CreateTUNFromFile(os.NewFile(uintptr(fd), "/dev/net/tun"), cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("tun: %w", err)
	}
	name := cfg.Name
	if real, err := dev.Name(); err == nil {
		name = real
	}
	if err := configure(name, cfg); err != nil {
		dev.Close()
		return nil, err
	}
	return newWGDevice(dev, name, cfg.MTU), nil
}

// configure sets the address, MTU and routes with iproute2 and turns IPv6
// off on the adapter, so the kernel sends no IPv6 chatter into the radio.
func configure(name string, cfg Config) error {
	disableIPv6 := "/proc/sys/net/ipv6/conf/" + name + "/disable_ipv6"
	if _, err := os.Stat(disableIPv6); err == nil {
		_ = os.WriteFile(disableIPv6, []byte("1"), 0o644) // best effort
	}
	cmds := [][]string{
		{"addr", "add", cfg.Address.String(), "dev", name},
		{"link", "set", "dev", name, "mtu", strconv.Itoa(cfg.MTU), "up"},
	}
	for _, r := range cfg.Routes {
		cmds = append(cmds, []string{"route", "replace", r.String(), "dev", name})
	}
	for _, args := range cmds {
		cmd := exec.Command("ip", args...)
		if os.Geteuid() != 0 {
			// Started with only CAP_NET_ADMIN (setcap): pass it on to ip.
			cmd.SysProcAttr = &syscall.SysProcAttr{AmbientCaps: []uintptr{unix.CAP_NET_ADMIN}}
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("tun: ip %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

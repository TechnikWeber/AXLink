// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package tun

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
	wgtun "golang.zx2c4.com/wireguard/tun"
)

// headroom in front of each packet buffer, as wireguard-go expects.
const headroom = 16

type linuxDevice struct {
	dev   wgtun.Device
	name  string
	rbufs [][]byte
	sizes []int

	wmu  sync.Mutex // WritePacket may be called from several goroutines
	wbuf []byte
}

func open(cfg Config) (Device, error) {
	name := cfg.Name
	if name == "" {
		name = "axl0"
	}
	// Open the device ourselves without IFF_VNET_HDR: wireguard-go then
	// disables GSO offloads. With MTU 256 a 64 KB offload packet would split
	// into more segments than one read can return.
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("tun: open /dev/net/tun: %w", err)
	}
	ifr, err := unix.NewIfreq(name)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("tun: create %s: %w (needs CAP_NET_ADMIN)", name, err)
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	dev, err := wgtun.CreateTUNFromFile(os.NewFile(uintptr(fd), "/dev/net/tun"), cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("tun: %w", err)
	}
	if real, err := dev.Name(); err == nil {
		name = real
	}
	d := &linuxDevice{dev: dev, name: name, sizes: make([]int, 1), wbuf: make([]byte, headroom+65535)}
	d.rbufs = [][]byte{make([]byte, headroom+65535)}
	if err := configure(name, cfg); err != nil {
		dev.Close()
		return nil, err
	}
	return d, nil
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

func (d *linuxDevice) Name() string { return d.name }

func (d *linuxDevice) ReadPacket(buf []byte) (int, error) {
	for {
		n, err := d.dev.Read(d.rbufs, d.sizes, headroom)
		if err != nil {
			return 0, err
		}
		if n == 0 || d.sizes[0] == 0 {
			continue
		}
		return copy(buf, d.rbufs[0][headroom:headroom+d.sizes[0]]), nil
	}
}

func (d *linuxDevice) WritePacket(pkt []byte) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	n := copy(d.wbuf[headroom:], pkt)
	_, err := d.dev.Write([][]byte{d.wbuf[:headroom+n]}, headroom)
	return err
}

func (d *linuxDevice) Close() error { return d.dev.Close() }

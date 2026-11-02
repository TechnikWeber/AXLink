// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package tun

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"golang.org/x/sys/windows"
	wgtun "golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// Windows does not accept an IPv4 MTU below 576 (WireGuard for Windows uses
// the same floor). AXLink keeps radio packets at 256 bytes itself.
const minMTU = 576

func open(cfg Config) (Device, error) {
	wgtun.WintunTunnelType = "AXLink"
	guid := adapterGUID(cfg.Name)
	dev, err := wgtun.CreateTUNWithRequestedGUID(cfg.Name, &guid, cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("tun: create %s: %w (needs administrator rights and wintun.dll next to axlinkd.exe; see scripts/fetch-wintun.ps1)", cfg.Name, err)
	}
	native, ok := dev.(interface{ LUID() uint64 })
	if !ok {
		dev.Close()
		return nil, errors.New("tun: the Wintun device has no LUID")
	}
	if err := configure(winipcfg.LUID(native.LUID()), cfg); err != nil {
		dev.Close()
		return nil, err
	}
	return newWGDevice(dev, cfg.Name, cfg.MTU), nil
}

// adapterGUID derives a stable GUID from the adapter name, so that Windows
// recognises the network again next time and keeps its network profile.
func adapterGUID(name string) windows.GUID {
	h := sha256.Sum256([]byte("AXLink network adapter: " + name))
	g := windows.GUID{
		Data1: binary.BigEndian.Uint32(h[0:4]),
		Data2: binary.BigEndian.Uint16(h[4:6]),
		Data3: binary.BigEndian.Uint16(h[6:8])&0x0FFF | 0x5000, // name-based, as RFC 4122 version 5
	}
	copy(g.Data4[:], h[8:16])
	g.Data4[0] = g.Data4[0]&0x3F | 0x80 // RFC 4122 variant
	return g
}

// configure sets the MTU, the IPv4 address and extra routes through the IP
// Helper API, as WireGuard for Windows does.
func configure(luid winipcfg.LUID, cfg Config) error {
	family := winipcfg.AddressFamily(windows.AF_INET)
	// The interface first: with duplicate address detection off, the address
	// is usable at once.
	ipif, err := ipInterface(luid, family)
	if err != nil {
		return fmt.Errorf("tun: read the IPv4 interface: %w", err)
	}
	ipif.NLMTU = uint32(cfg.MTU)
	ipif.DadTransmits = 0
	ipif.RouterDiscoveryBehavior = winipcfg.RouterDiscoveryDisabled
	if err := ipif.Set(); err != nil {
		return fmt.Errorf("tun: set MTU %d: %w", cfg.MTU, err)
	}
	if err := luid.SetIPAddressesForFamily(family, []netip.Prefix{cfg.Address}); err != nil {
		if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
			return fmt.Errorf("tun: address %s is already used by another adapter", cfg.Address)
		}
		return fmt.Errorf("tun: set address %s: %w", cfg.Address, err)
	}
	for _, r := range cfg.Routes {
		if err := luid.AddRoute(r.Masked(), netip.IPv4Unspecified(), 0); err != nil &&
			!errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
			return fmt.Errorf("tun: add route %s: %w", r, err)
		}
	}
	return waitForAddress(luid, cfg.Address.Addr())
}

// ipInterface returns the IPv4 settings of the adapter. Windows can add them
// a moment after the adapter itself.
func ipInterface(luid winipcfg.LUID, family winipcfg.AddressFamily) (*winipcfg.MibIPInterfaceRow, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		ipif, err := luid.IPInterface(family)
		if err == nil || !errors.Is(err, windows.ERROR_NOT_FOUND) || time.Now().After(deadline) {
			return ipif, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitForAddress waits until Windows treats the new address as usable, so
// that the first packets do not fail. After five seconds it carries on.
func waitForAddress(luid winipcfg.LUID, addr netip.Addr) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if row, err := luid.IPAddress(addr); err == nil {
			switch row.DadState {
			case winipcfg.DadStatePreferred:
				return nil
			case winipcfg.DadStateDuplicate:
				return fmt.Errorf("tun: address %s is already used on this network", addr)
			}
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package tun

import (
	"testing"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// The GUID depends only on the name, so Windows finds the same network again.
func TestAdapterGUIDIsStableAndWellFormed(t *testing.T) {
	a, b := adapterGUID("axl0"), adapterGUID("axl0")
	if a != b {
		t.Fatal("the same name gave two GUIDs")
	}
	if adapterGUID("axl1") == a {
		t.Fatal("two names gave the same GUID")
	}
	if a.Data3>>12 != 5 || a.Data4[0]>>6 != 2 {
		t.Fatalf("version %d, variant bits %b", a.Data3>>12, a.Data4[0]>>6)
	}
}

// Records whether Windows accepts an IPv4 MTU below 576 on the adapter. It
// does not fail either way: AXLink keeps radio packets small itself.
func TestWindowsMTUFloor(t *testing.T) {
	dev := openTestAdapter(t)
	native, ok := dev.(*wgDevice).dev.(interface{ LUID() uint64 })
	if !ok {
		t.Fatal("no LUID")
	}
	ipif, err := winipcfg.LUID(native.LUID()).IPInterface(winipcfg.AddressFamily(windows.AF_INET))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("adapter MTU now %d", ipif.NLMTU)
	ipif.NLMTU = 256
	if err := ipif.Set(); err != nil {
		t.Logf("Windows refuses MTU 256: %v", err)
		return
	}
	again, err := winipcfg.LUID(native.LUID()).IPInterface(winipcfg.AddressFamily(windows.AF_INET))
	if err == nil {
		t.Logf("Windows accepted MTU 256; it reads back as %d", again.NLMTU)
	}
}

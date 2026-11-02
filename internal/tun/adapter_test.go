// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || windows

package tun

import (
	"encoding/binary"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/TechnikWeber/AXLink/internal/ipbridge"
)

// openTestAdapter opens a real adapter. It needs CAP_NET_ADMIN (Linux, e.g.
// inside "unshare -Urn") or administrator rights and wintun.dll (Windows), so
// it only runs with AXLINK_TUN_TEST=1.
func openTestAdapter(t *testing.T) Device {
	t.Helper()
	if os.Getenv("AXLINK_TUN_TEST") != "1" {
		t.Skip("set AXLINK_TUN_TEST=1 to create a real network adapter")
	}
	dev, err := Open(Config{Name: "axltest", MTU: 256, Address: netip.MustParsePrefix("10.73.99.1/24")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dev.Close() })
	return dev
}

// The operating system pings 10.73.99.2 through the adapter; the test plays
// that station and answers. This checks both directions through the adapter.
func TestAdapterCarriesPing(t *testing.T) {
	dev := openTestAdapter(t)
	t.Logf("adapter %s, MTU %d", dev.Name(), dev.MTU())
	peer := netip.MustParseAddr("10.73.99.2")
	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := dev.ReadPacket(buf)
			if err != nil {
				return
			}
			if reply := echoReply(buf[:n], peer); reply != nil {
				if err := dev.WritePacket(reply); err != nil {
					return
				}
			}
		}
	}()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "3", "-w", "5000", peer.String())
	} else {
		cmd = exec.Command("ping", "-c", "3", "-W", "5", peer.String())
	}
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	// Windows ping can exit 0 on "destination unreachable"; a reply has a TTL.
	if err != nil || !strings.Contains(strings.ToUpper(string(out)), "TTL=") {
		t.Fatalf("no ping reply through the adapter: %v", err)
	}
}

// echoReply answers an ICMP echo request addressed to peer.
func echoReply(p []byte, peer netip.Addr) []byte {
	if len(p) < 28 || p[0]>>4 != 4 || p[9] != 1 {
		return nil
	}
	ihl := int(p[0]&0x0F) * 4
	if len(p) < ihl+8 || p[ihl] != 8 || netip.AddrFrom4([4]byte(p[16:20])) != peer {
		return nil
	}
	r := append([]byte(nil), p...)
	copy(r[12:16], p[16:20])
	copy(r[16:20], p[12:16])
	r[8] = 64
	r[10], r[11] = 0, 0
	binary.BigEndian.PutUint16(r[10:], ipbridge.Checksum(r[:ihl]))
	r[ihl] = 0 // echo reply
	r[ihl+2], r[ihl+3] = 0, 0
	binary.BigEndian.PutUint16(r[ihl+2:], ipbridge.Checksum(r[ihl:]))
	return r
}

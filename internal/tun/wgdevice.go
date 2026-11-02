// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || windows

package tun

import (
	"sync"

	wgtun "golang.zx2c4.com/wireguard/tun"
)

// headroom in front of each packet buffer, as wireguard-go expects.
const headroom = 16

// wgDevice turns a wireguard-go device, which works in batches with an
// offset, into one packet per call. Batches of one suffice at 1200 bit/s.
type wgDevice struct {
	dev   wgtun.Device
	name  string
	mtu   int
	rbufs [][]byte
	sizes []int

	wmu  sync.Mutex // WritePacket may be called from several goroutines
	wbuf []byte
}

func newWGDevice(dev wgtun.Device, name string, mtu int) *wgDevice {
	return &wgDevice{
		dev: dev, name: name, mtu: mtu,
		rbufs: [][]byte{make([]byte, headroom+65535)},
		sizes: make([]int, 1),
		wbuf:  make([]byte, headroom+65535),
	}
}

func (d *wgDevice) Name() string { return d.name }

func (d *wgDevice) MTU() int { return d.mtu }

func (d *wgDevice) ReadPacket(buf []byte) (int, error) {
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

func (d *wgDevice) WritePacket(pkt []byte) error {
	d.wmu.Lock()
	defer d.wmu.Unlock()
	n := copy(d.wbuf[headroom:], pkt)
	_, err := d.dev.Write([][]byte{d.wbuf[:headroom+n]}, headroom)
	return err
}

func (d *wgDevice) Close() error { return d.dev.Close() }

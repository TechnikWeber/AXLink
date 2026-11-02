// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin && !cgo

package tnc

import "go.bug.st/serial"

// ListPorts lists port names only: USB details on macOS need cgo (IOKit).
// Native macOS builds have cgo and use ports_enum.go.
func ListPorts() ([]PortInfo, error) {
	names, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	out := make([]PortInfo, 0, len(names))
	for _, n := range names {
		out = append(out, PortInfo{Name: n})
	}
	return out, nil
}

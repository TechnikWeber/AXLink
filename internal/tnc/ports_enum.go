// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !(darwin && !cgo)

package tnc

import (
	"strings"

	"go.bug.st/serial/enumerator"
)

// ListPorts lists the serial ports with their USB identity.
func ListPorts() ([]PortInfo, error) {
	list, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	out := make([]PortInfo, 0, len(list))
	for _, p := range list {
		out = append(out, PortInfo{
			Name: p.Name, USB: p.IsUSB,
			VID: strings.ToUpper(p.VID), PID: strings.ToUpper(p.PID),
			Product: p.Product, SerialNumber: p.SerialNumber,
		})
	}
	return out, nil
}

// SPDX-License-Identifier: GPL-3.0-or-later

package tnc

import (
	"errors"
	"fmt"
	"strings"
)

// USB identity of the F4HWN firmware's CDC port (App/usb/usbd_cdc_if.c).
const (
	F4HWNVendorID  = "36B7"
	F4HWNProductID = "FFFF"
)

// PortInfo describes one serial port.
type PortInfo struct {
	Name         string
	USB          bool
	VID, PID     string // upper-case hex, USB ports only
	Product      string
	SerialNumber string
}

// IsF4HWN reports whether the port is a radio with F4HWN firmware.
func (p PortInfo) IsF4HWN() bool {
	return p.USB && strings.EqualFold(p.VID, F4HWNVendorID) && strings.EqualFold(p.PID, F4HWNProductID)
}

// FindRadio returns the port of the one connected F4HWN radio.
func FindRadio() (string, error) {
	ports, err := ListPorts()
	if err != nil {
		return "", err
	}
	var found []string
	for _, p := range ports {
		if p.IsF4HWN() {
			found = append(found, p.Name)
		}
	}
	switch len(found) {
	case 0:
		return "", errors.New("no radio with F4HWN firmware (USB 36B7:FFFF) found; is it switched on and connected?")
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("several F4HWN radios found (%s); choose one with -port", strings.Join(found, ", "))
}

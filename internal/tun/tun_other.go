// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package tun

func open(Config) (Device, error) { return nil, ErrUnsupported }

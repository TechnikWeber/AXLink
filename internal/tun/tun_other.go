// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !windows && !darwin

package tun

const minMTU = 68

func open(Config) (Device, error) { return nil, ErrUnsupported }

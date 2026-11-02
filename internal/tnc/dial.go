// SPDX-License-Identifier: GPL-3.0-or-later

package tnc

import (
	"context"
	"fmt"
	"io"
	"net"

	"go.bug.st/serial"
)

// Dialer opens a byte stream to a KISS TNC.
type Dialer interface {
	Dial(ctx context.Context) (io.ReadWriteCloser, error)
	String() string
}

// Serial opens a serial port: USB-CDC radios, hardware TNCs.
type Serial struct {
	// Path is the port (e.g. /dev/ttyACM0, COM5). Empty or "auto" picks the
	// single F4HWN radio among the USB serial ports.
	Path string
	Baud int // ignored by USB-CDC radios; 0 means 115200
}

func (s Serial) String() string {
	if s.Path == "" {
		return "serial:auto"
	}
	return "serial:" + s.Path
}

// Dial opens the port with DTR set: the F4HWN KISS app only sends to a host
// that has the port open (DTR). The port is opened exclusively.
func (s Serial) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	path := s.Path
	if path == "" || path == "auto" {
		found, err := FindRadio()
		if err != nil {
			return nil, err
		}
		path = found
	}
	baud := s.Baud
	if baud == 0 {
		baud = 115200
	}
	port, err := serial.Open(path, &serial.Mode{BaudRate: baud, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := port.SetDTR(true); err != nil {
		port.Close()
		return nil, fmt.Errorf("set DTR on %s: %w", path, err)
	}
	return port, nil
}

// TCP connects to a KISS-over-TCP server (Dire Wolf, net-sim, a node).
type TCP struct{ Addr string }

func (t TCP) String() string { return "tcp:" + t.Addr }

func (t TCP) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", t.Addr)
}

// Unix connects to a KISS stream on a Unix socket (the AXLink lab channel).
type Unix struct{ Path string }

func (u Unix) String() string { return "unix:" + u.Path }

func (u Unix) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", u.Path)
}

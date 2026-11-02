// SPDX-License-Identifier: GPL-3.0-or-later

// Command axlinkd is the AXLink daemon. This early version has offline tools
// only; radio access and the network adapter follow.
package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/kiss"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "0.2.0-dev"

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, "axlinkd", version)
		return 0
	case "decode":
		return decode(args[1:], stdin, stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	}
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprint(w, `usage: axlinkd <command>

commands:
  version   print the version
  decode    read a KISS byte stream on stdin and print one JSON line per frame
            -hex   the input is hex text instead of raw bytes
`)
}

type frameJSON struct {
	Port    uint8    `json:"port"`
	Command uint8    `json:"command"`
	Dest    string   `json:"dest,omitempty"`
	Src     string   `json:"src,omitempty"`
	Path    []string `json:"path,omitempty"` // "*" marks a repeater that has repeated
	Control *uint8   `json:"control,omitempty"`
	PID     *uint8   `json:"pid,omitempty"`
	Info    string   `json:"info,omitempty"`
	Payload string   `json:"payload,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func decode(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	hexInput := fs.Bool("hex", false, "the input is hex text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	enc := json.NewEncoder(stdout)
	d := &kiss.Decoder{}
	emit := func(f kiss.Frame) { _ = enc.Encode(describe(f)) }
	if *hexInput {
		text, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "decode:", err)
			return 1
		}
		data, err := hex.DecodeString(strings.Join(strings.Fields(string(text)), ""))
		if err != nil {
			fmt.Fprintln(stderr, "decode: bad hex:", err)
			return 1
		}
		d.Feed(data, emit)
	} else {
		// Stream, so that a live serial port can be piped in.
		buf := make([]byte, 4096)
		for {
			n, err := stdin.Read(buf)
			d.Feed(buf[:n], emit)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				fmt.Fprintln(stderr, "decode:", err)
				return 1
			}
		}
	}
	if n := d.Dropped(); n > 0 {
		fmt.Fprintf(stderr, "decode: %d invalid KISS frames dropped\n", n)
	}
	return 0
}

func describe(f kiss.Frame) frameJSON {
	out := frameJSON{Port: f.Port, Command: uint8(f.Command)}
	if f.Command != kiss.CmdData {
		out.Payload = hex.EncodeToString(f.Payload)
		return out
	}
	fr, err := ax25.Decode(f.Payload)
	if err != nil {
		out.Error = err.Error()
		out.Payload = hex.EncodeToString(f.Payload)
		return out
	}
	out.Dest, out.Src = fr.Dest.String(), fr.Src.String()
	for _, r := range fr.Path {
		s := r.String()
		if r.Bit7 {
			s += "*"
		}
		out.Path = append(out.Path, s)
	}
	c := fr.Control
	out.Control = &c
	if ax25.HasPID(fr.Control) {
		p := fr.PID
		out.PID = &p
	}
	out.Info = hex.EncodeToString(fr.Info)
	return out
}

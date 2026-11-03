// SPDX-License-Identifier: GPL-3.0-or-later

// Package capture keeps the frames heard and sent for the packet monitor of
// the web interface, all of them: also those for other stations, as with an
// open squelch. Each one comes with a short description (an IP packet, a
// beacon, an APRS message). The TNC already drops frames with a bad
// checksum, so those never arrive. Optionally every frame also goes into a
// pcap file that Wireshark opens (link type AX.25), and into the log.
package capture

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/TechnikWeber/AXLink/internal/ax25"
	"github.com/TechnikWeber/AXLink/internal/chat"
)

// Keep is how many frames the ring holds.
const Keep = 500

// The kinds of frames.
const (
	KindIP      = "ip"
	KindBeacon  = "beacon"
	KindMessage = "message" // APRS message, acknowledgment included
	KindAPRS    = "aprs"    // other APRS data (positions, status, ...)
	KindOther   = "other"
)

// Entry is one frame.
type Entry struct {
	N    uint64    `json:"n"`
	At   time.Time `json:"at"`
	Out  bool      `json:"out,omitempty"` // sent by this station
	From string    `json:"from"`
	To   string    `json:"to"`
	Via  []string  `json:"via,omitempty"` // digipeaters, "*" for used
	Kind string    `json:"kind"`
	Text string    `json:"text"`
	Len  int       `json:"len"`  // frame bytes without FCS
	Mine bool      `json:"mine"` // from this station or for it
}

// Config sets up a Capture.
type Config struct {
	Call   ax25.Address
	Pcap   io.Writer    // nil: no pcap file
	Logger *slog.Logger // nil: frames are not logged
	Now    func() time.Time
}

// Capture is safe for concurrent use.
type Capture struct {
	cfg Config

	mu      sync.Mutex
	ring    []Entry
	next    uint64
	pcapErr bool
}

// New returns a capture; with cfg.Pcap set it writes the pcap file header.
func New(cfg Config) (*Capture, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Call.Bit7 = false
	c := &Capture{cfg: cfg, next: 1}
	if cfg.Pcap != nil {
		var h [24]byte
		binary.LittleEndian.PutUint32(h[0:], 0xa1b2c3d4) // microseconds
		binary.LittleEndian.PutUint16(h[4:], 2)
		binary.LittleEndian.PutUint16(h[6:], 4)
		binary.LittleEndian.PutUint32(h[16:], 65535) // snapshot length
		binary.LittleEndian.PutUint32(h[20:], 3)     // LINKTYPE_AX25: the frame, no flags, no FCS
		if _, err := cfg.Pcap.Write(h[:]); err != nil {
			return nil, fmt.Errorf("capture: %w", err)
		}
	}
	return c, nil
}

// OpenPcap creates a pcap file for New.
func OpenPcap(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}
	return f, nil
}

// Add records a frame heard (out false) or sent.
func (c *Capture) Add(frame []byte, out bool) {
	now := c.cfg.Now()
	e := c.describe(frame)
	e.At, e.Out, e.Len = now, out, len(frame)
	c.mu.Lock()
	e.N = c.next
	c.next++
	if len(c.ring) < Keep {
		c.ring = append(c.ring, e)
	} else {
		c.ring[int(e.N-1)%Keep] = e
	}
	if c.cfg.Pcap != nil && !c.pcapErr {
		var h [16]byte
		binary.LittleEndian.PutUint32(h[0:], uint32(now.Unix()))
		binary.LittleEndian.PutUint32(h[4:], uint32(now.Nanosecond()/1000))
		binary.LittleEndian.PutUint32(h[8:], uint32(len(frame)))
		binary.LittleEndian.PutUint32(h[12:], uint32(len(frame)))
		_, err := c.cfg.Pcap.Write(append(h[:], frame...))
		if err != nil {
			c.pcapErr = true // reported once, below
		}
		if err != nil && c.cfg.Logger != nil {
			c.cfg.Logger.Error("pcap file not written any more", "err", err)
		}
	}
	c.mu.Unlock()
	if c.cfg.Logger != nil {
		dir := "in"
		if out {
			dir = "out"
		}
		c.cfg.Logger.Info("frame", "dir", dir, "from", e.From, "to", e.To, "via", strings.Join(e.Via, ","),
			"kind", e.Kind, "len", e.Len, "text", e.Text)
	}
}

// Since returns the frames after number n, oldest first.
func (c *Capture) Since(n uint64) []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []Entry{}
	first := uint64(1)
	if c.next > Keep {
		first = c.next - Keep
	}
	for i := max(n+1, first); i < c.next; i++ {
		out = append(out, c.ring[int(i-1)%Keep])
	}
	return out
}

// describe names the parts of a frame and summarises its content.
func (c *Capture) describe(frame []byte) Entry {
	f, err := ax25.Decode(frame)
	if err != nil {
		return Entry{Kind: KindOther, Text: fmt.Sprintf("not decodable (%v)", err)}
	}
	src, dst := f.Src, f.Dest
	src.Bit7, dst.Bit7 = false, false
	e := Entry{From: src.String(), To: dst.String(),
		Mine: src.SameStation(c.cfg.Call) || dst.SameStation(c.cfg.Call)}
	for _, d := range f.Path {
		s := d.String()
		if d.Bit7 {
			s += "*"
		}
		e.Via = append(e.Via, s)
	}
	switch {
	case !f.IsUI():
		e.Kind, e.Text = KindOther, fmt.Sprintf("control 0x%02X", f.Control)
	case f.PID == ax25.PIDIPv4:
		e.Kind, e.Text = KindIP, describeIP(f.Info)
	case f.PID == ax25.PIDNoL3 && strings.HasPrefix(string(f.Info), "AXL"):
		e.Kind, e.Text = KindBeacon, printable(f.Info)
	case f.PID == ax25.PIDNoL3:
		if m, err := chat.Parse(f.Info); err == nil {
			e.Kind = KindMessage
			if a, err := ax25.ParseAddress(m.To); err == nil && a.SameStation(c.cfg.Call) {
				e.Mine = true
			}
			switch m.Kind {
			case chat.KindAck:
				e.Text = "ack " + m.ID + " to " + m.To
			case chat.KindRej:
				e.Text = "rej " + m.ID + " to " + m.To
			default:
				e.Text = "to " + m.To + ": " + m.Text
				if m.ID != "" {
					e.Text += " {" + m.ID + "}"
				}
			}
		} else {
			e.Kind, e.Text = KindAPRS, printable(f.Info)
		}
	default:
		e.Kind, e.Text = KindOther, fmt.Sprintf("PID 0x%02X, %d bytes", f.PID, len(f.Info))
	}
	return e
}

// describeIP summarises an IPv4 packet: addresses, protocol, ports, flags.
func describeIP(p []byte) string {
	if len(p) < 20 || p[0]>>4 != 4 {
		return fmt.Sprintf("not IPv4, %d bytes", len(p))
	}
	ihl := int(p[0]&0x0F) * 4
	total := int(binary.BigEndian.Uint16(p[2:]))
	src, dst := netip.AddrFrom4([4]byte(p[12:16])), netip.AddrFrom4([4]byte(p[16:20]))
	s := fmt.Sprintf("%s → %s ", src, dst)
	frag := binary.BigEndian.Uint16(p[6:])
	if ihl < 20 || total < ihl || total > len(p) {
		return s + "(broken header)"
	}
	if frag&0x1FFF != 0 {
		return s + fmt.Sprintf("fragment, %d bytes", total-ihl)
	}
	l4 := p[ihl:total]
	switch p[9] {
	case 6:
		if len(l4) < 20 {
			return s + "TCP (short)"
		}
		off := int(l4[12]>>4) * 4
		data := max(len(l4)-off, 0)
		var flags []string
		for i, name := range []string{"FIN", "SYN", "RST", "PSH", "ACK"} {
			if l4[13]&(1<<i) != 0 {
				flags = append(flags, name)
			}
		}
		return s + fmt.Sprintf("TCP %d → %d %s, %d B", binary.BigEndian.Uint16(l4[0:]), binary.BigEndian.Uint16(l4[2:]),
			strings.Join(flags, " "), data)
	case 17:
		if len(l4) < 8 {
			return s + "UDP (short)"
		}
		return s + fmt.Sprintf("UDP %d → %d, %d B", binary.BigEndian.Uint16(l4[0:]), binary.BigEndian.Uint16(l4[2:]), len(l4)-8)
	case 1:
		if len(l4) < 2 {
			return s + "ICMP (short)"
		}
		names := map[byte]string{0: "echo reply", 3: "unreachable", 8: "echo request", 11: "time exceeded"}
		name := names[l4[0]]
		if name == "" {
			name = fmt.Sprintf("type %d", l4[0])
		}
		if l4[0] == 3 {
			name += fmt.Sprintf(" (code %d)", l4[1])
		}
		return s + "ICMP " + name
	}
	return s + fmt.Sprintf("protocol %d, %d B", p[9], len(l4))
}

// printable shows a text field as printable characters, at most 120.
func printable(b []byte) string {
	var sb strings.Builder
	for _, r := range string(b) {
		if sb.Len() >= 120 {
			sb.WriteString("…")
			break
		}
		if unicode.IsPrint(r) {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('·')
		}
	}
	return sb.String()
}

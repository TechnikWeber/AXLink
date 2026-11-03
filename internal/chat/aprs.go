// SPDX-License-Identifier: GPL-3.0-or-later

// Package chat carries text messages between stations in the APRS message
// format, directly in AX.25 UI frames: no IP, no TCP, no HTTP. One message is
// one frame, its acknowledgment another, so a message costs about a second
// of airtime. Radios that speak APRS messages, such as the APRS messenger
// firmware for the UV-K1/UV-K5 V3, can take part.
//
// A message to a station is acknowledged and repeated until it is: after 30,
// 60, 120 and 240 s, as the messenger does. If the station does not answer,
// the message waits in the mailbox and goes out again when the station is
// heard, for up to a week. Messages to CQ (also QST and ALL) are for every
// station around, are sent twice a minute apart and never acknowledged.
// Messages arrive whatever the firewall and the internet sharing say: they
// are not IP traffic.
package chat

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// Tocall is the destination of the frames AXLink sends: APZ for software
// still experimental, as the APRS messenger uses APZK5M. Receivers look at
// the information field, not at the destination.
var Tocall = ax25.Address{Call: "APZAXL"}

// Room is the addressee of messages for every station around.
const Room = "CQ"

// lobby are the addressees read as messages for everyone, as the messenger
// reads them.
var lobby = []string{"CQ", "QST", "ALL"}

// MaxText is the longest message text the APRS specification allows.
const MaxText = 67

const (
	addresseeLen = 9
	maxID        = 5
)

// Kind tells a text from an acknowledgment.
type Kind int

// The kinds of APRS messages.
const (
	KindText Kind = iota
	KindAck
	KindRej
)

// Msg is one APRS message as it is on the air.
type Msg struct {
	To   string // addressee, e.g. TESTB-1 or CQ
	Kind Kind
	Text string // KindText only
	// ID is the message number the sender wants acknowledged, or for
	// KindAck and KindRej the number acknowledged.
	ID string
	// ReplyAck is the number of one of our messages that a text in the
	// "{MM}AA" form acknowledges on the way ("" for none).
	ReplyAck string
}

var errFormat = errors.New("chat: not an APRS message")

// Parse reads an information field ":ADDRESSEE:text{ID".
func Parse(info []byte) (Msg, error) {
	if len(info) < 2+addresseeLen || info[0] != ':' || info[1+addresseeLen] != ':' {
		return Msg{}, errFormat
	}
	m := Msg{To: strings.ToUpper(strings.TrimRight(string(info[1:1+addresseeLen]), " "))}
	if m.To == "" || strings.ContainsAny(m.To, " :") {
		return Msg{}, errFormat
	}
	t := strings.TrimRight(string(info[2+addresseeLen:]), "\r\n ")
	for kind, word := range map[Kind]string{KindAck: "ack", KindRej: "rej"} {
		rest, ok := strings.CutPrefix(t, word)
		if !ok {
			continue
		}
		id, after, _ := strings.Cut(rest, "}") // "ackMM}AA" acknowledges MM
		if validID(id) && (after == "" || validID(after) || strings.HasSuffix(rest, "}")) {
			m.Kind, m.ID = kind, id
			return m, nil
		}
	}
	m.Kind, m.Text = KindText, t
	if i := strings.LastIndexByte(t, '{'); i >= 0 {
		id, reply, hasReply := strings.Cut(t[i+1:], "}")
		if validID(id) && (!hasReply || reply == "" || validID(reply)) {
			m.Text, m.ID = t[:i], id
			if hasReply {
				m.ReplyAck = reply
			}
		}
	}
	return m, nil
}

func validID(s string) bool {
	if len(s) < 1 || len(s) > maxID {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			return false
		}
	}
	return true
}

// Info returns the information field.
func (m Msg) Info() ([]byte, error) {
	to := strings.ToUpper(m.To)
	if to == "" || len(to) > addresseeLen || strings.ContainsAny(to, " :") {
		return nil, errors.New("chat: addressee must be 1-9 characters")
	}
	var b strings.Builder
	b.WriteString(":" + to + strings.Repeat(" ", addresseeLen-len(to)) + ":")
	switch m.Kind {
	case KindAck, KindRej:
		if !validID(m.ID) {
			return nil, errors.New("chat: bad message number")
		}
		b.WriteString(map[Kind]string{KindAck: "ack", KindRej: "rej"}[m.Kind] + m.ID)
	default:
		if m.Text != Clean(m.Text) || len(m.Text) > MaxText {
			return nil, errors.New("chat: text must be at most 67 plain characters")
		}
		b.WriteString(m.Text)
		if m.ID != "" {
			if !validID(m.ID) || m.ReplyAck != "" && !validID(m.ReplyAck) {
				return nil, errors.New("chat: bad message number")
			}
			b.WriteString("{" + m.ID)
			if m.ReplyAck != "" {
				b.WriteString("}" + m.ReplyAck)
			}
		}
	}
	return []byte(b.String()), nil
}

// Frame returns the message as a UI frame (without FCS) from call.
func (m Msg) Frame(call ax25.Address) ([]byte, error) { return m.FrameVia(call, nil) }

// FrameVia returns the message as a UI frame from call over the relays in
// path.
func (m Msg) FrameVia(call ax25.Address, path []ax25.Address) ([]byte, error) {
	info, err := m.Info()
	if err != nil {
		return nil, err
	}
	return ax25.NewUI(Tocall, call, path, ax25.PIDNoL3, info).Encode()
}

// IsLobby reports whether an addressee means every station.
func IsLobby(to string) bool {
	for _, l := range lobby {
		if strings.EqualFold(to, l) {
			return true
		}
	}
	return false
}

var plain = map[rune]string{
	'ä': "ae", 'ö': "oe", 'ü': "ue", 'Ä': "Ae", 'Ö': "Oe", 'Ü': "Ue", 'ß': "ss",
	'à': "a", 'á': "a", 'â': "a", 'å': "a", 'ã': "a", 'æ': "ae", 'ç': "c",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
	'ñ': "n", 'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ø': "o", 'œ': "oe",
	'ù': "u", 'ú': "u", 'û': "u", 'ý': "y", 'ÿ': "y",
	'À': "A", 'Á': "A", 'Â': "A", 'Å': "A", 'Ã': "A", 'Æ': "Ae", 'Ç': "C",
	'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E", 'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I",
	'Ñ': "N", 'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ø': "O", 'Œ': "Oe",
	'Ù': "U", 'Ú': "U", 'Û': "U", 'Ý': "Y",
	'€': "EUR", '°': "deg", '–': "-", '—': "-", '‘': "'", '’': "'", '‚': ",", '“': "\"", '”': "\"", '„': "\"",
	'…': "...", ' ': " ",
}

// Clean makes a text fit the APRS message format: printable ASCII without
// "|", "~" and "{" (which ends the text), as the messenger writes it.
// German umlauts and other Latin letters become their usual ASCII spelling
// (ä: ae), anything else "?".
func Clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '|' || r == '~' || r == '{':
			b.WriteByte('-')
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case r >= 0x20 && r <= 0x7E:
			b.WriteRune(r)
		case plain[r] != "":
			b.WriteString(plain[r])
		case r == utf8.RuneError || r < 0x20 || r == 0x7F:
			// dropped
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// Split cuts a cleaned text into parts of at most MaxText characters, at a
// space where there is one in the last third of a part.
func Split(s string) []string {
	s = strings.TrimSpace(s)
	var parts []string
	for len(s) > MaxText {
		cut := strings.LastIndexByte(s[:MaxText+1], ' ')
		if cut < MaxText*2/3 {
			cut = MaxText
		}
		parts = append(parts, strings.TrimRight(s[:cut], " "))
		s = strings.TrimLeft(s[cut:], " ")
	}
	if s != "" {
		parts = append(parts, s)
	}
	return parts
}

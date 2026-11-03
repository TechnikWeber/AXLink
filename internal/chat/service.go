// SPDX-License-Identifier: GPL-3.0-or-later

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// Timing, as the APRS messenger does it; variables so that tests can change
// them.
var (
	// retryAfter[n] is the wait after transmission n+1 of a message; after
	// the last one, the time left for its acknowledgment.
	retryAfter = []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second, 240 * time.Second, 120 * time.Second}
	roomRepeat = time.Minute     // the second transmission of a message to CQ
	heardGap   = 2 * time.Minute // a waiting message goes out again at most this often
	keepFor    = 7 * 24 * time.Hour
	dupWindow  = 24 * time.Hour   // a message number seen again in this time is a repeat
	roomDup    = 10 * time.Minute // the same CQ text again in this time is a repeat
)

// The states of a message this station sends.
const (
	StateSending   = "sending"   // waiting for its acknowledgment, repeated
	StateWaiting   = "waiting"   // not acknowledged yet: in the mailbox, sent again when the station is heard
	StateDelivered = "delivered" // acknowledged
	StateFailed    = "failed"    // refused, or not delivered within a week
	StateSent      = "sent"      // to CQ: sent twice, no acknowledgment
)

// maxMessages are kept; older ones are dropped first.
const maxMessages = 2000

// Message is one message, sent or received.
type Message struct {
	N    uint64    `json:"n"`              // unique on this station
	Out  bool      `json:"out,omitempty"`  // sent by this station
	Peer string    `json:"peer"`           // the other station, or CQ for the room
	From string    `json:"from,omitempty"` // in the room: who wrote it
	Text string    `json:"text"`
	At   time.Time `json:"at"`
	ID   string    `json:"id,omitempty"` // APRS message number
	// For messages sent: state, transmissions so far, the next one.
	State string    `json:"state,omitempty"`
	Tries int       `json:"tries,omitempty"`
	Next  time.Time `json:"next,omitzero"`
	Last  time.Time `json:"last,omitzero"` // the last transmission
	Read  bool      `json:"read,omitempty"`
}

// Conversation is the summary of one peer's messages.
type Conversation struct {
	Peer   string    `json:"peer"`
	Last   string    `json:"last"` // the newest text
	At     time.Time `json:"at"`
	Unread int       `json:"unread"`
	Count  int       `json:"count"`
}

// Config sets up a Service.
type Config struct {
	Call ax25.Address
	// Send hands a frame to the radio; replace names the frame, so that a
	// repetition still in the transmit queue replaces the older copy.
	Send func(frame []byte, replace string) bool
	// Route gives the relays to send to a station over (station discovery);
	// Flood the path for messages to CQ. Both may be nil: no relays.
	Route func(peer ax25.Address) []ax25.Address
	Flood func() []ax25.Address
	// Path is the mailbox file; "" keeps the messages in memory only.
	Path   string
	Logger *slog.Logger
	Now    func() time.Time // nil means time.Now
}

// Stats counts frames.
type Stats struct {
	Sent, Received, Acks, Repeats uint64
}

// Service is the mailbox and the delivery of messages. Its methods may be
// called from any goroutine.
type Service struct {
	cfg  Config
	wake chan struct{}

	mu     sync.Mutex
	box    mailbox
	stats  Stats
	saveOK bool // the last write worked; a failure is logged once
}

type mailbox struct {
	NextN  uint64    `json:"next_n"`
	NextID int       `json:"next_id"` // 1-999, as the messenger counts
	Msgs   []Message `json:"messages"`
}

// New returns a service with the mailbox read from cfg.Path.
func New(cfg Config) (*Service, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Service{cfg: cfg, wake: make(chan struct{}, 1), box: mailbox{NextN: 1, NextID: 1}, saveOK: true}
	if cfg.Path == "" {
		return s, nil
	}
	data, err := os.ReadFile(cfg.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	if err := json.Unmarshal(data, &s.box); err != nil {
		return nil, fmt.Errorf("chat: %s: %w", cfg.Path, err)
	}
	if s.box.NextID < 1 || s.box.NextID > 999 {
		s.box.NextID = 1
	}
	for _, m := range s.box.Msgs {
		s.box.NextN = max(s.box.NextN, m.N+1)
	}
	return s, nil
}

// DefaultPath is messages.json next to the settings file.
func DefaultPath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), "messages.json")
}

// Run sends what is due until ctx ends.
func (s *Service) Run(ctx context.Context) error {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		next := s.tick(s.cfg.Now())
		wait := time.Hour
		if !next.IsZero() {
			wait = max(next.Sub(s.cfg.Now()), 10*time.Millisecond)
		}
		timer.Reset(wait)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.wake:
		case <-timer.C:
		}
	}
}

func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// tick sends the transmissions due at now and returns when the next one is.
func (s *Service) tick(now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	var next time.Time
	inFlight := map[string]bool{} // one message per station at a time, in order
	for i := range s.box.Msgs {
		m := &s.box.Msgs[i]
		if !m.Out || m.State != StateSending && m.State != StateWaiting {
			continue
		}
		if now.Sub(m.At) > keepFor && m.State == StateWaiting {
			m.State, m.Next = StateFailed, time.Time{}
			changed = true
			continue
		}
		if m.Peer == Room {
			if !now.Before(m.Next) {
				s.transmit(m, now)
				changed = true
				if m.Tries >= 2 {
					m.State, m.Next = StateSent, time.Time{}
				} else {
					m.Next = now.Add(roomRepeat)
				}
			}
			if m.State == StateSending {
				next = earliest(next, m.Next)
			}
			continue
		}
		if m.State == StateWaiting || inFlight[m.Peer] {
			if m.State == StateSending {
				inFlight[m.Peer] = true // waits behind an earlier one
			}
			continue
		}
		inFlight[m.Peer] = true
		if now.Before(m.Next) {
			next = earliest(next, m.Next)
			continue
		}
		changed = true
		if m.Tries >= len(retryAfter) { // the last wait is over: into the mailbox
			m.State, m.Next = StateWaiting, time.Time{}
			delete(inFlight, m.Peer) // the next message for that station may go
			s.cfg.Logger.Info("message not acknowledged, kept in the mailbox", "to", m.Peer, "id", m.ID)
			continue
		}
		s.transmit(m, now)
		m.Next = now.Add(retryAfter[m.Tries-1])
		next = earliest(next, m.Next)
	}
	if changed {
		s.saveLocked()
	}
	return next
}

func earliest(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// transmit sends one copy of m; the caller holds mu.
func (s *Service) transmit(m *Message, now time.Time) {
	msg := Msg{To: m.Peer, Text: m.Text, ID: m.ID}
	var path []ax25.Address
	if m.Peer == Room {
		if s.cfg.Flood != nil {
			path = s.cfg.Flood()
		}
	} else if a, err := ax25.ParseAddress(m.Peer); err == nil && s.cfg.Route != nil {
		path = s.cfg.Route(a)
	}
	frame, err := msg.FrameVia(s.cfg.Call, path)
	if err != nil {
		s.cfg.Logger.Error("message not built", "err", err)
		m.State = StateFailed
		return
	}
	if m.Tries > 0 {
		s.stats.Repeats++
	}
	m.Tries++
	m.Last = now
	s.stats.Sent++
	if !s.cfg.Send(frame, "chat:"+m.Peer+":"+strconv.FormatUint(m.N, 10)) {
		s.cfg.Logger.Warn("message dropped: transmit queue full", "to", m.Peer)
	}
}

// Send queues a text for a station, or for every station with to = CQ. A
// long text goes out in parts of at most 67 characters.
func (s *Service) Send(to, text string) ([]Message, error) {
	to = strings.ToUpper(strings.TrimSpace(to))
	if IsLobby(to) {
		to = Room
	} else {
		a, err := ax25.ParseAddress(to)
		if err != nil {
			return nil, fmt.Errorf("chat: recipient: %w", err)
		}
		if a.SameStation(s.cfg.Call) {
			return nil, errors.New("chat: that is this station")
		}
		to = a.String()
	}
	parts := Split(Clean(text))
	if len(parts) == 0 {
		return nil, errors.New("chat: no text")
	}
	if len(parts) > 5 {
		return nil, errors.New("chat: text too long (at most 5 parts of 67 characters)")
	}
	now := s.cfg.Now()
	s.mu.Lock()
	var out []Message
	for _, p := range parts {
		m := Message{N: s.box.NextN, Out: true, Peer: to, Text: p, At: now, State: StateSending, Next: now, Read: true}
		s.box.NextN++
		if to != Room {
			m.ID = s.newIDLocked(to)
		}
		s.box.Msgs = append(s.box.Msgs, m)
		out = append(out, m)
	}
	s.pruneLocked()
	s.saveLocked()
	s.mu.Unlock()
	s.poke()
	return out, nil
}

// newIDLocked returns the next message number not in use for peer.
func (s *Service) newIDLocked(peer string) string {
	for range 999 {
		id := strconv.Itoa(s.box.NextID)
		s.box.NextID = s.box.NextID%999 + 1
		if !slices.ContainsFunc(s.box.Msgs, func(m Message) bool {
			return m.Out && m.Peer == peer && m.ID == id && (m.State == StateSending || m.State == StateWaiting)
		}) {
			return id
		}
	}
	return strconv.Itoa(s.box.NextID)
}

// Retry sends a waiting or failed message again now.
func (s *Service) Retry(n uint64) error {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.box.Msgs {
		m := &s.box.Msgs[i]
		if m.N != n || !m.Out || m.Peer == Room {
			continue
		}
		if m.State != StateWaiting && m.State != StateFailed {
			return errors.New("chat: the message is not waiting")
		}
		m.State, m.Tries, m.Next = StateSending, 0, now
		if m.At.Before(now.Add(-keepFor)) {
			m.At = now // a week from now again
		}
		s.saveLocked()
		s.poke()
		return nil
	}
	return errors.New("chat: no such message")
}

// Heard is called for every frame heard from a station: its waiting
// messages go out again.
func (s *Service) Heard(call ax25.Address) {
	call.Bit7 = false
	peer := call.String()
	now := s.cfg.Now()
	s.mu.Lock()
	woke := false
	for i := range s.box.Msgs {
		m := &s.box.Msgs[i]
		if m.Out && m.State == StateWaiting && m.Peer == peer && now.Sub(m.Last) >= heardGap {
			// Two more transmissions (the last two waits of a round): a
			// station heard is not always one that hears us.
			m.State, m.Tries, m.Next = StateSending, len(retryAfter)-2, now
			woke = true
		}
	}
	if woke {
		s.cfg.Logger.Info("station heard again: sending its waiting messages", "to", peer)
		s.saveLocked()
	}
	s.mu.Unlock()
	if woke {
		s.poke()
	}
}

// Handle takes a frame heard on the radio and reports whether it was an
// APRS message (for this station or not); other frames are left to the
// caller.
func (s *Service) Handle(frame []byte) bool {
	f, err := ax25.Decode(frame)
	if err != nil || !f.IsUI() || f.PID != ax25.PIDNoL3 {
		return false
	}
	m, err := Parse(f.Info)
	if err != nil {
		return false
	}
	from := f.Src
	from.Bit7 = false
	if from.SameStation(s.cfg.Call) || f.InTransit() {
		return true // our own, repeated; or a relay has still to repeat it
	}
	back := ax25.Reverse(f.Repeaters()) // the way back, for the acknowledgment
	switch {
	case m.Kind != KindText:
		if s.isMe(m.To) {
			s.acknowledged(from, m.ID, m.Kind == KindRej)
		}
	case IsLobby(m.To):
		s.room(from, m.Text)
	case s.isMe(m.To):
		s.received(from, m, back)
	}
	return true
}

func (s *Service) isMe(to string) bool {
	a, err := ax25.ParseAddress(to)
	return err == nil && a.SameStation(s.cfg.Call)
}

// acknowledged marks our message id to from as delivered (or refused).
func (s *Service) acknowledged(from ax25.Address, id string, rej bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Acks++
	// The same station, or the same callsign with another SSID, as the
	// messenger accepts an acknowledgment.
	for _, exact := range []bool{true, false} {
		for i := len(s.box.Msgs) - 1; i >= 0; i-- {
			m := &s.box.Msgs[i]
			if !m.Out || m.ID != id || m.Peer == Room || (m.State != StateSending && m.State != StateWaiting) {
				continue
			}
			p, err := ax25.ParseAddress(m.Peer)
			if err != nil || p.Call != from.Call || (exact && p.SSID != from.SSID) {
				continue
			}
			m.State, m.Next = StateDelivered, time.Time{}
			if rej {
				m.State = StateFailed
			}
			s.saveLocked()
			s.poke() // the next message for that station may go
			return
		}
	}
}

// received stores a message for this station and acknowledges it, over
// path.
func (s *Service) received(from ax25.Address, m Msg, path []ax25.Address) {
	now := s.cfg.Now()
	peer := from.String()
	if m.ReplyAck != "" {
		s.acknowledged(from, m.ReplyAck, false)
	}
	s.mu.Lock()
	repeat := m.ID != "" && slices.ContainsFunc(s.box.Msgs, func(x Message) bool {
		return !x.Out && x.Peer == peer && x.ID == m.ID && x.Text == m.Text && now.Sub(x.At) < dupWindow
	})
	if !repeat {
		s.stats.Received++
		s.box.Msgs = append(s.box.Msgs, Message{N: s.box.NextN, Peer: peer, Text: m.Text, At: now, ID: m.ID})
		s.box.NextN++
		s.pruneLocked()
		s.saveLocked()
		s.cfg.Logger.Info("message received", "from", peer)
	}
	s.mu.Unlock()
	if m.ID == "" {
		return
	}
	// Our acknowledgment may have been lost: a repeat is acknowledged again.
	frame, err := Msg{To: peer, Kind: KindAck, ID: m.ID}.FrameVia(s.cfg.Call, path)
	if err == nil {
		s.cfg.Send(frame, "ack:"+peer+":"+m.ID)
	}
}

// room stores a message for every station.
func (s *Service) room(from ax25.Address, text string) {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	sender := from.String()
	if slices.ContainsFunc(s.box.Msgs, func(x Message) bool {
		return !x.Out && x.Peer == Room && x.From == sender && x.Text == text && now.Sub(x.At) < roomDup
	}) {
		return // the second copy
	}
	s.stats.Received++
	s.box.Msgs = append(s.box.Msgs, Message{N: s.box.NextN, Peer: Room, From: sender, Text: text, At: now})
	s.box.NextN++
	s.pruneLocked()
	s.saveLocked()
}

// Messages returns the messages of one conversation, oldest first.
func (s *Service) Messages(peer string) []Message {
	peer = strings.ToUpper(peer)
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Message
	for _, m := range s.box.Msgs {
		if m.Peer == peer {
			out = append(out, m)
		}
	}
	return out
}

// Conversations lists the peers, the room first, then the newest first.
func (s *Service) Conversations() []Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	by := map[string]*Conversation{Room: {Peer: Room}}
	for _, m := range s.box.Msgs {
		c := by[m.Peer]
		if c == nil {
			c = &Conversation{Peer: m.Peer}
			by[m.Peer] = c
		}
		c.Count++
		c.Last, c.At = m.Text, m.At
		if !m.Out && !m.Read {
			c.Unread++
		}
	}
	out := make([]Conversation, 0, len(by))
	for _, c := range by {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b Conversation) int {
		switch {
		case a.Peer == Room:
			return -1
		case b.Peer == Room:
			return 1
		}
		return b.At.Compare(a.At)
	})
	return out
}

// Unread counts the messages not yet read.
func (s *Service) Unread() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, m := range s.box.Msgs {
		if !m.Out && !m.Read {
			n++
		}
	}
	return n
}

// MarkRead marks a conversation as read.
func (s *Service) MarkRead(peer string) {
	peer = strings.ToUpper(peer)
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for i := range s.box.Msgs {
		if m := &s.box.Msgs[i]; m.Peer == peer && !m.Read {
			m.Read, changed = true, true
		}
	}
	if changed {
		s.saveLocked()
	}
}

// Delete removes a conversation.
func (s *Service) Delete(peer string) {
	peer = strings.ToUpper(peer)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.box.Msgs = slices.DeleteFunc(s.box.Msgs, func(m Message) bool { return m.Peer == peer })
	s.saveLocked()
}

// Stats returns a snapshot of the counters.
func (s *Service) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// Path is the mailbox file; "" for none.
func (s *Service) Path() string { return s.cfg.Path }

func (s *Service) pruneLocked() {
	if n := len(s.box.Msgs) - maxMessages; n > 0 {
		s.box.Msgs = append(s.box.Msgs[:0], s.box.Msgs[n:]...)
	}
}

// saveLocked writes the mailbox atomically, readable by the owner only.
func (s *Service) saveLocked() {
	if s.cfg.Path == "" {
		return
	}
	err := writeFile(s.cfg.Path, s.box)
	if err != nil && s.saveOK {
		s.cfg.Logger.Error("mailbox not written", "err", err)
	}
	s.saveOK = err == nil
}

func writeFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".messages-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

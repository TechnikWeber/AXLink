// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// Timing from the specification; variables so that tests can shorten them.
var (
	firstDelay  = [2]time.Duration{1 * time.Second, 5 * time.Second}
	answerDelay = [2]time.Duration{2 * time.Second, 8 * time.Second}
	answerGap   = 30 * time.Second
)

// Config sets up a Service.
type Config struct {
	Call  ax25.Address
	Addr  netip.Prefix // own address and subnet
	Tags  []string
	Proxy uint16 // port of the own HTTP proxy to the internet; 0 for none
	// Every is the interval of periodic beacons; 0 sends none, the service
	// then only listens.
	Every  time.Duration
	Send   func(frame []byte) bool // hands a beacon frame to the radio port
	Logger *slog.Logger
	Now    func() time.Time // nil means time.Now
	Rand   *rand.Rand       // nil means a random seed
}

// Station is what the service knows about another station.
type Station struct {
	Call  ax25.Address
	Addr  netip.Prefix
	Tags  []string
	Proxy uint16 // its HTTP proxy port; 0 for none
	Every time.Duration
	Heard time.Time // its last beacon
}

func (s *Station) expired(now time.Time) bool {
	return now.After(s.Heard.Add(3*s.Every + time.Minute))
}

// Stats counts beacons.
type Stats struct {
	Sent, Heard, Invalid, Relayed, Conflicts uint64
}

// Service sends this station's beacons and keeps the table of the others.
// Handle and Lookup may be called from any goroutine.
type Service struct {
	cfg  Config
	wake chan struct{}

	mu       sync.Mutex
	stations map[ax25.Address]*Station // by callsign and SSID
	warned   map[string]bool
	stats    Stats
	// schedule
	next     time.Time // next periodic beacon
	first    bool      // no beacon has gone out yet: send ask=1
	answerAt time.Time // a pending answer; zero for none
	lastSent time.Time
}

// New returns a service; start Run to send beacons.
func New(cfg Config) *Service {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	s := &Service{cfg: cfg, wake: make(chan struct{}, 1), stations: map[ax25.Address]*Station{},
		warned: map[string]bool{}, first: true}
	s.next = cfg.Now().Add(s.between(firstDelay))
	return s
}

// Check reports whether the configuration gives a valid beacon.
func (cfg Config) Check() error {
	if cfg.Every == 0 {
		return checkTags(cfg.Tags)
	}
	_, err := cfg.beacon(false).Marshal()
	return err
}

func (cfg Config) beacon(ask bool) Beacon {
	return Beacon{Addr: cfg.Addr, Tags: cfg.Tags, Every: cfg.Every, Proxy: cfg.Proxy, Ask: ask}
}

// between returns a random duration in [r[0], r[1]]; the caller holds mu
// or owns the service exclusively.
func (s *Service) between(r [2]time.Duration) time.Duration {
	return r[0] + time.Duration(s.cfg.Rand.Int64N(int64(r[1]-r[0])+1))
}

// Run sends beacons until ctx ends.
func (s *Service) Run(ctx context.Context) error {
	if s.cfg.Every <= 0 {
		<-ctx.Done()
		return ctx.Err()
	}
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		now := s.cfg.Now()
		send, ask, wake := s.due(now)
		if send {
			s.send(now, ask)
			continue
		}
		timer.Reset(wake.Sub(now))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.wake:
		case <-timer.C:
		}
	}
}

// due decides whether a beacon is due at now, and else when to look again.
func (s *Service) due(now time.Time) (send, ask bool, wake time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A pending answer: every beacon sent clears it, so none has gone out
	// since the beacon that asked for it.
	if !s.answerAt.IsZero() && !now.Before(s.answerAt) {
		if now.Sub(s.lastSent) < answerGap {
			s.answerAt = s.lastSent.Add(answerGap) // at most one answer in 30 s
		} else {
			return true, s.first, time.Time{}
		}
	}
	if !now.Before(s.next) {
		return true, s.first, time.Time{}
	}
	wake = s.next
	if !s.answerAt.IsZero() && s.answerAt.Before(wake) {
		wake = s.answerAt
	}
	return false, false, wake
}

func (s *Service) send(now time.Time, ask bool) {
	s.mu.Lock()
	frame, err := s.cfg.beacon(ask).Frame(s.cfg.Call)
	s.lastSent = now
	s.first = false
	s.answerAt = time.Time{} // this beacon answers everything heard before
	// The next periodic beacon, varied by ±10 %.
	s.next = now.Add(s.cfg.Every*9/10 + time.Duration(s.cfg.Rand.Int64N(int64(s.cfg.Every/5)+1)))
	if err == nil {
		s.stats.Sent++
	}
	s.mu.Unlock()
	if err != nil {
		s.cfg.Logger.Error("beacon not built", "err", err)
		return
	}
	if !s.cfg.Send(frame) {
		s.cfg.Logger.Warn("beacon dropped: transmit queue full")
	}
}

// SetAnnounce changes the tags and the proxy port this station announces;
// a beacon with them goes out within a few seconds.
func (s *Service) SetAnnounce(tags []string, proxy uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	next.Tags, next.Proxy = append([]string(nil), tags...), proxy
	if err := next.Check(); err != nil {
		return err
	}
	if slices.Equal(next.Tags, s.cfg.Tags) && next.Proxy == s.cfg.Proxy {
		return nil
	}
	s.cfg = next
	if s.cfg.Every > 0 {
		s.answerAt = s.cfg.Now().Add(s.between(answerDelay))
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return nil
}

// Announce returns the tags and the proxy port this station announces.
func (s *Service) Announce() ([]string, uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cfg.Tags...), s.cfg.Proxy
}

// Handle takes a frame heard on the radio. It reports whether the frame was
// a beacon; other frames are left to the caller.
func (s *Service) Handle(frame []byte) bool {
	f, err := ax25.Decode(frame)
	if err != nil {
		return false
	}
	call, b, ok, err := FromFrame(f)
	if !ok {
		return false
	}
	call.Bit7 = false // the same station as a command or a response
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case errors.Is(err, errRelayed):
		s.stats.Relayed++
		return true
	case err != nil:
		s.stats.Invalid++
		s.warnOnce("invalid:"+call.String(), "invalid beacon ignored", "call", call.String(), "err", err)
		return true
	case call.SameStation(s.cfg.Call):
		echo := b.Addr == s.cfg.Addr && b.Every == s.cfg.Every && b.Proxy == s.cfg.Proxy && slices.Equal(b.Tags, s.cfg.Tags)
		if !echo { // an echo of our own beacon (some TNCs return what they sent) is fine
			s.warnOnce("call", "another station sends beacons with our callsign", "call", call.String(), "ip", b.Addr.String())
		}
		return true
	}
	s.stats.Heard++
	if b.Addr.Addr() == s.cfg.Addr.Addr() {
		s.warnOnce("addr:"+call.String(), "a station announces our own address", "call", call.String(), "ip", b.Addr.String())
	}
	old, known := s.stations[call]
	if known && old.expired(now) {
		known = false
	}
	if !known || old.Addr != b.Addr || old.Proxy != b.Proxy || !slices.Equal(old.Tags, b.Tags) {
		args := []any{"call", call.String(), "ip", b.Addr.String(), "tags", strings.Join(b.Tags, ",")}
		if b.Proxy != 0 {
			args = append(args, "proxy", netip.AddrPortFrom(b.Addr.Addr(), b.Proxy).String())
		}
		s.cfg.Logger.Info("station heard", args...)
	}
	for _, other := range s.stations {
		if other.Call != call && other.Addr.Addr() == b.Addr.Addr() && !other.expired(now) {
			s.stats.Conflicts++
			s.cfg.Logger.Warn("two stations announce the same address; the newer one wins",
				"ip", b.Addr.Addr().String(), "newer", call.String(), "older", other.Call.String())
		}
	}
	s.stations[call] = &Station{Call: call, Addr: b.Addr, Tags: b.Tags, Proxy: b.Proxy, Every: b.Every, Heard: now}
	if (b.Ask || !known) && s.cfg.Every > 0 {
		if s.answerAt.IsZero() {
			s.answerAt = now.Add(s.between(answerDelay))
		}
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return true
}

func (s *Service) warnOnce(key, msg string, args ...any) {
	if !s.warned[key] {
		s.warned[key] = true
		s.cfg.Logger.Warn(msg, args...)
	}
}

// Lookup returns the station that announced addr; of two, the newer one.
func (s *Service) Lookup(addr netip.Addr) (ax25.Address, bool) {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var best *Station
	for _, st := range s.stations {
		if st.Addr.Addr() == addr && !st.expired(now) && (best == nil || st.Heard.After(best.Heard)) {
			best = st
		}
	}
	if best == nil {
		return ax25.Address{}, false
	}
	return best.Call, true
}

// Stations returns the stations heard and not yet forgotten, by callsign.
func (s *Service) Stations() []Station {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Station
	for call, st := range s.stations {
		if st.expired(now) {
			delete(s.stations, call)
			continue
		}
		out = append(out, *st)
	}
	slices.SortFunc(out, func(a, b Station) int { return strings.Compare(a.Call.String(), b.Call.String()) })
	return out
}

// Summary lists the stations in one line, e.g. "TESTB-1 10.73.0.2/24 INET".
func (s *Service) Summary() string {
	var parts []string
	for _, st := range s.Stations() {
		p := st.Call.String() + " " + st.Addr.String()
		if len(st.Tags) > 0 {
			p += " " + strings.Join(st.Tags, ",")
		}
		if st.Proxy != 0 {
			p += " proxy " + netip.AddrPortFrom(st.Addr.Addr(), st.Proxy).String()
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, "; ")
}

// Stats returns a snapshot of the counters.
func (s *Service) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

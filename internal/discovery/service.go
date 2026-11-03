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
	"github.com/TechnikWeber/AXLink/internal/mesh"
)

// Timing from the specification; variables so that tests can shorten them.
var (
	firstDelay  = [2]time.Duration{1 * time.Second, 5 * time.Second}
	answerDelay = [2]time.Duration{2 * time.Second, 8 * time.Second}
	answerGap   = 30 * time.Second
	// An ask nobody answered, while other stations are known, goes out again
	// after askAgain, maxAsks times in all: two stations that do not hear each
	// other may have asked at the same moment and collided at a relay.
	askAgain = [2]time.Duration{10 * time.Second, 20 * time.Second}
	maxAsks  = 3
	// A beacon sent with the flood alias comes back repeated by the relay;
	// without that echo after echoWait it goes out again, maxSends times in
	// all: it collided at the relay with a station we cannot hear.
	echoWait = [2]time.Duration{10 * time.Second, 20 * time.Second}
	maxSends = 3
)

// Config sets up a Service.
type Config struct {
	Call  ax25.Address
	Addr  netip.Prefix // own address and subnet
	Tags  []string
	Proxy uint16 // port of the own HTTP proxy to the internet; 0 for none
	// Locator is announced in the beacon when set (Maidenhead, e.g. JO62qm).
	Locator string
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
	Call    ax25.Address
	Addr    netip.Prefix
	Tags    []string
	Proxy   uint16 // its HTTP proxy port; 0 for none
	Locator string // its locator, if it announces one
	Every   time.Duration
	Heard   time.Time // its last beacon
	// Via are the relays to send to it over, in order; empty while its
	// beacons arrive directly.
	Via []ax25.Address

	directAt, viaAt time.Time // the last beacon directly, and over Via
}

func (s *Station) expired(now time.Time) bool {
	return now.After(s.Heard.Add(s.keep()))
}

func (s *Station) keep() time.Duration { return 3*s.Every + time.Minute }

// direct reports whether its beacons still arrive without a relay.
func (s *Station) direct(now time.Time) bool {
	return !s.directAt.IsZero() && !now.After(s.directAt.Add(s.keep()))
}

// path is the way to the station: none while it is heard directly.
func (s *Station) path(now time.Time) []ax25.Address {
	if s.direct(now) {
		return nil
	}
	return s.Via
}

// Stats counts beacons.
type Stats struct {
	Sent, Heard, Invalid, Conflicts uint64
	Relayed                         uint64 // beacons heard over relays
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
	askAt    time.Time // ask again then, if nothing was heard since the last ask; zero for not
	asks     int       // beacons with ask=1 sent
	echoAt   time.Time // send again then, if no relay has repeated the last beacon; zero for not
	echoAsk  bool      // that beacon asked
	sends    int       // copies of that beacon sent
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
	return Beacon{Addr: cfg.Addr, Tags: cfg.Tags, Every: cfg.Every, Proxy: cfg.Proxy, Locator: cfg.Locator, Ask: ask}
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
			// At most one answer in 30 s; the random part keeps two stations
			// whose beacons collided from answering in step again.
			s.answerAt = s.lastSent.Add(answerGap + s.between(answerDelay))
		} else {
			return true, s.first, time.Time{}
		}
	}
	if !now.Before(s.next) {
		return true, s.first, time.Time{}
	}
	if !s.askAt.IsZero() && !now.Before(s.askAt) {
		return true, true, time.Time{}
	}
	if !s.echoAt.IsZero() && !now.Before(s.echoAt) {
		return true, s.echoAsk, time.Time{}
	}
	wake = s.next
	if !s.answerAt.IsZero() && s.answerAt.Before(wake) {
		wake = s.answerAt
	}
	if !s.askAt.IsZero() && s.askAt.Before(wake) {
		wake = s.askAt
	}
	if !s.echoAt.IsZero() && s.echoAt.Before(wake) {
		wake = s.echoAt
	}
	return false, false, wake
}

func (s *Service) send(now time.Time, ask bool) {
	s.mu.Lock()
	path := s.floodLocked(now)
	frame, err := s.cfg.beacon(ask).FrameVia(s.cfg.Call, path)
	again := !s.echoAt.IsZero() && !now.Before(s.echoAt) // the last copy did not come back
	s.echoAt = time.Time{}
	if len(path) > 0 {
		if again {
			s.sends++
		} else {
			s.sends = 1
		}
		if s.sends < maxSends {
			s.echoAt, s.echoAsk = now.Add(s.between(echoWait)), ask
		}
	}
	s.lastSent = now
	s.first = false
	s.answerAt = time.Time{} // this beacon answers everything heard before
	s.askAt = time.Time{}
	if ask {
		s.asks++
		if s.asks < maxAsks && s.knownLocked(now) {
			s.askAt = now.Add(s.between(askAgain))
		}
	}
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

// SetLocator changes the locator this station announces ("" for none); a
// beacon with it goes out within a few seconds.
func (s *Service) SetLocator(loc string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	next.Locator = loc
	if err := next.Check(); err != nil {
		return err
	}
	if next.Locator == s.cfg.Locator {
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
	via := ax25.Reverse(f.Repeaters())
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case errors.Is(err, errInTransit):
		return true // the relay repeats it; we take it from there
	case err != nil:
		s.stats.Invalid++
		s.warnOnce("invalid:"+call.String(), "invalid beacon ignored", "call", call.String(), "err", err)
		return true
	case call.SameStation(s.cfg.Call):
		if len(via) > 0 { // a relay repeated our beacon: it reached the relay and goes on from there
			s.echoAt, s.askAt = time.Time{}, time.Time{}
		}
		echo := b.Addr == s.cfg.Addr && b.Every == s.cfg.Every && b.Proxy == s.cfg.Proxy && slices.Equal(b.Tags, s.cfg.Tags) &&
			b.Locator == s.cfg.Locator
		if !echo { // an echo of our own beacon (some TNCs return what they sent) is fine
			s.warnOnce("call", "another station sends beacons with our callsign", "call", call.String(), "ip", b.Addr.String())
		}
		return true
	}
	s.stats.Heard++
	s.askAt = time.Time{} // our ask, if any, has an answer, or at least the channel works
	if len(via) > 0 {
		s.stats.Relayed++
	}
	if b.Addr.Addr() == s.cfg.Addr.Addr() {
		s.warnOnce("addr:"+call.String(), "a station announces our own address", "call", call.String(), "ip", b.Addr.String())
	}
	old, known := s.stations[call]
	if known && old.expired(now) {
		known = false
	}
	st := &Station{Call: call, Addr: b.Addr, Tags: b.Tags, Proxy: b.Proxy, Locator: b.Locator, Every: b.Every, Heard: now}
	if known {
		st.directAt, st.viaAt, st.Via = old.directAt, old.viaAt, old.Via
	}
	if len(via) == 0 {
		st.directAt = now
	} else if st.Via == nil || len(via) <= len(st.Via) || now.After(st.viaAt.Add(st.Every+time.Minute)) {
		// a shorter way, the same again, or the only one while the shorter is silent
		st.Via, st.viaAt = via, now
	}
	newWay := !known || !slices.Equal(old.path(now), st.path(now))
	if !known || old.Addr != b.Addr || old.Proxy != b.Proxy || !slices.Equal(old.Tags, b.Tags) || newWay {
		args := []any{"call", call.String(), "ip", b.Addr.String(), "tags", strings.Join(b.Tags, ",")}
		if b.Proxy != 0 {
			args = append(args, "proxy", netip.AddrPortFrom(b.Addr.Addr(), b.Proxy).String())
		}
		if p := st.path(now); len(p) > 0 {
			args = append(args, "via", pathString(p))
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
	s.stations[call] = st
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
	call, _, ok := s.Route(addr)
	return call, ok
}

// Route returns the station that announced addr and the relays to send to
// it over (none when it is heard directly).
func (s *Service) Route(addr netip.Addr) (ax25.Address, []ax25.Address, bool) {
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
		return ax25.Address{}, nil, false
	}
	return best.Call, slices.Clone(best.path(now)), true
}

// PathTo returns the relays to send to a station over; known is false for a
// station not heard.
func (s *Service) PathTo(call ax25.Address) (path []ax25.Address, known bool) {
	call.Bit7 = false
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.stations[call]
	if !ok || st.expired(now) {
		return nil, false
	}
	return slices.Clone(st.path(now)), true
}

// Flood is the path for frames for every station (beacons, messages to CQ):
// the flood alias while another relay is heard directly, else none.
func (s *Service) Flood() []ax25.Address {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.floodLocked(now)
}

// knownLocked reports whether another station is known.
func (s *Service) knownLocked(now time.Time) bool {
	for _, st := range s.stations {
		if !st.expired(now) {
			return true
		}
	}
	return false
}

func (s *Service) floodLocked(now time.Time) []ax25.Address {
	for _, st := range s.stations {
		if st.direct(now) && slices.Contains(st.Tags, TagRelay) {
			return []ax25.Address{mesh.Flood}
		}
	}
	return nil
}

func pathString(p []ax25.Address) string {
	parts := make([]string, len(p))
	for i, a := range p {
		parts[i] = a.String()
	}
	return strings.Join(parts, ",")
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
		if len(st.Via) > 0 && !st.direct(s.cfg.Now()) {
			p += " via " + pathString(st.Via)
		}
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

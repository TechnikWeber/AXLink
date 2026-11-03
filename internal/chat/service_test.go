// SPDX-License-Identifier: GPL-3.0-or-later

package chat

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

type testChat struct {
	*Service
	t    *testing.T
	now  time.Time
	sent []Msg
	keys []string
	log  bytes.Buffer
}

func newTestChat(t *testing.T, call, path string) *testChat {
	t.Helper()
	tc := &testChat{t: t, now: time.Unix(1_800_000_000, 0)}
	s, err := New(Config{Call: ax25.MustAddress(call), Path: path,
		Send: func(frame []byte, key string) bool {
			f, err := ax25.Decode(frame)
			if err != nil || f.Dest.String() != "APZAXL" {
				t.Fatalf("%v %+v", err, f)
			}
			m, err := Parse(f.Info)
			if err != nil {
				t.Fatal(err)
			}
			tc.sent, tc.keys = append(tc.sent, m), append(tc.keys, key)
			return true
		},
		Logger: slog.New(slog.NewTextHandler(&tc.log, nil)),
		Now:    func() time.Time { return tc.now }})
	if err != nil {
		t.Fatal(err)
	}
	tc.Service = s
	return tc
}

// run advances the clock to t, sending whatever is due on the way.
func (tc *testChat) run(t time.Time) {
	for {
		next := tc.tick(tc.now)
		if next.IsZero() || next.After(t) {
			tc.now = t
			tc.tick(tc.now)
			return
		}
		tc.now = next
	}
}

func (tc *testChat) after(d time.Duration) time.Time { return tc.now.Add(d) }

// hear hands a message from call to the service.
func (tc *testChat) hear(call string, m Msg) {
	tc.t.Helper()
	frame, err := m.Frame(ax25.MustAddress(call))
	if err != nil {
		tc.t.Fatal(err)
	}
	if !tc.Handle(frame) {
		tc.t.Fatal("not taken as a message")
	}
}

func TestAMessageIsRepeatedUntilAcknowledged(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	out, err := tc.Send("testb-1", "Hello")
	if err != nil || len(out) != 1 || out[0].Peer != "TESTB-1" || out[0].ID == "" {
		t.Fatalf("%+v %v", out, err)
	}
	tc.run(tc.after(time.Second))
	if len(tc.sent) != 1 || tc.sent[0].To != "TESTB-1" || tc.sent[0].Text != "Hello" || tc.sent[0].ID != out[0].ID {
		t.Fatalf("%+v", tc.sent)
	}
	tc.run(tc.after(31 * time.Second))
	if len(tc.sent) != 2 {
		t.Fatalf("no repeat after 30 s: %d", len(tc.sent))
	}
	tc.hear("TESTB-1", Msg{To: "TESTA-1", Kind: KindAck, ID: out[0].ID})
	tc.run(tc.after(time.Hour))
	if len(tc.sent) != 2 || tc.Messages("TESTB-1")[0].State != StateDelivered {
		t.Fatalf("sent %d, %+v", len(tc.sent), tc.Messages("TESTB-1"))
	}
}

func TestUnansweredMessagesWaitInTheMailbox(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	tc.Send("TESTB-1", "first")
	tc.Send("TESTB-1", "second")
	start := tc.now
	// At 0, 30, 90, 210 and 450 s; the last wait of 120 s ends at 570 s.
	tc.run(start.Add(569 * time.Second))
	if len(tc.sent) != 5 || tc.sent[4].Text != "first" {
		t.Fatalf("one message at a time, five times: %+v", tc.sent)
	}
	tc.run(start.Add(571 * time.Second))
	msgs := tc.Messages("TESTB-1")
	if msgs[0].State != StateWaiting || msgs[1].State != StateSending || tc.sent[len(tc.sent)-1].Text != "second" {
		t.Fatalf("%+v; sent %+v", msgs, tc.sent)
	}
	tc.run(tc.after(20 * time.Minute))
	if n := len(tc.sent); n != 10 || tc.Messages("TESTB-1")[1].State != StateWaiting {
		t.Fatalf("%d sent, %+v", n, tc.Messages("TESTB-1"))
	}
	// The station is heard again: both go out again, two times each.
	tc.Heard(ax25.MustAddress("TESTB-1"))
	tc.run(tc.after(time.Second))
	if len(tc.sent) != 11 || tc.sent[10].Text != "first" {
		t.Fatalf("%+v", tc.sent[10:])
	}
	tc.hear("TESTB-1", Msg{To: "TESTA-1", Kind: KindAck, ID: tc.sent[10].ID})
	tc.run(tc.after(time.Second))
	if len(tc.sent) != 12 || tc.sent[11].Text != "second" {
		t.Fatalf("the second did not follow the first: %+v", tc.sent[10:])
	}
	tc.run(tc.after(time.Hour))
	if n := len(tc.sent); n != 13 || tc.Messages("TESTB-1")[1].State != StateWaiting {
		t.Fatalf("%d sent, %+v", n, tc.Messages("TESTB-1"))
	}
	// Heard again right after its last transmission: not yet.
	tc.Heard(ax25.MustAddress("TESTB-1"))
	if tc.Messages("TESTB-1")[1].State != StateSending {
		t.Fatal("not woken although the last try was long ago")
	}
}

func TestAWaitingMessageFailsAfterAWeek(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	out, _ := tc.Send("TESTB-1", "lost")
	tc.run(tc.after(keepFor + time.Hour))
	if m := tc.Messages("TESTB-1")[0]; m.State != StateFailed {
		t.Fatalf("%+v", m)
	}
	if err := tc.Retry(out[0].N); err != nil || tc.Messages("TESTB-1")[0].State != StateSending {
		t.Fatalf("retry: %v", err)
	}
	if tc.Retry(9999) == nil {
		t.Fatal("retried a message that does not exist")
	}
}

func TestARejectedMessageFails(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	out, _ := tc.Send("TESTB-1", "x")
	tc.run(tc.after(time.Second))
	tc.hear("TESTB-1", Msg{To: "TESTA-1", Kind: KindRej, ID: out[0].ID})
	if m := tc.Messages("TESTB-1")[0]; m.State != StateFailed {
		t.Fatalf("%+v", m)
	}
}

func TestAnAckFromAnotherSSIDCounts(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	out, _ := tc.Send("TESTB-7", "x")
	tc.run(tc.after(time.Second))
	tc.hear("TESTB", Msg{To: "TESTA-1", Kind: KindAck, ID: out[0].ID})
	if m := tc.Messages("TESTB-7")[0]; m.State != StateDelivered {
		t.Fatalf("%+v", m)
	}
	// An ack for another station is not ours.
	out, _ = tc.Send("TESTB-7", "y")
	tc.run(tc.after(time.Second))
	tc.hear("TESTB-7", Msg{To: "TESTZ-1", Kind: KindAck, ID: out[0].ID})
	if m := tc.Messages("TESTB-7")[1]; m.State != StateSending {
		t.Fatalf("%+v", m)
	}
}

func TestAReceivedMessageIsStoredOnceAndAcknowledgedEachTime(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	for range 2 {
		tc.hear("TESTB-1", Msg{To: "TESTA-1", Text: "Hi there", ID: "42"})
	}
	msgs := tc.Messages("TESTB-1")
	if len(msgs) != 1 || msgs[0].Text != "Hi there" || msgs[0].Out || msgs[0].Read {
		t.Fatalf("%+v", msgs)
	}
	if len(tc.sent) != 2 || tc.sent[0] != (Msg{To: "TESTB-1", Kind: KindAck, ID: "42"}) || tc.keys[0] != tc.keys[1] {
		t.Fatalf("%+v %v", tc.sent, tc.keys)
	}
	if tc.Unread() != 1 {
		t.Fatal("not unread")
	}
	tc.MarkRead("testb-1")
	if tc.Unread() != 0 {
		t.Fatal("still unread")
	}
	// Without a number: stored, not acknowledged; for another station: ignored.
	tc.hear("TESTB-1", Msg{To: "TESTA-1", Text: "no number"})
	tc.hear("TESTB-1", Msg{To: "TESTC-1", Text: "not for us", ID: "1"})
	if len(tc.Messages("TESTB-1")) != 2 || len(tc.sent) != 2 || len(tc.Messages("TESTC-1")) != 0 {
		t.Fatalf("%+v %+v", tc.Messages("TESTB-1"), tc.sent)
	}
}

func TestAReplyAckDeliversOurMessage(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	out, _ := tc.Send("TESTB-1", "question")
	tc.run(tc.after(time.Second))
	tc.hear("TESTB-1", Msg{To: "TESTA-1", Text: "answer", ID: "7", ReplyAck: out[0].ID})
	if tc.Messages("TESTB-1")[0].State != StateDelivered {
		t.Fatal("the reply-ack did not count")
	}
}

func TestTheRoom(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	if _, err := tc.Send("cq", "anyone around?"); err != nil {
		t.Fatal(err)
	}
	tc.run(tc.after(time.Hour))
	if len(tc.sent) != 2 || tc.sent[0].To != "CQ" || tc.sent[0].ID != "" || tc.Messages("CQ")[0].State != StateSent {
		t.Fatalf("%+v %+v", tc.sent, tc.Messages("CQ"))
	}
	for range 2 { // sent twice by the other side
		tc.hear("TESTB-1", Msg{To: "CQ", Text: "here!"})
	}
	tc.hear("TESTC-1", Msg{To: "QST", Text: "here!"})
	room := tc.Messages("CQ")
	if len(room) != 3 || room[1].From != "TESTB-1" || room[2].From != "TESTC-1" || len(tc.sent) != 2 {
		t.Fatalf("%+v; sent %+v", room, tc.sent)
	}
	conv := tc.Conversations()
	if conv[0].Peer != "CQ" || conv[0].Unread != 2 || conv[0].Count != 3 {
		t.Fatalf("%+v", conv)
	}
}

func TestSendChecks(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	for _, c := range []struct{ to, text string }{
		{"TESTA-1", "to myself"}, {"not a call", "x"}, {"TESTB-1", "   "}, {"TESTB-1", strings.Repeat("x", 6*67)},
	} {
		if _, err := tc.Send(c.to, c.text); err == nil {
			t.Errorf("%q %q accepted", c.to, c.text)
		}
	}
	out, err := tc.Send("TESTB-1", "Grüße "+strings.Repeat("lang ", 20))
	if err != nil || len(out) != 2 || !strings.HasPrefix(out[0].Text, "Gruesse") || out[0].ID == out[1].ID {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestOwnFramesAndOtherFrames(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	tc.hear("TESTA-1", Msg{To: "TESTB-1", Text: "our own, digipeated", ID: "1"})
	if len(tc.Messages("TESTB-1")) != 0 {
		t.Fatal("our own message was stored")
	}
	pos, _ := ax25.NewUI(ax25.MustAddress("APZK5M"), ax25.MustAddress("TESTB-7"), nil, ax25.PIDNoL3,
		[]byte("!5230.00N/01323.00E-")).Encode()
	ip, _ := ax25.NewUI(ax25.MustAddress("TESTA-1"), ax25.MustAddress("TESTB-1"), nil, ax25.PIDIPv4,
		[]byte(":TESTA-1  :looks like a message")).Encode()
	if tc.Handle(pos) || tc.Handle(ip) {
		t.Fatal("a position or an IP frame was taken as a message")
	}
}

func TestTheMailboxSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlink", "messages.json")
	tc := newTestChat(t, "TESTA-1", path)
	tc.hear("TESTB-1", Msg{To: "TESTA-1", Text: "while you were away", ID: "5"})
	tc.Send("TESTB-1", "an answer")
	tc.run(tc.after(time.Hour)) // into the mailbox
	info, err := os.Stat(path)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) { // Windows has no such modes
		t.Fatalf("%v %v", info, err)
	}
	again := newTestChat(t, "TESTA-1", path)
	msgs := again.Messages("TESTB-1")
	if len(msgs) != 2 || msgs[0].Text != "while you were away" || msgs[0].Read || msgs[1].State != StateWaiting {
		t.Fatalf("%+v", msgs)
	}
	out, _ := again.Send("TESTB-1", "new")
	if out[0].N <= msgs[1].N || out[0].ID == msgs[1].ID {
		t.Fatalf("numbers repeat after a restart: %+v after %+v", out[0], msgs[1])
	}
	if DefaultPath(filepath.Join("cfg", "settings.json")) != filepath.Join("cfg", "messages.json") || DefaultPath("") != "" {
		t.Fatal("DefaultPath")
	}
}

func TestMessagesTakeTheWayOverRelays(t *testing.T) {
	tc := newTestChat(t, "TESTA-1", "")
	var paths [][]ax25.Address
	send := tc.cfg.Send
	tc.cfg.Send = func(frame []byte, key string) bool {
		f, _ := ax25.Decode(frame)
		paths = append(paths, f.Path)
		return send(frame, key)
	}
	tc.cfg.Route = func(peer ax25.Address) []ax25.Address {
		if peer.String() == "TESTC-1" {
			return []ax25.Address{ax25.MustAddress("TESTB-1")}
		}
		return nil
	}
	tc.cfg.Flood = func() []ax25.Address { return []ax25.Address{{Call: "AXHOP", SSID: 2}} }
	tc.Send("TESTC-1", "over B")
	tc.Send("CQ", "to all")
	tc.run(tc.after(time.Second))
	if len(paths) != 2 || len(paths[0]) != 1 || paths[0][0].String() != "TESTB-1" || paths[1][0].String() != "AXHOP-2" {
		t.Fatalf("%v", paths)
	}
	// A message from C over B: taken from B's copy, acknowledged the same way back.
	b := ax25.MustAddress("TESTB-1")
	msg, _ := Msg{To: "TESTA-1", Text: "from C", ID: "9"}.FrameVia(ax25.MustAddress("TESTC-1"), []ax25.Address{b})
	tc.Handle(msg)
	if len(tc.Messages("TESTC-1")) != 1 {
		t.Fatal("taken before the relay repeated it")
	}
	b.Bit7 = true
	msg, _ = Msg{To: "TESTA-1", Text: "from C", ID: "9"}.FrameVia(ax25.MustAddress("TESTC-1"), []ax25.Address{b})
	tc.Handle(msg)
	last := paths[len(paths)-1]
	if len(tc.Messages("TESTC-1")) != 2 || len(last) != 1 || last[0].String() != "TESTB-1" || last[0].Bit7 {
		t.Fatalf("%d messages, ack path %v", len(tc.Messages("TESTC-1")), last)
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package chat

import (
	"strings"
	"testing"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

func TestParse(t *testing.T) {
	for info, want := range map[string]Msg{
		":TESTB-1  :Hello{12":            {To: "TESTB-1", Kind: KindText, Text: "Hello", ID: "12"},
		":TESTB-1  :Hello":               {To: "TESTB-1", Kind: KindText, Text: "Hello"},
		":testb-1  :lower case to":       {To: "TESTB-1", Kind: KindText, Text: "lower case to"},
		":TESTA-1  :ack12":               {To: "TESTA-1", Kind: KindAck, ID: "12"},
		":TESTA-1  :rej7":                {To: "TESTA-1", Kind: KindRej, ID: "7"},
		":TESTA-1  :ack12}":              {To: "TESTA-1", Kind: KindAck, ID: "12"},
		":TESTA-1  :ack12}34":            {To: "TESTA-1", Kind: KindAck, ID: "12"},
		":TESTA-1  :Hi again{13}12":      {To: "TESTA-1", Kind: KindText, Text: "Hi again", ID: "13", ReplyAck: "12"},
		":TESTA-1  :Hi{13}":              {To: "TESTA-1", Kind: KindText, Text: "Hi", ID: "13"},
		":TESTA-1  :acknowledge this{3":  {To: "TESTA-1", Kind: KindText, Text: "acknowledge this", ID: "3"},
		":TESTA-1  :a {brace} inside":    {To: "TESTA-1", Kind: KindText, Text: "a {brace} inside"},
		":CQ       :anyone around?":      {To: "CQ", Kind: KindText, Text: "anyone around?"},
		":TESTB-1  :trailing \r\n":       {To: "TESTB-1", Kind: KindText, Text: "trailing"},
		":BLN3FAMIL:club meeting friday": {To: "BLN3FAMIL", Kind: KindText, Text: "club meeting friday"},
	} {
		got, err := Parse([]byte(info))
		if err != nil || got != want {
			t.Errorf("%q: %+v %v, want %+v", info, got, err, want)
		}
	}
	for _, bad := range []string{"", "!5230.00N/01323.00E-", ":SHORT:x", ":         :empty addressee", ":TESTB-1   Hello"} {
		if m, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%q accepted as %+v", bad, m)
		}
	}
}

func TestInfoRoundTrip(t *testing.T) {
	for want, m := range map[string]Msg{
		":TESTB-1  :Hello{12": {To: "testb-1", Text: "Hello", ID: "12"},
		":CQ       :anyone?":  {To: "CQ", Text: "anyone?"},
		":TESTA-1  :ack12":    {To: "TESTA-1", Kind: KindAck, ID: "12"},
		":TESTA-1  :rej12":    {To: "TESTA-1", Kind: KindRej, ID: "12"},
		":TESTA-1  :Hi{13}12": {To: "TESTA-1", Text: "Hi", ID: "13", ReplyAck: "12"},
	} {
		info, err := m.Info()
		if err != nil || string(info) != want {
			t.Fatalf("%+v: %q %v, want %q", m, info, err, want)
		}
		if _, err := Parse(info); err != nil {
			t.Fatal(err)
		}
	}
	for _, bad := range []Msg{
		{To: "", Text: "x"}, {To: "TOOLONGADDR", Text: "x"}, {To: "TESTB-1", Text: strings.Repeat("x", 68)},
		{To: "TESTB-1", Text: "a|b"}, {To: "TESTB-1", Text: "Grüße"}, {To: "TESTB-1", Text: "x", ID: "123456"},
		{To: "TESTB-1", Kind: KindAck},
	} {
		if info, err := bad.Info(); err == nil {
			t.Errorf("%+v gave %q", bad, info)
		}
	}
}

func TestFrame(t *testing.T) {
	b, err := Msg{To: "TESTB-1", Text: "Hi", ID: "1"}.Frame(ax25.MustAddress("TESTA-1"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := ax25.Decode(b)
	if err != nil || f.Dest.String() != "APZAXL" || f.Src.String() != "TESTA-1" || f.PID != ax25.PIDNoL3 ||
		string(f.Info) != ":TESTB-1  :Hi{1" || len(f.Path) != 0 {
		t.Fatalf("%v %+v", err, f)
	}
}

func TestCleanAndSplit(t *testing.T) {
	if got := Clean("Grüße aus Köln – 5 °C, {ok} | ~ 日本\n"); got != "Gruesse aus Koeln - 5 degC, -ok} - - ?? " {
		t.Fatalf("%q", got)
	}
	long := strings.Repeat("word ", 30) // 150 characters
	parts := Split(Clean(long))
	if len(parts) != 3 {
		t.Fatalf("%d parts: %q", len(parts), parts)
	}
	for _, p := range parts {
		if len(p) > MaxText || strings.HasPrefix(p, " ") || strings.HasSuffix(p, " ") || !strings.HasSuffix(p, "word") {
			t.Fatalf("%q", p)
		}
	}
	if p := Split(strings.Repeat("x", 100)); len(p) != 2 || len(p[0]) != MaxText {
		t.Fatalf("a text without spaces: %q", p)
	}
	if p := Split("  "); len(p) != 0 {
		t.Fatalf("%q", p)
	}
	if !IsLobby("cq") || !IsLobby("QST") || !IsLobby("ALL") || IsLobby("TESTB-1") || IsLobby("BLN1") {
		t.Fatal("lobby")
	}
}

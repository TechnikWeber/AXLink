// SPDX-License-Identifier: GPL-3.0-or-later

package locator

import (
	"math"
	"testing"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{"jo62QM": "JO62qm", "JO62": "JO62", " io91wm ": "IO91wm", "JN58td25": "JN58td25",
		"jn58TD25xx": "JN58td25xx"} {
		if got, err := Normalize(in); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "JO6", "JO62q", "SO62", "JO6A", "JO62qz", "JO62qmA1", "JO62qm123", "near Berlin"} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseGivesTheCentre(t *testing.T) {
	// JO62qm: 13.333 to 13.417 E, 52.5 to 52.542 N (Berlin)
	p, err := Parse("JO62qm")
	if err != nil || math.Abs(p.Lon-13.375) > 0.001 || math.Abs(p.Lat-52.5208) > 0.001 {
		t.Fatalf("%+v %v", p, err)
	}
	// JO62: 12 to 14 E, 52 to 53 N
	if p, _ := Parse("JO62"); p.Lon != 13 || p.Lat != 52.5 {
		t.Fatalf("%+v", p)
	}
	// AA00aa00aa: the corner square at -180, -90
	if p, _ := Parse("AA00aa00aa"); p.Lon > -179.99 || p.Lat > -89.99 {
		t.Fatalf("%+v", p)
	}
}

func TestDistanceAndBearing(t *testing.T) {
	berlin, _ := Parse("JO62qm")
	munich, _ := Parse("JN58td")
	d := Distance(berlin, munich)
	if d < 495 || d > 510 { // about 504 km between the square centres
		t.Fatalf("Berlin-Munich %.1f km", d)
	}
	if b := Bearing(berlin, munich); b < 190 || b > 205 || Compass(b) != "S" {
		t.Fatalf("bearing %.1f %s", b, Compass(b))
	}
	if Distance(berlin, berlin) != 0 {
		t.Fatal("not 0 to itself")
	}
	for b, want := range map[float64]string{0: "N", 22: "N", 23: "NE", 90: "E", 180: "S", 270: "W", 337: "NW", 338: "N", 359.9: "N"} {
		if got := Compass(b); got != want {
			t.Errorf("%v: %s, want %s", b, got, want)
		}
	}
}

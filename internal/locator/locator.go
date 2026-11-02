// SPDX-License-Identifier: GPL-3.0-or-later

// Package locator reads Maidenhead locators (e.g. JO62qm) and gives the
// distance and direction between two of them, from the centres of their
// squares.
package locator

import (
	"errors"
	"math"
	"strings"
)

var errLocator = errors.New("locator: want 4, 6, 8 or 10 characters such as JO62 or JO62qm")

// Point is a position in degrees.
type Point struct{ Lat, Lon float64 }

// Normalize checks a locator and writes it in the usual case: field
// upper-case, subsquare lower-case (JO62qm).
func Normalize(s string) (string, error) {
	s = strings.TrimSpace(s)
	if n := len(s); n < 4 || n > 10 || n%2 != 0 {
		return "", errLocator
	}
	out := []byte(s)
	for i := range out {
		c := out[i]
		switch pair := i / 2; {
		case pair == 0: // field A-R
			c = upper(c)
			if c < 'A' || c > 'R' {
				return "", errLocator
			}
		case pair%2 == 1: // square, extended square: digits
			if c < '0' || c > '9' {
				return "", errLocator
			}
		default: // subsquare a-x
			c = lower(c)
			if c < 'a' || c > 'x' {
				return "", errLocator
			}
		}
		out[i] = c
	}
	return string(out), nil
}

// Parse returns the centre of a locator's square.
func Parse(s string) (Point, error) {
	s, err := Normalize(s)
	if err != nil {
		return Point{}, err
	}
	lon, lat := -180.0, -90.0
	w, h := 20.0, 10.0 // the field
	for i := 0; i < len(s); i += 2 {
		var div float64
		var x, y float64
		switch pair := i / 2; {
		case pair == 0:
			div, x, y = 1, float64(s[i]-'A'), float64(s[i+1]-'A')
		case pair%2 == 1:
			div, x, y = 10, float64(s[i]-'0'), float64(s[i+1]-'0')
		default:
			div, x, y = 24, float64(s[i]-'a'), float64(s[i+1]-'a')
		}
		if i > 0 {
			w, h = w/div, h/div
		}
		lon += x * w
		lat += y * h
	}
	return Point{Lat: lat + h/2, Lon: lon + w/2}, nil
}

const earthKm = 6371.0

// Distance is the great-circle distance in kilometres.
func Distance(a, b Point) float64 {
	la1, la2 := rad(a.Lat), rad(b.Lat)
	dLat, dLon := la2-la1, rad(b.Lon-a.Lon)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthKm * math.Asin(math.Min(1, math.Sqrt(h)))
}

// Bearing is the initial direction from a to b in degrees, 0 = north,
// 90 = east.
func Bearing(a, b Point) float64 {
	la1, la2 := rad(a.Lat), rad(b.Lat)
	dLon := rad(b.Lon - a.Lon)
	y := math.Sin(dLon) * math.Cos(la2)
	x := math.Cos(la1)*math.Sin(la2) - math.Sin(la1)*math.Cos(la2)*math.Cos(dLon)
	return math.Mod(deg(math.Atan2(y, x))+360, 360)
}

// Compass names a bearing with one of eight points: N, NE, E, SE, S, SW,
// W, NW.
func Compass(bearing float64) string {
	points := [8]string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	return points[int(math.Mod(bearing+22.5, 360)/45)]
}

func rad(d float64) float64 { return d * math.Pi / 180 }
func deg(r float64) float64 { return r * 180 / math.Pi }

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

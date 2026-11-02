// SPDX-License-Identifier: GPL-3.0-or-later

package airtime

import (
	"testing"
	"time"
)

func TestStuffedBits(t *testing.T) {
	for _, tc := range []struct {
		in   []byte
		want int
	}{
		{nil, 0},
		{[]byte{0x00}, 8},
		{[]byte{0x1F}, 9},              // five 1 bits, then a stuffed 0
		{[]byte{0xFF}, 9},              // 11111 (0) 111
		{[]byte{0xFF, 0xFF}, 16 + 3},   // the run continues across bytes
		{[]byte{0x7E}, 9},              // a flag value as data: 0 11111 (0) 1 0
		{[]byte{0xF0, 0x0F}, 16 + 1},   // 0000 1111 1111 0000 (LSB first)
		{[]byte{0x55, 0xAA, 0x55}, 24}, // alternating bits never stuff
	} {
		if got := StuffedBits(tc.in); got != tc.want {
			t.Errorf("StuffedBits(% X) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestDuration(t *testing.T) {
	if got := Duration(1200, 1200); got != time.Second {
		t.Fatalf("got %v", got)
	}
	// The F4HWN KISS app sends 40 flags (267 ms) before every frame by default.
	if got := Duration(40*8, 1200); got.Round(time.Millisecond) != 267*time.Millisecond {
		t.Fatalf("got %v", got)
	}
}

func TestBitsIncludesFCSAndFlags(t *testing.T) {
	frame := make([]byte, 16) // zeros: no stuffing in the frame itself
	got := Bits(frame, 40, 3)
	// 40+3 flags, 16 frame bytes, 2 FCS bytes (stuffing of the FCS varies).
	if got < 8*(40+3+16+2) || got > 8*(40+3+16+2)+4 {
		t.Fatalf("Bits = %d", got)
	}
}

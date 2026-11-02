// SPDX-License-Identifier: GPL-3.0-or-later

// Package airtime computes how long an AX.25 frame occupies the channel under
// HDLC framing: preamble flags, the bit-stuffed frame with its FCS, tail flags.
package airtime

import (
	"time"

	"github.com/TechnikWeber/AXLink/internal/ax25"
)

// StuffedBits returns the number of bits b takes on the air after HDLC bit
// stuffing: bytes go out LSB first, and a 0 follows every five 1 bits in a row.
func StuffedBits(b []byte) int {
	bits, ones := 0, 0
	for _, v := range b {
		for i := 0; i < 8; i++ {
			bits++
			if v>>i&1 == 1 {
				ones++
				if ones == 5 {
					bits++
					ones = 0
				}
			} else {
				ones = 0
			}
		}
	}
	return bits
}

// Bits returns the bits a frame (without FCS) occupies on the air with the
// given number of preamble and tail flags.
func Bits(frame []byte, txDelayFlags, tailFlags int) int {
	withFCS := ax25.AppendFCS(append([]byte(nil), frame...))
	return 8*txDelayFlags + StuffedBits(withFCS) + 8*tailFlags
}

// Duration returns the time n bits take at baud bit/s.
func Duration(bits, baud int) time.Duration {
	return time.Duration(bits) * time.Second / time.Duration(baud)
}

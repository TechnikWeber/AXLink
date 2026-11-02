// SPDX-License-Identifier: GPL-3.0-or-later

package ipbridge

import (
	"encoding/binary"
	"fmt"

	"github.com/TechnikWeber/AXLink/internal/txqueue"
)

// UrgentMax is the largest IP packet that goes before larger ones in the
// transmit queue: TCP acknowledgements and set-up, pings, DNS.
const UrgentMax = 128

const (
	tcpFIN = 0x01
	tcpSYN = 0x02
	tcpRST = 0x04
)

// classify tells the transmit queue what a packet is (see package txqueue):
// small packets are urgent; packets of one flow keep their order; a TCP
// retransmission replaces its waiting original and is dropped if the
// original has just gone out; a newer pure ACK replaces a waiting older one.
func classify(pkt []byte) txqueue.Meta {
	if len(pkt) < 20 {
		return txqueue.Meta{}
	}
	ihl := int(pkt[0]&0x0F) * 4
	src, dst, proto := pkt[12:16], pkt[16:20], pkt[9]
	if binary.BigEndian.Uint16(pkt[6:8])&(flagMF|offsetMask) != 0 || ihl < 20 || len(pkt) < ihl {
		// A fragment: ordinary, in the order of its datagram.
		return txqueue.Meta{Flow: fmt.Sprintf("frag %d %x %x", proto, src, dst)}
	}
	m := txqueue.Meta{Urgent: len(pkt) <= UrgentMax, Flow: fmt.Sprintf("ip %d %x %x", proto, src, dst)}
	l4 := pkt[ihl:]
	switch proto {
	case 6:
		if len(l4) < 20 {
			return m
		}
		dataOff := int(l4[12]>>4) * 4
		if dataOff < 20 || dataOff > len(l4) {
			return m
		}
		m.Flow = fmt.Sprintf("tcp %x:%x %x:%x", src, l4[0:2], dst, l4[2:4])
		payload := len(l4) - dataOff
		flags := l4[13] & (tcpFIN | tcpSYN | tcpRST)
		if payload == 0 && flags == 0 {
			m.Replace = "ack " + m.Flow // cumulative: the newest says it all
			return m
		}
		seq := binary.BigEndian.Uint32(l4[4:8])
		m.Replace = fmt.Sprintf("seg %s seq %d len %d flags %x", m.Flow, seq, payload, flags)
		m.Repeat = m.Replace
	case 17:
		if len(l4) >= 4 {
			m.Flow = fmt.Sprintf("udp %x:%x %x:%x", src, l4[0:2], dst, l4[2:4])
		}
	}
	return m
}

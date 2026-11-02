// SPDX-License-Identifier: GPL-3.0-or-later

package ipbridge

import (
	"bytes"
	"testing"
)

func tcpPacket(src, dst string, flags byte, opts []byte, payload int) []byte {
	seg := append(tcpSegment(src, dst, flags, opts), bytes.Repeat([]byte{'x'}, payload)...)
	return ipv4(src, dst, 6, nil, seg, flagDF)
}

func TestClassifyTCP(t *testing.T) {
	syn := tcpPacket("10.73.0.1", "10.73.0.2", tcpSYN, synOptions(536), 0)
	again := append([]byte(nil), syn...)
	again[4], again[5] = 0x99, 0x99 // a retransmission has another IP ID
	m, m2 := classify(syn), classify(again)
	if !m.Urgent || m.Repeat == "" || m.Replace != m.Repeat || m2 != m {
		t.Fatalf("SYN %+v, again %+v", m, m2)
	}

	data := classify(tcpPacket("10.73.0.1", "10.73.0.2", 0x18, nil, 200)) // 240 bytes
	if data.Urgent || data.Repeat == "" || data.Repeat == m.Repeat || data.Flow != m.Flow {
		t.Fatalf("data %+v", data)
	}

	ack := classify(tcpPacket("10.73.0.1", "10.73.0.2", 0x10, nil, 0))
	if !ack.Urgent || ack.Repeat != "" || ack.Replace == "" || ack.Flow != m.Flow {
		t.Fatalf("pure ACK %+v", ack)
	}
	back := classify(tcpPacket("10.73.0.2", "10.73.0.1", 0x10, nil, 0))
	if back.Flow == ack.Flow || back.Replace == ack.Replace {
		t.Fatalf("the other direction shares keys: %+v", back)
	}
}

func TestClassifyOtherPackets(t *testing.T) {
	ping := ipv4("10.73.0.1", "10.73.0.2", 1, nil, make([]byte, 64), 0) // 84 bytes
	if m := classify(ping); !m.Urgent || m.Replace != "" || m.Repeat != "" || m.Flow == "" {
		t.Fatalf("ping %+v", m)
	}
	if m := classify(udpPacket("10.73.0.1", "10.73.0.2", string(make([]byte, 200)))); m.Urgent || m.Flow == "" {
		t.Fatalf("large UDP %+v", m)
	}
	frag := ipv4("10.73.0.1", "10.73.0.2", 17, nil, make([]byte, 40), flagMF)
	if m := classify(frag); m.Urgent || m.Replace != "" || m.Repeat != "" {
		t.Fatalf("fragment %+v", m)
	}
}

func TestBridgeHandsTheClassToThePort(t *testing.T) {
	h := newHarness(t, stationA())
	h.b.FromAdapter(tcpPacket("10.73.0.1", "10.73.0.2", tcpSYN, synOptions(536), 0))
	h.b.FromAdapter(ipv4("10.73.0.1", "10.73.0.2", 17, nil, bytes.Repeat([]byte{7}, 600), 0))
	if len(h.meta) != 4 || !h.meta[0].Urgent || h.meta[0].Repeat == "" {
		t.Fatalf("meta %+v", h.meta)
	}
	for _, m := range h.meta[1:] { // the fragments of the large datagram
		if m.Urgent || m.Repeat != "" || m.Flow != h.meta[1].Flow {
			t.Fatalf("fragment meta %+v", m)
		}
	}
}

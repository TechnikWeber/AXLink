# AXLink relays (mesh), version 1

Licence: this specification is under CC BY 4.0 ([licenses/CC-BY-4.0.txt](../../licenses/CC-BY-4.0.txt));
any project may implement it. Status: draft, implemented in axlinkd 0.2.0-dev, tested in the lab
(three stations in a line), not yet used on the air.

## Purpose

Stations on one frequency that do not all hear each other. A station whose operator makes it a
**relay** (beacon tag `RELAY`) repeats frames as an AX.25 digipeater, so that A reaches C over B.
Everything stays on layer 2: a relay does not route IP, needs no routing table and never passes
what it repeats to its own computer.

## Two ways a relay repeats

**A frame that names it.** The relay's callsign is the first digipeater in the path without the
H bit: the relay sets the H bit and sends the frame again, unchanged otherwise. This is ordinary
AX.25 digipeating; IP frames (PID `0xCC`), chat messages to a station and their acknowledgments
use it.

**The flood alias `AXHOP-n`.** The first unused digipeater is `AXHOP` with SSID n ≥ 1, the frame
is a UI frame with PID `0xF0` (beacons, messages to CQ), the path is shorter than 8 digipeaters
and does not contain the relay's callsign yet. The relay inserts its callsign with the H bit
before the alias and lowers n by one; at 0 the alias gets the H bit:

```text
TESTC-1>AXLINK,AXHOP-2              sent by C
TESTC-1>AXLINK,TESTB-1*,AXHOP-1     repeated by B
TESTC-1>AXLINK,TESTB-1,TESTD-1*,AXHOP*  repeated by D: no hops left
```

Senders use `AXHOP-2` (at most two relays), and only while they hear a station with the tag
`RELAY` directly; without a relay around, frames go without a path. IP frames are never flooded.
A sender hears its own flooded beacon come back from the relay; without that copy it sends the
beacon again (see the beacon's timing): stations that do not hear each other may have collided
at the relay.

**Duplicates.** A relay does not repeat the same frame (source, destination, PID and information
field; the path changes on the way) twice within 30 s when flooding and 5 s when named.

## Receiving

- A frame whose path still names a station (not an alias) without the H bit is **in transit**:
  its destination ignores it and takes the relay's copy. An alias not yet used (`WIDE1-1`,
  `AXHOP-2`) does not hold a frame back; it only asks for a repeat.
- The callsigns with the H bit, without aliases, are the stations that repeated the frame, in
  order. Reversed they are the **way back** to the sender: an acknowledgment or a refusal goes
  that way.
- A beacon heard over relays gives the way to its sender (see [beacon.md](beacon.md)). A
  station keeps one way per station: none (direct) while its beacons arrive directly, else the
  shortest one heard, replaced by a longer one only when the shorter has been silent for its
  beacon interval plus a minute. Frames to that station name the relays of its way, first relay
  first.

## Load

Every repeat costs the frame's airtime once more on the same frequency: a frame from A to C over
B takes twice the airtime of a direct one. Stations that do not hear each other may transmit at
the same time and collide at the relay (hidden stations); AX.25 has no remedy for that on one
frequency, and the senders' own repetitions (TCP, chat retries) recover the frame.

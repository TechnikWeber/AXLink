# AXLink station beacon, version 1

Licence: this specification is under CC BY 4.0 ([licenses/CC-BY-4.0.txt](../../licenses/CC-BY-4.0.txt));
any project may implement it. Status: draft, implemented in axlinkd 0.2.0-dev, not yet used on
the air.

## Purpose

A station announces its IPv4 address and its roles. Others can then reach it without a
configured route and can find gateways. A beacon is one AX.25 UI frame.

## Frame

| Field | Value |
|---|---|
| Destination | `AXLINK`, SSID 0 |
| Source | the station's callsign and SSID, the same as on its IP frames |
| Digipeaters | none, or the flood alias `AXHOP-2` while the sender hears a relay directly, so that relays pass the beacon on ([mesh.md](mesh.md)) |
| Control | UI (`0x03`), command |
| PID | `0xF0` (no layer 3) |
| Information | ASCII text as below, at most 128 bytes |

## Text

`AXL1`, then fields, each as a space and `key=value`:

```text
AXL1 ip=10.73.0.2/24 tags=INET,AUTO
```

| Field | | Meaning |
|---|---|---|
| `ip` | required | the station's IPv4 address with the prefix length of its subnet |
| `every` | optional | seconds until its next periodic beacon, 60–86400; default 600 |
| `tags` | optional | roles, comma-separated: 1–8 characters `A`–`Z` and `0`–`9` each, at most 8, no duplicates |
| `proxy` | optional | TCP port 1–65535 of an HTTP proxy on the sender's address that forwards to the internet (requests with full URLs and CONNECT); with the tag `INET` |
| `loc` | optional | the sender's Maidenhead locator, 4, 6, 8 or 10 characters (`JO62`, `JO62qm`); only when its operator chose to announce it. Receivers accept any case; encoders write the subsquare in lower case |
| `ask` | optional | only `ask=1`: the sender has just started and asks the others for their beacons |

- Keys are lower-case letters and digits; values are printable ASCII without spaces
  (`0x21`–`0x7E`). Every key appears at most once.
- Receivers ignore unknown keys and keep unknown tags without a meaning, so that later versions
  can add both. Anything else makes the beacon invalid: another first word, two spaces, an
  address with leading zeros, a value out of range.
- Encoders write `ip`, `every`, `tags`, `proxy`, `loc`, `ask` in this order and leave out `every`
  when it is 600, and `proxy`, `loc` and `ask` when they are not set.

## Tags

| Tag | Meaning |
|---|---|
| `RELAY` | forwards for others, by choice of its operator |
| `INET` | shares an internet connection with the radio network |
| `LINK` | connects radio islands over the internet (AXUDP) |
| `AUTO` | runs unattended |

Tags describe the station; they grant nothing. A client uses a gateway only when its operator
chooses that, e.g. by setting the announced proxy as its HTTP proxy.

## Timing

- After the start: a beacon with `ask=1` after a random 1–5 s. Every beacon carries `ask=1`
  until the first one has gone out.
- Then periodically every `every` seconds, varied at random by ±10 %.
- Answer: after a beacon with `ask=1`, or one from a station that is not in the table, a station
  sends its own beacon after a random 2–8 s, unless it has sent one since it heard that beacon.
  It answers at most once in 30 s and postpones a later answer instead of dropping it.
- A station is forgotten 3 × its `every` + 60 s after its last beacon.

Load at 1200 bit/s: a beacon of 30–50 bytes is about 0.6 s on the air. Every 10 minutes that is
0.1 % of the channel per station, while answering at most 2 % (one beacon in 30 s).

## Use

- Packets for an announced address go to the announcing station's callsign. A configured route
  for exactly that address comes first; a learned address comes before configured shorter
  prefixes.
- A beacon that relays repeated gives the way to its sender: the relays that repeated it, last
  first ([mesh.md](mesh.md)). Packets to that station name those relays. A beacon whose path
  still names a relay that has not repeated it is ignored; the relay's copy follows.
- When two stations announce the same address, the newer beacon wins and the receiver warns. A
  beacon with the receiver's own address, or a different one with its own callsign, also makes
  it warn.
- Beacons are not authenticated, just as ARP is not; the source callsign identifies the sender.

# AXLink — architecture

## Goal

Ordinary IPv4 programs over amateur packet radio: a KISS radio or TNC, a virtual network
adapter, and IP packets in AX.25 UI frames with PID `0xCC`, the framing that Linux, JNOS, BPQ and
Packet.NET use. Later: station discovery, chat, gateways and mesh routing. At 1200 bit/s this
suits small, text-based use.

## Today

```text
 ordinary programs (ping, curl, servers)
          │ sockets
   operating-system IPv4 stack
          │ axl0 (TUN, MTU 256)
┌──────────────────────────── axlinkd ─────────────────────────────┐
│ internal/tun        virtual network adapter (Linux)               │
│ internal/ipbridge   IPv4 ↔ AX.25 UI/PID 0xCC, destination filter, │
│                     routes, broadcast filter                       │
│ internal/tnc        connection, reconnecting, paced sending        │
│ internal/pacing     profile per TNC                                │
│ internal/kiss · internal/ax25 · internal/airtime                   │
└────────────────────────────────────────────────────────────────────┘
          │ USB-CDC (serial) · KISS over TCP · Unix socket
   radio with the F4HWN KISS app · any KISS TNC · the lab channel
```

## Data path

**Out:** the operating system routes a packet to `axl0`. The bridge checks the IPv4 header,
drops broadcast and multicast (switchable), finds the destination callsign by the longest
matching route, and builds a UI command frame: destination the route's callsign, source the
own callsign, PID `0xCC`, the IP packet as the information field. The frame goes into the port's
queue; the sender encodes it as KISS (or ACKMODE) and writes it as the TNC profile's plan says.

**In:** the KISS decoder hands every data frame to the bridge. Only UI frames with PID `0xCC`
addressed to the own callsign and SSID pass, and only with a valid IPv4 header (checksum,
length). The packet then goes to `axl0`.

## TNC profiles

| Profile | For | Behaviour |
|---|---|---|
| `generic-kiss` | TNCs with a queue (Dire Wolf, most hardware TNCs) | whole frames, no waiting |
| `f4hwn-kisstnc` | the KISS TNC app of the F4HWN firmware as it is | one frame in flight; writes of ≤ 64 B every 40 ms (the app drains 1.6 kB/s); the next frame after the estimated airtime; PERSIST 255 with channel access left to the host |
| `f4hwn-kisstnc-ackmode` | AXLink TNC with ACKMODE | as above, but waits for the ACKMODE echo instead of an estimate |

Why: the F4HWN app holds one frame and its USB ring has no back-pressure; see section 4 of
[REVIEW_2026-11-02.md](REVIEW_2026-11-02.md).

After a USB reset (RF on the cable at key-up) the port dials again, finding the radio by USB ID
`36B7:FFFF`. It then sends `FESC 00 FEND`, which makes the app drop a frame the reset cut off, and
the KISS parameters again. A frame that did not reach the TNC completely is sent once more.

## Addressing

For now static: the own address and prefix on the adapter, routes `IP/PREFIX=CALLSIGN`.
Private addresses (e.g. `10.73.0.0/24`) suit tests and private networks; AMPRNet (44/8)
addresses come with an allocation. Automatic addressing comes with station discovery.

## Simulation and lab

- `internal/sim/f4hwnkiss`: a behavioural model of the F4HWN KISS app, written from its source,
  in virtual time.
- `internal/sim/channel`: a frame-level radio channel in real time: half duplex, collisions, a
  JSON trace; in front of each station either an ideal TNC or the F4HWN model (optionally with
  ACKMODE).
- `scripts/lab-netns.sh`: two stations in their own network namespaces with real kernel IP
  stacks; ping, UDP and HTTP. Also in CI.

Results from the model are not hardware results.

## Platforms

- Linux: TUN through wireguard-go without offloads (one packet per read), configured with
  iproute2; root or `CAP_NET_ADMIN`. Builds for amd64, arm64 and ARMv6 (Pi Zero W).
- Windows: serial ports work; the adapter (Wintun through wireguard-go) is planned.
- macOS: serial ports work (USB details need cgo); the adapter (utun) is planned.

## Planned

- Station discovery with tags `RELAY`, `INET`, `LINK`, `AUTO`.
- Chat in the APRS message format, so that messenger radios can take part.
- A transmit queue with priorities and duplicate suppression.
- A link gateway (AXUDP between radio islands) and an internet gateway in userspace (gVisor
  netstack); operation without admin rights through a local proxy.
- Mesh forwarding in userspace, independent of the operating system's forwarding.
- A KISS-TCP multiplexer, so that other programs can share the radio.
- A web UI in English and German.

## Design rules

- Strict parsers; frames of other implementations serve as cross-checks.
- One process owns a serial port (it is opened exclusively).
- No legal restrictions built in (D2); technical channel protection can be switched off.
- Placeholder callsigns only (D3). No transmission in tests.

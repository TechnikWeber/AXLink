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
          │ axl0 (Linux: TUN, MTU 256 · Windows: Wintun, MTU 576)
┌───────────────────────────── axlinkd ──────────────────────────────┐
│ internal/tun        virtual network adapter (Linux, Windows)       │
│ internal/ipbridge   IPv4 ↔ AX.25 UI/PID 0xCC, destination filter,  │
│                     routes, broadcast filter, MSS clamping,        │
│                     fragmentation                                  │
│ internal/discovery  station beacons, learned addresses             │
│ internal/gateway    HTTP proxy that shares the internet            │
│ internal/txqueue    transmit queue: small first, repeats merged    │
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
own callsign, PID `0xCC`, the IP packet as the information field. The frame goes into the
transmit queue; the sender takes one frame at a time, encodes it as KISS (or ACKMODE) and writes
it as the TNC profile's plan says.

**Transmit queue** (`internal/txqueue`): a full frame is two seconds on the air at 1200 bit/s,
so the order matters. The bridge tells the queue what each packet is:

- Packets of up to 128 bytes (TCP acknowledgements and set-up, pings, DNS) go first. A larger
  frame waits behind at most three of them, and a small one never overtakes a larger one of its
  own flow.
- A TCP retransmission takes the place of its original while that still waits; a newer pure ACK
  that of an older one (acknowledgements are cumulative).
- A TCP retransmission is dropped if its original went to the TNC less than twice the frame's
  airtime plus one second ago: no answer could have come back yet. TCP repeats a SYN after one
  second, while the answer over the radio takes about two.
- Frames that waited longer than 60 s are dropped.

**In:** the KISS decoder hands every data frame to the bridge. Only UI frames with PID `0xCC`
addressed to the own callsign and SSID pass, and only with a valid IPv4 header (checksum,
length). The packet then goes to `axl0`.

**MTU:** an IP packet on the radio has at most 256 bytes (`-mtu`), the classic AX.25
information field. Linux gives the adapter the same MTU. Windows does not accept an IPv4 MTU
below 576 on an adapter, so there the adapter MTU is larger than the radio's. The bridge then
behaves like a router in front of a smaller link:

- It lowers the MSS option of TCP SYNs in both directions to the radio MTU − 40 (216), so TCP
  segments fit; it corrects the TCP checksum.
- It fragments other packets without DF (RFC 791); each fragment travels in its own UI frame
  and the receiving IP stack reassembles them.
- It drops packets with DF that do not fit (counter `TooBig`). It does not answer with ICMP
  "fragmentation needed" yet.

## TNC profiles

| Profile | For | Behaviour |
|---|---|---|
| `generic-kiss` | TNCs with a queue (Dire Wolf, most hardware TNCs) | whole frames, no waiting |
| `f4hwn-kisstnc` | the KISS TNC app of the F4HWN firmware as it is | one frame in flight; writes of ≤ 64 B every 40 ms (the app drains 1.6 kB/s); the next frame after the estimated airtime, later if the radio heard another station before the frame could have gone out; PERSIST 255 with channel access left to the host |
| `f4hwn-kisstnc-ackmode` | AXLink TNC with ACKMODE | as above, but waits for the ACKMODE echo instead of an estimate, and PERSIST 63: the app randomises channel access, so two stations waiting for a busy channel do not both start in the first clear slot |

Why: the F4HWN app holds one frame and its USB ring has no back-pressure; see section 4 of
[REVIEW_2026-11-02.md](REVIEW_2026-11-02.md).

The estimate assumes a clear channel. The app sends only when the channel is clear, so a frame
handed over while another station transmits goes out late; a next frame written during that
transmission would lap the app's USB ring. Every frame the radio receives therefore goes into a
small log: if one began before the own frame could have ended, the own frame waited for it, and
the next frame waits accordingly longer. The check runs again before every next frame, because a
long frame of the other station may end only after the estimate, and the next own frame is
often the answer to it (a TCP acknowledgement). A frame still on the air when the next one is
ready cannot be known yet; only ACKMODE covers that case.

After a USB reset (RF on the cable at key-up) the port dials again, finding the radio by USB ID
`36B7:FFFF`. It then sends `FESC 00 FEND`, which makes the app drop a frame the reset cut off, and
the KISS parameters again. A frame that did not reach the TNC completely is sent once more.

## Addressing

Each station has its own address and prefix on the adapter (`-ip`, still set by hand). It
announces both, with its tags, in a beacon ([protocol/beacon.md](protocol/beacon.md), CC BY 4.0):
a short UI frame to `AXLINK` with PID `0xF0`, e.g. `AXL1 ip=10.73.0.2/24 tags=INET`, once
shortly after the start and then every 10 minutes. A station that has just started asks the
others for their beacons (`ask=1`); they answer after 2–8 s, at most once in 30 s. Others thus
learn which callsign has which address; an entry is forgotten after three missed beacons.

Packets for a learned address go to that callsign. A configured route `IP/PREFIX=CALLSIGN` for
exactly that address comes first, a learned address before configured shorter prefixes. Two
stations with one address: the newer beacon wins, with a warning. Beacons are not
authenticated, just as ARP is not.

Private addresses (e.g. `10.73.0.0/24`) suit tests and private networks; AMPRNet (44/8)
addresses come with an allocation.

## Internet gateway

A station with an internet connection shares it as an HTTP proxy (`internal/gateway`,
`run -proxy 8080`) on its radio address, and announces `tags=INET proxy=8080` in its beacon.
Clients use it deliberately, by setting it as their HTTP proxy; no default route goes over the
radio, so updates and background traffic of the client stay off the air.

- Requests with full URLs are forwarded; HTTPS goes through CONNECT tunnels, end to end
  encrypted between client and server.
- The gateway resolves names itself: DNS costs no airtime.
- It does not connect to private, shared (100.64/10), loopback or link-local addresses unless
  `-proxy-lan` allows it; radio users get the internet, not the operator's own network. The
  check runs on every resolved address, right before the connection. This protects the
  operator's network; it is no rule about content (D2).
- At most 16 client connections at a time; a connection silent for 10 minutes is closed. The
  log names the client's callsign (from its beacon) for each request.

Planned: the gateway's rules in its beacon (services, rate, contact), a local proxy on the client
that picks a heard gateway, pages shrunk for 1200 bit/s, and a transparent gateway in userspace
(gVisor netstack) for other protocols.

## Simulation and lab

- `internal/sim/f4hwnkiss`: a behavioural model of the F4HWN KISS app, written from its source,
  in virtual time.
- `internal/sim/channel`: a frame-level radio channel in real time: half duplex, collisions, a
  JSON trace; in front of each station either an ideal TNC or the F4HWN model (optionally with
  ACKMODE). The trace also records frames the model drops before the air.
- `scripts/lab-netns.sh`: two stations in their own network namespaces with real kernel IP
  stacks. They find each other by beacons; then ping, UDP, HTTP and the internet through B as a
  gateway (a proxied request, a CONNECT tunnel, the own network refused). Optionally with
  adapter MTU 576 as on Windows, which adds a fragmented ping and a 2 KB download. Also in CI.
  The model draws its p-persistence from a seeded random source, as the app does.

Results from the model are not hardware results.

## Platforms

- Linux: TUN through wireguard-go without offloads (one packet per read), configured with
  iproute2; root or `CAP_NET_ADMIN`. Builds for amd64, arm64 and ARMv6 (Pi Zero W).
- Windows: Wintun through wireguard-go, with a fixed GUID per adapter name so that Windows
  recognises the network again; address, routes and MTU through the IP Helper API (`winipcfg`
  of WireGuard for Windows). Needs administrator rights and `wintun.dll` next to `axlinkd.exe`
  (`scripts/fetch-wintun.ps1` until a Windows package contains it, D8). Adapter MTU at least 576, see MTU above. CI creates
  a real adapter and pings through it; not yet tested with a radio.
- macOS: serial ports work (USB details need cgo); the adapter (utun) is planned.

## Planned

- Gateway rules in the beacon, a client-side proxy that picks a gateway, shrunk pages, a
  transparent userspace gateway (see Internet gateway).
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

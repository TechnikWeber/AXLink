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
│                     fragmentation; only the own address comes in   │
│ internal/firewall   what radio stations may send to this computer  │
│ internal/discovery  station beacons, learned addresses             │
│ internal/gateway    gateway proxy, local proxy, choice of gateway  │
│ internal/welcome    the station's own page on its radio address    │
│ internal/ui         web interface (English, German), live monitor  │
│ internal/settings   settings that survive a restart                │
│ internal/monitor    throughput and channel load, sampled           │
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

**In:** the KISS decoder hands every data frame to the bridge (beacons go to station discovery).
Only UI frames with PID `0xCC` addressed to the own callsign and SSID pass, only with a valid
IPv4 header (checksum, length), and only for the own IP address: a station must not use this
computer to get into other networks, even when the system forwards packets (Docker, for example,
switches that on). Broadcasts pass only with `-allow-broadcast`. The firewall decides last;
then the packet goes to `axl0`.

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
- It can be open to listed callsigns only; others get a clear refusal.
- Sharing is switched on and off in the web interface (or with `-proxy`) without a restart.

Several gateways: a client uses the one it chose, or in `auto` mode the gateway heard most
recently, kept while it is heard. There is no load sharing and no measure of link quality yet;
a gateway that falls silent is replaced in `auto` mode once its entry expires (three missed
beacons).

On the client, a local proxy (`127.0.0.1:7374`) forwards every request to the chosen gateway's
proxy over the radio; browser and programs are set up once. Requests for stations of the radio
network (an address in the own subnet, a station heard, or `<callsign>.axl`) it serves itself,
without a gateway, so that welcome pages open with the same proxy setting. The choice is `off`, `auto` (the
gateway heard most recently, kept while it is heard) or one station, made in the web interface
or with `-gateway`. Connections to the gateway are kept open between requests, which saves the
TCP handshake over the radio.

Planned: the gateway's rules in its beacon (services, rate, contact), pages shrunk for
1200 bit/s, and a transparent gateway in userspace (gVisor netstack) for other protocols.

## Firewall

`internal/firewall` decides which packets from the radio reach this computer, before the
operating system sees them. The default mode is **protect**:

- answers to connections this computer started pass (TCP, UDP and pings are tracked; TCP for
  30 minutes, UDP for 3 minutes of silence);
- pings to this station pass (switchable), ICMP error messages always;
- a rule opens a port (TCP or UDP), for every station or only for listed callsigns; a callsign
  without SSID stands for all its SSIDs. The welcome page and the gateway open their own ports;
- later fragments pass if their first fragment did;
- everything else is dropped and counted; the last eight show in the web interface with the
  sender's callsign.

A dropped packet is answered, so the other side's program fails at once instead of trying
again over the air for a minute or two: TCP with a reset, as a closed port answers ("connection
refused"), everything else with ICMP "communication administratively prohibited" (type 3, code
13). Never answered: ICMP messages other than an echo request, resets, later fragments,
broadcasts and packets from addresses that are no single station. At most one answer per station
every 2 s and ten in a minute for all stations together. The setting "silent" drops without an
answer. An answer is 40 or 56 bytes, less airtime than the repeated attempts it saves. The
welcome page lists the opened ports with their notes, and whether a port is open only for some
stations.

Mode **open** lets every packet for the own address through, as before. Callsigns identify the
sender without authentication, as everywhere in amateur radio; a rule per callsign keeps casual
use out, not a determined attacker. The own network (home network) cannot be reached through
the radio at all: the bridge passes only packets for the own address, and the gateway refuses
private addresses unless `-proxy-lan` or the setting allows them.

## Port forwarding

`internal/forward` publishes a TCP service of the own network on a port of the radio address:
`-forward 2222=192.168.1.10:22=N0CALL-1` (or a row in the settings) lets the station N0CALL-1
reach the SSH server of 192.168.1.10 at `10.73.0.2:2222`. axlinkd accepts the connection itself
and opens one to the target, so the operating system needs neither forwarding nor NAT, and
nothing else of the own network becomes reachable. The firewall opens the port for the same
callsigns; the forwarding checks them again by the client's address (station discovery), so a
rule with callsigns holds in the firewall mode "open" too. A target `127.0.0.1:PORT` publishes a
service of this computer that listens locally only. At most eight connections per rule at the
same time; TCP only for now. The welcome page lists forwarded ports like opened ones.

## Welcome page

Every station offers a small page on its radio address (`http://10.73.0.2/`, port 80 by
default): callsign, name, locator, QTH, equipment, a short text, its roles and its gateway. The
operator writes it in the web interface. It has no pictures and no extra files, carries an
inline icon so that the browser does not ask for `/favicon.ico`, and goes out compressed when
the browser accepts that; a typical page is under 1 KB. `/station.json` gives the same to
programs. Through the local proxy a station's page is also reachable by its callsign:
`http://testb-1.axl/`.

## Mesh (relays)

`internal/mesh` lets stations reach others beyond their range, on the same frequency. Every
station is a relay by default (tag `RELAY`; "Relay" in the settings, `-relay=false` turns it off)
and repeats frames as an AX.25 digipeater: frames that name it as the next digipeater at once,
and beacons and CQ messages with the flood alias `AXHOP-2`, into whose path it writes its
callsign, after a random wait and only while a station it hears directly may not have them yet
(not their sender, not one of the relays that repeated them). Stations learn from the relayed
beacons over which relays another station is reached (`discovery`), the bridge and the chat
address their frames over that path, and acknowledgments and refusals go back the way their
frame came. Everything stays on layer 2: a relay routes no IP and passes nothing it repeats to
its own computer or network. The format: [protocol/mesh.md](protocol/mesh.md).

The lab `scripts/lab-mesh.sh` puts three stations in a line (A hears B, B hears C, A and C do
not hear each other) with B as the relay. In the F4HWN model with ACKMODE, in eleven runs on
2026-11-03 (up to five at the same time, all stations relaying in the last five), A and C found
each other over B in 35-74 s; ping took about 5 s per round trip, a small HTTP file 21-59 s, and
a chat message from A to C was delivered in all runs, mostly at the first try. The channel shows
collisions at B: A and C cannot hear each other's carrier (hidden stations), which AX.25 on one
frequency cannot prevent; TCP and the chat repeat what was lost. In one of the eleven runs an
HTTP download failed: A's and C's TCP repetitions kept colliding at B, in step, until C gave the
half-open connection up. Earlier runs showed what the beacon timing must cover: the first beacons of A and
C collided at B, and their answers, postponed by exactly 30 s, collided again; nobody would have
sent another beacon for ten minutes. Hence the random postponement, the relay's echo and asking
again (see the beacon timing).

## Packet monitor

`internal/capture` keeps the last 500 frames heard and sent, all of them, those for other
stations too, as with an open squelch, each with a short description: an IP packet with
addresses, ports and TCP flags, a beacon, an APRS message or acknowledgment, other APRS data.
The web interface shows them in the view "Monitor" ("Mithören"), filtered by all, this
station's, or other stations' frames. `-pcap FILE` writes every frame into a pcap file with the
link type AX.25, which Wireshark opens and decodes down to the IP packets; `-debug` writes every
frame into the log. Frames with a bad checksum never reach the host: the TNC drops them.

## Chat

`internal/chat` carries text messages in the APRS message format (APRS 1.01, chapter 14),
directly in AX.25 UI frames to `APZAXL` with PID `0xF0`: no IP, no TCP, no HTTP. A message is one
frame of about 100 bytes, its acknowledgment another; at 1200 bit/s that is under a second each.
Radios with APRS messaging take part as they are, for example the APRS messenger firmware for the
UV-K1/UV-K5 V3: its private messages, acknowledgments and lobby (CQ) messages are the same format.

- **To a station:** `:TESTB-1  :text{12`. The station answers `ack12`. Until then the message is
  sent again after 30, 60, 120 and 240 s (as the messenger does), one message per station at a
  time and in order. Without an answer it waits in the **mailbox** and goes out twice more each
  time a frame from that station is heard (beacon, IP, anything), for up to a week. A repeat
  that arrives twice is stored once and acknowledged again.
- **To everyone:** `:CQ       :text`, sent twice a minute apart, not acknowledged; QST and ALL
  arrive in the same room.
- **Text:** at most 67 characters per message, as APRS allows; a longer text goes out in up to
  five parts. Umlauts and other Latin letters are written in ASCII (ä: ae), as APRS radios show
  them; `|`, `~` and `{` become `-`.
- **Always reachable:** messages are not IP traffic, so neither the firewall nor the internet
  sharing affects them. Everyone on the frequency can read them: amateur radio is open.
- **Mailbox file:** `messages.json` next to `settings.json`, written atomically, readable by the
  owner only; at most 2000 messages are kept.

The web interface has a chat view: the room and one conversation per station, the state of
each message sent (sending, in the mailbox, delivered, not delivered), unread counts in the
tab. In a conversation with a gateway there is "Ask for internet access", which prepares a
message; the gateway's operator finds "Allow my gateway" (adds the callsign to the stations
allowed) and "Open a port…" in the conversation.

## Settings

What the web interface changes is stored in `settings.json` in the user's configuration folder
(`-config` chooses another file, `""` none): the choice of the gateway, sharing the internet,
the welcome page and the firewall. Changes take effect at once, without a restart: sharing
starts or stops the gateway and announces it in a beacon within seconds. The file is written
atomically and readable by the owner only. Options on the command line that cover the same
things (`-proxy`, `-proxy-lan`, `-gateway`, `-open`, `-firewall`) are stored like a change in the
web interface.

## Web interface

`internal/ui` serves one page on `127.0.0.1:7373` with two views. **Overview:** the live monitor,
this station, the stations heard with links to their welcome pages, the choice of the internet
gateway, the own gateway, the firewall with the packets it blocked, and the traffic counters.
**Settings:** sharing the internet, the welcome page and the firewall rules. The overview asks
`/api/status` every 2 seconds; `POST /api/gateway` and `POST /api/settings` change things. Page,
style and script are built into `axlinkd`, so it works on a computer without internet.

The live monitor (`internal/monitor`) takes a sample every 2 seconds and keeps 10 minutes:
bytes per second in both directions, the queue, and the airtime of the frames sent and heard,
estimated from their length. Two gauges show the channel load of the last minute (own and heard
airtime as a share of the time) and the transmit queue; a chart shows the throughput of the
last 5 minutes, smoothed over 6 seconds. Signals the radio cannot decode are not counted, so the
channel load is a lower bound. Costs: a few kilobytes of memory and one small request every 2
seconds while the page is open.

- Language: an explicit choice (`?lang=`, kept in a cookie), else the browser's language, else
  English (D7). Both languages have the same texts; a test checks it.
- Safety: everything that comes from the radio (callsigns, tags, addresses) is set as text,
  never as HTML; a content security policy allows only the page's own files. On a loopback
  address only loopback host names are accepted (against DNS rebinding); changes need JSON from
  the page's own origin (against requests from other web pages).

## Simulation and lab

- `internal/sim/f4hwnkiss`: a behavioural model of the F4HWN KISS app, written from its source,
  in virtual time.
- `internal/sim/channel`: a frame-level radio channel in real time: half duplex, collisions, a
  JSON trace; in front of each station either an ideal TNC or the F4HWN model (optionally with
  ACKMODE). The trace also records frames the model drops before the air. With `-link A-B`
  only the listed pairs hear each other (carrier sense and collisions too).
- `scripts/lab-netns.sh`: two stations in their own network namespaces with real kernel IP
  stacks. They find each other by beacons; then ping, UDP, HTTP and the internet through B as a
  gateway (a proxied request, a CONNECT tunnel, the own network refused), the web interface and
  the local proxy, B's welcome page, a refused port, B's port forwarding and a chat (a message
  each way, a CQ). Optionally with adapter MTU 576 as on Windows, which adds a fragmented ping
  and a 2 KB download. Also in CI. The model draws its p-persistence from a seeded random
  source, as the app does.
- `scripts/lab-mesh.sh`: three stations in a line with a relay in the middle (see Mesh).

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

- Gateway rules in the beacon, shrunk pages, a transparent userspace gateway (see Internet
  gateway).
- Chat in the APRS message format, so that messenger radios can take part.
- A transmit queue with priorities and duplicate suppression.
- A link gateway (AXUDP between radio islands) and an internet gateway in userspace (gVisor
  netstack); operation without admin rights through a local proxy.
- Mesh forwarding in userspace, independent of the operating system's forwarding.
- A KISS-TCP multiplexer, so that other programs can share the radio.

## Design rules

- Strict parsers; frames of other implementations serve as cross-checks.
- One process owns a serial port (it is opened exclusively).
- No legal restrictions built in (D2); technical channel protection can be switched off.
- Placeholder callsigns only (D3). No transmission in tests.

# Changes

## Unreleased

- Every station relays by default (switchable). A relay passes a flooded beacon or CQ message on
  only while a station it hears directly may not have it yet, after a random wait in which
  copies from other relays count too: no relay storm where everyone hears everyone, no repeats
  at the end of a line.
- Packet monitor: the view "Monitor" ("Mithören") shows the last 500 frames heard and sent, also
  those for other stations, with a short description; `-pcap FILE` writes every frame for
  Wireshark, `-debug` into the log.

## 0.2.0-alpha.1 — 2026-11-03

A new start after the review of 2026-11-02 ([REVIEW](docs/REVIEW_2026-11-02.md),
[DECISIONS](docs/DECISIONS.md)): the core in Go, licence GPL-3.0-or-later (protocol texts
CC BY 4.0), the repository in English. A prerelease: tested in the lab and in CI, not yet on
the air with a real radio.

- Packages for Linux (amd64, arm64, armv6), Windows (amd64, arm64; with `wintun.dll` 0.14.1
  and its licence, as D8 allows) and macOS (arm64, amd64), built by `.github/workflows/release.yml`
  from a tag. [QUICKSTART.md](docs/QUICKSTART.md): from the box to the first ping on each
  system.
- macOS: the network adapter as utun (wireguard-go), address, MTU and the subnet route set as
  wg-quick does. CI creates one on the macOS runner and pings through it.
- Chat (`internal/chat`): APRS messages directly in AX.25 UI frames, no IP, so the firewall and
  the sharing do not affect them. A message to a station is acknowledged and repeated after 30,
  60, 120 and 240 s, as the APRS messenger firmware does; unanswered it waits in the mailbox
  (`messages.json`) and goes out again when the station is heard, for up to a week. Messages
  to CQ (QST, ALL) reach every station, sent twice. A chat view in the web interface with
  "Ask for internet access" and, for the gateway's operator, "Allow my gateway" and "Open a
  port…". Lab: a message from A to B acknowledged at the first try in 3-5 s.
- Mesh (`internal/mesh`, [protocol/mesh.md](docs/protocol/mesh.md)): a station with "Relay"
  (`-relay`, tag `RELAY`) repeats frames as an AX.25 digipeater; beacons and CQ messages travel
  with the flood alias `AXHOP-2` over up to two relays, and stations learn from them over which
  relays another station is reached. `scripts/lab-mesh.sh`: three stations in a line; A and C
  found each other over B in 35-74 s (six runs), ping, HTTP and chat over B. Stations that do not hear each
  other collide at the relay (hidden stations); TCP and the chat repeat what is lost. Because the
  beacons of two hidden stations can collide at the relay too, a station sends a flooded beacon
  again when the relay's copy does not come back, asks again when its ask stays unanswered among
  known stations, and postpones answers by a random time, so that stations that collided once do
  not stay in step.
- Port forwarding (`internal/forward`): `-forward 2222=192.168.1.10:22=<CALL>` or the settings
  publish a TCP service of the own network on a port of the radio address; nothing else of the
  network becomes reachable. Lab: B's local server through B's port 8022.
- Firewall: refused packets get an answer at once (TCP reset, ICMP "administratively
  prohibited"), rate-limited; "silent" turns it off. In the lab a refused port fails in 2-4 s
  instead of a timeout. The welcome page lists the open ports.
- Locator: the welcome page's Maidenhead locator can be announced in the beacon (`loc=`,
  opt-in); the station list shows locator, distance and direction from the own locator.
- `sim-channel -link A-B`: who hears whom, carrier sense and collisions included.

- Licence: an additional permission (GPL-3.0 section 7) allows passing AXLink on together
  with `wintun.dll` (D8).
- `internal/kiss`: KISS framing with escapes, parameters and G8BPQ ACKMODE; fuzz test.
- `internal/ax25`: AX.25 v2.2 addresses and frames without FCS, the FCS (CRC-16/X.25). Cross-check
  with 123 frames that Packet.NET produced in the v0.1 lab: the same fields, re-encoded byte for
  byte.
- `internal/airtime`: bit stuffing and airtime.
- `internal/sim/f4hwnkiss`: a behavioural model of the KISS TNC app of the F4HWN firmware
  (256-byte ring, 16 bytes per 10 ms, one transmit slot, interrupts masked while sending),
  optionally with ACKMODE. It counts the frames it drops; the lab trace shows them (`drop`).
- `internal/pacing`: pacing per TNC profile. In the model a full packet without pacing is lost;
  with the profile `f4hwn-kisstnc` all arrive (~104 B/s). With a busy channel the estimate alone
  loses frames; the port then also uses the frames the radio heard (below), and ACKMODE avoids
  losses fully.
- `internal/discovery` and [docs/protocol/beacon.md](docs/protocol/beacon.md) (CC BY 4.0): station
  beacons with address and tags (`RELAY`, `INET`, `LINK`, `AUTO`), one after the start that asks
  for the others' beacons, then every 10 minutes; answers at most once in 30 s. Learned
  addresses become routes; configured ones for the exact address come first. `run -tags` and
  `-beacon`; `decode` shows beacon texts. The lab now runs without routes: the stations find each
  other in 3–5 s (model).
- `internal/firewall`, on by default: radio stations reach this computer only with answers to
  its own connections, pings (switchable) and ports opened by rules, optionally per callsign;
  everything else is dropped and listed. The bridge now passes only packets for the own address,
  so a radio station can no longer use a computer that forwards packets to get into other
  networks. `run -open`, `-firewall`.
- `internal/settings`: the settings the web interface changes are stored (`settings.json`,
  `-config`) and take effect without a restart; sharing the internet is switched on and off in
  the web interface and announced within seconds; the gateway can be open to listed callsigns
  only.
- `internal/welcome`: every station offers a small page on its radio address (name, locator,
  QTH, equipment, text, roles, gateway; under 1 KB, compressed) and `/station.json`. The local
  proxy reaches radio stations itself, also as `<callsign>.axl`.
- Live monitor (`internal/monitor`): gauges for the channel load and the transmit queue and a
  throughput chart of the last 5 minutes; the TNC port counts bytes and estimated airtime in both
  directions. The web interface gets a settings view.
- The lab opens B's welcome page over the radio, finds a closed port blocked, and takes
  screenshots of both interfaces and the welcome page.
- `internal/ui`: a web interface in English and German on `127.0.0.1:7373`: this station, the
  stations heard, the choice of the internet gateway, the own gateway and the traffic counters,
  refreshed every 3 s. Built in, no internet needed; radio data shown as text only; foreign host
  names and requests from other web pages refused. The lab checks it and can take screenshots
  with headless Chrome (`AXLINK_LAB_SCREENSHOTS`).
- Client side of the gateway: a local proxy on `127.0.0.1:7374` forwards HTTP and CONNECT to the
  chosen gateway over the radio (`off`, `auto` or one station; `-gateway`, or in the web
  interface). In the lab a file comes through it in 13-14 s, through CONNECT in 13-15 s (model).
  Proxy byte counters grow while a transfer runs; the TNC port reports whether it is connected.
- `internal/gateway`: the internet gateway. `run -proxy 8080` shares the station's internet
  connection as an HTTP proxy on its radio address (full URLs and CONNECT), resolves names on the
  gateway, refuses the operator's own network unless `-proxy-lan`, logs the client's callsign,
  and announces `tags=INET proxy=8080` in the beacon. In the lab a client fetches a file from an
  address it has no route to in 11-13 s through the proxy and in 16-18 s through CONNECT (model).
- Pacing: before each frame the sender checks the previous one again against the frames heard
  meanwhile; ACKMODE uses PERSIST 63 so that two waiting stations do not start in the same
  slot; the app model draws its p-persistence from a random source. Six labs at once: no frame
  dropped, none lost on the air.
- `internal/txqueue`: the transmit queue. Packets of up to 128 bytes go first (at most three in
  a row while a larger one waits, never ahead of their own flow); a TCP retransmission replaces
  its waiting original or is dropped if the original has just gone out; a newer pure ACK
  replaces a waiting older one. In the lab HTTP takes 9 s instead of 13 s, and the repeated SYN,
  which used to cost the original app a dropped frame, stays off the air.
- `internal/tnc`: the connection to a TNC over serial (DTR set, exclusive), KISS-TCP or a Unix
  socket. Reconnects by itself after USB resets, then sends a resync and the KISS parameters
  again and repeats an interrupted frame once. Finds the radio by its USB ID 36B7:FFFF. Waits for
  ACKMODE echoes. Without ACKMODE it waits longer when the radio heard another station before its
  own frame could have gone out; in the lab the original app then drops nothing in a 2 KB
  download (39 s instead of 110 s).
- `internal/tun`: the network adapter on Linux (wireguard-go without GSO offloads), IPv6 off on
  the adapter; on Windows through Wintun with a fixed GUID per name, address, routes and MTU
  through the IP Helper API. `scripts/fetch-wintun.ps1` fetches `wintun.dll` with a checksum
  check, for builds from source.
- `internal/ipbridge`: IPv4 ↔ AX.25 UI/PID 0xCC with a destination filter, length and checksum
  checks, longest-prefix routes and a switchable broadcast filter. When the adapter MTU is larger
  than the radio's (Windows: at least 576), it lowers the TCP MSS in both directions and
  fragments larger packets without DF (RFC 791); larger packets with DF are dropped.
- `internal/sim/channel` and `scripts/lab-netns.sh`: two stations in their own network
  namespaces without root or Docker. With the model of the F4HWN app: ping 3/3 (RTT ~2.1 s),
  UDP 1.7 s, HTTP 9 s, nothing lost on the air or in the app. With adapter MTU 576 as on
  Windows also a 400-byte ping without DF (2/2, RTT ~10 s) and a 2 KB download: 39 s with the
  original app, 38 s with ACKMODE, no frame dropped. All from the model, not hardware.
- Finding in the app's source: a frame cut off by a USB reset would be closed by the next FEND
  and sent truncated. The f4hwn profile therefore sends `FESC 00 FEND` on connecting.
- `cmd/axlinkd`: `run`, `ports`, `decode`, `sim-channel`, `version`. `run -mtu` sets the radio
  MTU (default 256), `-adapter-mtu` the adapter's.
- Documentation: architecture, decisions, development, TODO list, history of v0.1, Multiboot
  guide until F4HWN v7.
- CI on Linux, Windows and macOS; builds also for ARMv6 (Pi Zero W). In CI also the lab, a real
  TUN adapter in a user namespace and, on Windows, a real Wintun adapter with a ping through it
  (on the GitHub runner Windows refuses an MTU of 256 and keeps 576).
- `scripts/setup-go.sh` installs Go into the project with a checksum check.

## 0.1.0-alpha.1 — 2026-10-31

The first runnable simulator state (Codex). Ordinary UDP and HTTP applications reached a second
private IP address over two real TUN/pdn-net/pdn process chains and the virtual AFSK1200 channel.
With the channel interrupted the requests failed; after it resumed they worked again. AX.25
UI/PID 0xCC traces showed the unchanged IP packets on both sides.

- Pinned upstreams and SDK, a reproducible audit runner without hardware tests.
- Packet.NET: 13,295 tests passed, 48 skipped; pdn-net: 40 passed.
- F1 receive defect reproduced; a local patch passes all 49 extended tests.
- An isolated Docker lab with clean-up, raw frames, route evidence and negative controls.
- Eleven Python tests for audit guardrails, bounded UDP retries and the evaluation of traces.

Experimental: no hardware approval, no Windows/Wintun, no chat or mesh. Details:
[HISTORY.md](docs/HISTORY.md).

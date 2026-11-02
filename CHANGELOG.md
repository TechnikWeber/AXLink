# Changes

## Unreleased (0.2.0-dev)

A new start after the review of 2026-11-02 ([REVIEW](docs/REVIEW_2026-11-02.md),
[DECISIONS](docs/DECISIONS.md)): the core in Go, licence GPL-3.0-or-later (protocol texts
CC BY 4.0), the repository in English.

- `internal/kiss`: KISS framing with escapes, parameters and G8BPQ ACKMODE; fuzz test.
- `internal/ax25`: AX.25 v2.2 addresses and frames without FCS, the FCS (CRC-16/X.25). Cross-check
  with 123 frames that Packet.NET produced in the v0.1 lab: the same fields, re-encoded byte for
  byte.
- `internal/airtime`: bit stuffing and airtime.
- `internal/sim/f4hwnkiss`: a behavioural model of the KISS TNC app of the F4HWN firmware
  (256-byte ring, 16 bytes per 10 ms, one transmit slot, interrupts masked while sending),
  optionally with ACKMODE. It counts the frames it drops; the lab trace shows them (`drop`).
- `internal/pacing`: pacing per TNC profile. In the model a full packet without pacing is lost;
  with the profile `f4hwn-kisstnc` all arrive (~104 B/s); with a busy channel only ACKMODE avoids
  losses.
- `internal/tnc`: the connection to a TNC over serial (DTR set, exclusive), KISS-TCP or a Unix
  socket. Reconnects by itself after USB resets, then sends a resync and the KISS parameters
  again and repeats an interrupted frame once. Finds the radio by its USB ID 36B7:FFFF. Waits for
  ACKMODE echoes.
- `internal/tun`: the network adapter on Linux (wireguard-go without GSO offloads), IPv6 off on
  the adapter; on Windows through Wintun with a fixed GUID per name, address, routes and MTU
  through the IP Helper API. `scripts/fetch-wintun.ps1` fetches `wintun.dll` (not shipped) with a
  checksum check. macOS follows.
- `internal/ipbridge`: IPv4 ↔ AX.25 UI/PID 0xCC with a destination filter, length and checksum
  checks, longest-prefix routes and a switchable broadcast filter. When the adapter MTU is larger
  than the radio's (Windows: at least 576), it lowers the TCP MSS in both directions and
  fragments larger packets without DF (RFC 791); larger packets with DF are dropped.
- `internal/sim/channel` and `scripts/lab-netns.sh`: two stations in their own network
  namespaces without root or Docker. With the model of the F4HWN app: ping 3/3 (RTT ~2.1 s),
  UDP 1.7 s, HTTP 13 s, no frame lost (model, not hardware). With adapter MTU 576 as on
  Windows also a 400-byte ping without DF (2/2, RTT ~10 s) and a 2 KB download: 111 s with the
  original app, which drops 7 frames before the air (slot busy, USB ring overflow), 44 s with
  ACKMODE without a drop (model).
- Finding in the app's source: a frame cut off by a USB reset would be closed by the next FEND
  and sent truncated. The f4hwn profile therefore sends `FESC 00 FEND` on connecting.
- `cmd/axlinkd`: `run`, `ports`, `decode`, `sim-channel`, `version`. `run -mtu` sets the radio
  MTU (default 256), `-adapter-mtu` the adapter's.
- Documentation: architecture, decisions, development, TODO list, history of v0.1, Multiboot
  guide until F4HWN v7.
- CI on Linux, Windows and macOS; builds also for ARMv6 (Pi Zero W). In CI also the lab, a real
  TUN adapter in a user namespace and, on Windows, a real Wintun adapter with a ping through it.
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

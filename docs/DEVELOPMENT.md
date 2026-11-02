# Development (Go core)

## Setup

```sh
bash scripts/setup-go.sh                 # Go as in go.mod into .tools/go, checksum from go.dev
export PATH="$PWD/.tools/go/bin:$PATH"
go test ./...                            # all tests
go vet ./...
go build -o axlinkd ./cmd/axlinkd
```

A system-wide Go of the right version works as well.

## Program

```text
axlinkd run          connect a radio or TNC, open the network adapter, carry IPv4
axlinkd ports        serial ports; marks radios with F4HWN firmware (USB 36B7:FFFF)
axlinkd decode       a KISS stream on stdin as JSON per frame
axlinkd sim-channel  lab only: a simulated radio channel
```

Example with a radio (Linux; the callsigns are placeholders, enter your own):

```sh
./axlinkd ports
sudo ./axlinkd run -call <CALL>-1 -port auto -ip 10.73.0.1/24 -route 10.73.0.2=<PEER>-1
```

- `-port auto` picks the one connected F4HWN radio; otherwise `-port /dev/ttyACM0`. Other TNCs:
  `-port` with `-profile generic-kiss`, or `-kiss-tcp host:port` (Dire Wolf).
- The network adapter (`axl0`, MTU 256) needs root or `CAP_NET_ADMIN`. Instead of `sudo`,
  `sudo setcap cap_net_admin+ep ./axlinkd` should work; that variant is not tested yet.
- The serial port needs read and write access; on Fedora the group `dialout`.
- After a USB reset (RF on the cable) AXLink reconnects by itself and sends the KISS parameters
  again. A frame that did not get through completely is sent once more.
- Without a callsign, or with a placeholder (`N0CALL`), `run` does not start.
- Broadcast and multicast are kept off the air; `-allow-broadcast` switches the filter off.
- Windows and macOS: serial ports work, the network adapter follows (Wintun, utun).

Pacing per TNC: `-profile f4hwn-kisstnc` (default for serial ports), `f4hwn-kisstnc-ackmode`
(for our own app with ACKMODE, AXLink TNC), `generic-kiss` (default otherwise).

## Lab without radios

```sh
bash scripts/lab-netns.sh                 # model of the F4HWN KISS app, profile f4hwn-kisstnc
bash scripts/lab-netns.sh f4hwn-ackmode   # the same with ACKMODE
bash scripts/lab-netns.sh ideal           # a TNC with a queue
```

Two stations in their own user and network namespaces (no root, no Docker) with real kernel IP
stacks and TUN devices, connected through `axlinkd sim-channel`. The lab checks ping, UDP and
HTTP from A to B. The channel trace must show IPv4 frames (UI, PID 0xCC) in both directions.
Needs `unshare`, `ip`, `python3`, `curl`, `ping`. Results from the model are not hardware
results.

## Layout

| Path | Content |
|---|---|
| `cmd/axlinkd` | the program |
| `internal/kiss` | KISS framing, parameters, ACKMODE (G8BPQ) |
| `internal/ax25` | AX.25 v2.2 addresses and frames (without FCS, as over KISS), FCS |
| `internal/airtime` | bit stuffing and airtime of a frame |
| `internal/pacing` | how AXLink feeds a TNC (profiles) |
| `internal/tnc` | the connection to a TNC: serial, KISS-TCP, Unix socket; reconnecting; paced sending |
| `internal/tun` | the virtual network adapter (Linux; Windows and macOS follow) |
| `internal/ipbridge` | IPv4 ↔ AX.25 UI/PID 0xCC, destination filter, routes, broadcast filter |
| `internal/sim/f4hwnkiss` | behavioural model of the KISS TNC app of the F4HWN firmware |
| `internal/sim/channel` | radio channel for the lab (half duplex, collisions, trace) |

The v0.1 lab with pdn/pdn-net (Python, Docker, `scripts/*.py`, `simulator/`, `tests/`) is
described in [HISTORY.md](HISTORY.md). Its Python tests still run in CI.

## Rules

- Everything in the repository is English (D7).
- Every new source file starts with `// SPDX-License-Identifier: GPL-3.0-or-later`.
- Placeholder callsigns only (`N0CALL`, `TESTA-1` …), never real ones (D3).
- No legal restrictions built in (D2); technical channel protection filters can be switched off.
- Results from the model or the simulator are not hardware results. Hardware measurements are
  documented separately.
- Parsers are strict: what we accept, we encode the same way again. Test vectors of other
  implementations (e.g. `internal/ax25/testdata`) serve as a cross-check.

## CI

`.github/workflows/ci.yml`:
- Linux: gofmt, vet, tests with `-race`, builds for Linux amd64/arm64/ARMv6, Windows amd64/arm64
  and macOS amd64/arm64, the Python tests of the v0.1 lab.
- Lab: `lab-netns.sh` in the modes `f4hwn` and `f4hwn-ackmode`.
- Windows and macOS tests.

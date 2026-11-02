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
- The network adapter (`axl0`) needs root or `CAP_NET_ADMIN`. Instead of `sudo`,
  `sudo setcap cap_net_admin+ep ./axlinkd` should work; that variant is not tested yet.
- `-mtu` is the largest IP packet on the radio (default 256, the classic AX.25 information
  field). The adapter gets the same MTU unless `-adapter-mtu` says otherwise; Windows needs at
  least 576. With a larger adapter MTU the bridge lowers the TCP MSS to fit the radio and
  fragments larger packets without DF; packets with DF that do not fit are dropped (counter
  `TooBig` in the statistics).
- The serial port needs read and write access; on Fedora the group `dialout`.
- After a USB reset (RF on the cable) AXLink reconnects by itself and sends the KISS parameters
  again. A frame that did not get through completely is sent once more.
- Without a callsign, or with a placeholder (`N0CALL`), `run` does not start.
- Broadcast and multicast are kept off the air; `-allow-broadcast` switches the filter off.
- macOS: serial ports work, the network adapter (utun) follows.

Windows, in a PowerShell started as administrator (the adapter needs it):

```powershell
# wintun.dll next to axlinkd.exe; Bypass lets just this script run
powershell -ExecutionPolicy Bypass -File .\scripts\fetch-wintun.ps1 -Destination C:\AXLink
C:\AXLink\axlinkd.exe ports
C:\AXLink\axlinkd.exe run -call <CALL>-1 -port auto -ip 10.73.0.1/24 -route 10.73.0.2=<PEER>-1
```

- AXLink does not ship `wintun.dll`; the script downloads Wintun 0.14.1 from wintun.net, checks
  its SHA-256 checksum and puts the DLL for this Windows's architecture (or `-Arch amd64|arm64`,
  matching `axlinkd.exe`) and its licence into the folder.
- The adapter keeps a fixed GUID per name, so Windows recognises the network again.
- Windows normally treats a new network like this one as public: its firewall then blocks
  incoming connections there (pings to this station, servers on it). Outgoing connections and
  their answers pass. A rule per interface opens what is needed, e.g.
  `New-NetFirewallRule -DisplayName "AXLink ping" -Direction Inbound -InterfaceAlias axl0 -Protocol ICMPv4 -IcmpType 8 -Action Allow`
  (not tested yet).
- Windows has been tested so far only in CI (a real Wintun adapter on the GitHub runner, a ping
  through it), not yet with a radio.

Pacing per TNC: `-profile f4hwn-kisstnc` (default for serial ports), `f4hwn-kisstnc-ackmode`
(for our own app with ACKMODE, AXLink TNC), `generic-kiss` (default otherwise).

## Lab without radios

```sh
bash scripts/lab-netns.sh                 # model of the F4HWN KISS app, profile f4hwn-kisstnc
bash scripts/lab-netns.sh f4hwn-ackmode   # the same with ACKMODE
bash scripts/lab-netns.sh ideal           # a TNC with a queue
bash scripts/lab-netns.sh f4hwn 576       # adapter MTU 576 as on Windows
```

Two stations in their own user and network namespaces (no root, no Docker) with real kernel IP
stacks and TUN devices, connected through `axlinkd sim-channel`. The lab checks ping, UDP and
HTTP from A to B. The channel trace must show IPv4 frames (UI, PID 0xCC) in both directions.
The result also lists frames the TNC model threw away before they reached the air (`drop` in
the trace): `tnc-busy` when a frame arrived while another still waited for the channel,
`usb-ring` when the 256-byte USB ring overflowed. The radio itself has no counter for either.
With an adapter MTU as second argument it also sends a 400-byte ping without DF and downloads
2 KB, and station A must have lowered an MSS and fragmented a packet. Needs `unshare`, `ip`,
`python3`, `curl`, `ping`. Results from the model are not hardware results.

The adapter itself: `AXLINK_TUN_TEST=1` lets `internal/tun` create a real adapter and ping
through it. On Linux inside a user namespace, without root:

```sh
go test -c -o /tmp/tun.test ./internal/tun
AXLINK_TUN_TEST=1 unshare -Urn sh -c 'ip link set lo up && /tmp/tun.test -test.v -test.run TestAdapterCarriesPing'
```

On Windows as administrator, with `wintun.dll` next to the test binary (`go test -c`, then
`fetch-wintun.ps1 -Destination` its folder).

## Layout

| Path | Content |
|---|---|
| `cmd/axlinkd` | the program |
| `internal/kiss` | KISS framing, parameters, ACKMODE (G8BPQ) |
| `internal/ax25` | AX.25 v2.2 addresses and frames (without FCS, as over KISS), FCS |
| `internal/airtime` | bit stuffing and airtime of a frame |
| `internal/pacing` | how AXLink feeds a TNC (profiles) |
| `internal/tnc` | the connection to a TNC: serial, KISS-TCP, Unix socket; reconnecting; paced sending |
| `internal/tun` | the virtual network adapter (Linux: TUN; Windows: Wintun; macOS follows) |
| `internal/ipbridge` | IPv4 ↔ AX.25 UI/PID 0xCC, destination filter, routes, broadcast filter, MSS clamping, fragmentation |
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
- Lab: the adapter test in a user namespace; `lab-netns.sh` in the modes `f4hwn` and
  `f4hwn-ackmode`, and `f4hwn` with adapter MTU 576.
- Windows: tests, then a real Wintun adapter (the runner has administrator rights) with
  `wintun.dll` from `fetch-wintun.ps1` and a ping through it.
- macOS: tests.

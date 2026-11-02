# AXLink — IP over amateur packet radio

<p>
  <img alt="Status: alpha" src="https://img.shields.io/badge/status-alpha-orange">
  <a href="https://github.com/TechnikWeber/AXLink/releases"><img alt="Latest release" src="https://img.shields.io/github/v/release/TechnikWeber/AXLink?include_prereleases&label=release"></a>
  <a href="https://github.com/TechnikWeber/AXLink/actions/workflows/ci.yml"><img alt="tests" src="https://img.shields.io/github/actions/workflow/status/TechnikWeber/AXLink/ci.yml?branch=main&label=tests"></a>
  <img alt="Platform: Linux (Windows, macOS planned)" src="https://img.shields.io/badge/platform-Linux%20%28Windows%2C%20macOS%20planned%29-blue">
  <img alt="Radio: any KISS TNC, UV-K1 | UV-K5 V3" src="https://img.shields.io/badge/radio-KISS%20TNC%20%7C%20UV--K1%20%7C%20UV--K5%20V3-blue">
  <img alt="UI: English (German planned)" src="https://img.shields.io/badge/UI-English%20%28German%20planned%29-informational">
  <a href="LICENSE"><img alt="License: GPL-3.0-or-later" src="https://img.shields.io/badge/license-GPL--3.0--or--later-green"></a>
</p>

**Ordinary network programs over a handheld radio: ping, curl and small web servers at
1200 baud.**

AXLink connects a KISS radio or TNC, opens a virtual network adapter and sends IP packets as
AX.25 UI frames (PID `0xCC`), the framing that Linux, JNOS, BPQ and Packet.NET use. Programs work
unchanged, only slowly.

It is tuned first to the Quansheng UV-K1 / UV-K5 V3 with the F4HWN firmware and its KISS TNC
app, and works with any KISS TNC (Dire Wolf, hardware TNCs, KISS over TCP).

> **Status: alpha (0.2.0-dev).** Linux works in the lab: two stations, a simulated channel,
> real kernel IP stacks. Real radios, Windows and macOS come next. Nothing has been on the air
> yet.

## What works

- `axlinkd run`: a radio over USB (F4HWN radios are found by their USB ID `36B7:FFFF`), KISS
  over TCP or a Unix socket. It reconnects after USB resets, paces frames per TNC profile and
  supports ACKMODE.
- A Linux network adapter `axl0` (MTU 256) and IPv4 over AX.25 UI/PID `0xCC`, with a destination
  filter, checksum checks, routes and a broadcast filter.
- A lab without radios: `scripts/lab-netns.sh` runs two stations in their own network namespaces
  (no root, no Docker) and checks ping, UDP and HTTP through a simulated channel with a model of
  the F4HWN KISS app. It also runs in CI.

## Quick start

```sh
bash scripts/setup-go.sh                 # Go into .tools/go
export PATH="$PWD/.tools/go/bin:$PATH"
go test ./...
bash scripts/lab-netns.sh                # the two-station lab
go build -o axlinkd ./cmd/axlinkd
./axlinkd ports
sudo ./axlinkd run -call <CALL>-1 -port auto -ip 10.73.0.1/24 -route 10.73.0.2=<PEER>-1
```

Use your own callsign; placeholders such as `N0CALL` are refused. Every operator is
responsible for what is allowed on their frequency; AXLink does not restrict traffic.

## Radio firmware

- F4HWN Labs v6.3.0 with its KISS TNC app works today.
- [AXLink TNC](https://github.com/TechnikWeber/AXLink-TNC): our variant of the KISS app with
  extensions for hosts such as AXLink (in progress). Runs on the original Labs v6.3.0.
- Messenger and KISS TNC on one radio until F4HWN v7: [MULTIBOOT_V6.md](docs/MULTIBOOT_V6.md).

## Documents

| Document | Content |
|---|---|
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | how AXLink is built |
| [DECISIONS.md](docs/DECISIONS.md) | the decisions; they take precedence |
| [DEVELOPMENT.md](docs/DEVELOPMENT.md) | build, test, run, lab |
| [TODO.md](docs/TODO.md) | open points |
| [MULTIBOOT_V6.md](docs/MULTIBOOT_V6.md) | messenger and KISS TNC via Multiboot (until v7) |
| [REVIEW_2026-11-02.md](docs/REVIEW_2026-11-02.md) | the review that led to the Go rewrite |
| [HISTORY.md](docs/HISTORY.md) | the v0.1 prototype (pdn/pdn-net) and its findings |
| [CHANGELOG.md](CHANGELOG.md) · [LICENSING.md](LICENSING.md) | changes · licences |

## Licence

GPL-3.0-or-later ([LICENSE](LICENSE)); protocol specifications CC BY 4.0. Third-party licences:
[LICENSING.md](LICENSING.md).

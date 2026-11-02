# AXLink — instructions for coding agents

Read [docs/DECISIONS.md](docs/DECISIONS.md) first; it takes precedence over everything else.
Then [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)
and [docs/TODO.md](docs/TODO.md).

## Mission

Ordinary IPv4 programs over KISS radios and TNCs, as one portable Go program (`axlinkd`):
Linux first, then Windows and macOS. Tuned first to the Quansheng UV-K1 / UV-K5 V3 with the
F4HWN KISS TNC app; any standard KISS TNC must keep working. Later: station discovery, chat,
gateways, mesh.

## Rules

- **English everywhere in the repository:** code, comments, documentation, commit messages,
  command-line output. The planned web UI offers English and German (D7).
- Small, tested steps. Run `gofmt`, `go vet ./...` and `go test -race ./...` before committing;
  run `scripts/lab-netns.sh` when the data path changes.
- Never report a test, a lab run or hardware compatibility as passed unless it ran. Results from
  the model or the simulator are not hardware results.
- Every new source file starts with `// SPDX-License-Identifier: GPL-3.0-or-later`.
- Placeholder callsigns only (`N0CALL`, `TESTA-1` …), never a real one (D3).
- No legal restrictions built into the software; operators are responsible for their frequency
  (D2). Technical channel protection (broadcast filters, airtime fairness) stays switchable.
- No transmission from tests; labs use simulated channels only.
- One process owns a serial port. Open only the configured port or an auto-detected F4HWN radio
  (USB `36B7:FFFF`), never other serial devices.
- Keep AX.25 on-air IPv4 in the standard framing (UI frames, PID `0xCC`); own wire formats only
  as documented extensions (specifications under CC BY 4.0).
- Parsers are strict; use frames of other implementations as cross-checks.
- Respect upstream licences: Packet.NET and pdn-net (AGPL) are test peers only; F4HWN code is
  Apache-2.0.
- Keep `docs/TODO.md` and `CHANGELOG.md` current with each change.

# AXLink — history

## Planning (October 2026)

AXLink began as a planning conversation (working title "RadioMesh"), summarised in German
handoff documents for Codex and Claude Code: ordinary IP applications over KISS radios, station
and service discovery, chat, small radio web pages, mesh routing, optional permanent nodes on a
Raspberry Pi. The originals are kept in tag `v0.1.0-alpha.1`:

- planning documents: <https://github.com/TechnikWeber/AXLink/tree/v0.1.0-alpha.1/docs>
  (`PROJECT_HANDOFF`, `ARCHITECTURE`, `ROADMAP`, `RESEARCH`, `OPEN_QUESTIONS`, `START_PROMPT`)
- the original handoff: <https://github.com/TechnikWeber/AXLink/tree/v0.1.0-alpha.1/archive>

## v0.1.0-alpha.1 (2026-10-31, Codex)

A prototype on Packet.NET, published as
[v0.1.0-alpha.1](https://github.com/TechnikWeber/AXLink/releases/tag/v0.1.0-alpha.1):

- Packet.NET `bded4671` and pdn-net `91c412c9` pinned, with an audit runner, source checksums
  and a licence inventory. Packet.NET: 13,295 tests passed, 48 skipped; pdn-net: 40 passed.
- A receive destination filter for pdn-net (finding F1): 49 tests passed with the patch.
- A Docker lab: two containers, each with pdn (the Packet.NET node, owner of the KISS port) and
  pdn-net (TUN ↔ RHPv2), connected through net-sim (an AFSK1200 audio-domain simulator). UDP and
  HTTP worked in both directions, failed while the channel was paused and worked again after it
  resumed. The IP packets appeared unchanged as AX.25 UI/PID 0xCC on both sides. 3 of 4 runs
  passed; UDP needed up to 3 tries, a tiny HTTP GET 9–31 s.

The German audit and lab reports (`UPSTREAM_AUDIT`, `TWO_NODE_DEMO`, `SIMULATOR_PLAN`) are in
the same tag under `docs/`.

## Findings about the upstreams (still valid)

| # | Finding |
|---|---|
| F1 | pdn-net (`IpAx25Bridge`) writes PID 0xCC frames addressed to other stations to the TUN: RHP custom subscriptions are promiscuous and the bridge does not check the destination. Reproduced with tests; patch in `patches/pdn-net-destination-filter.patch`. |
| F2 | pdn-net binds RHP without a port; with several ports, sending may use the first one while receiving listens on all. |
| F3 | Received IP goes to the TUN without much validation; disconnects, back-pressure and malformed packets are untested. |
| F4 | Segmentation (PID 0x08): pdn-net's reassembler reads bits 7–1 as the segment number and bit 0 as "last"; AX.25 2.2 figure 6.2 and Packet.NET's segmenter use bit 7 as "first" and bits 6–0 as the remaining count. The two do not interoperate. |
| F5 | `Ipv4Fragmenter.Fragment` ignores DF and an existing fragment offset, and copies all options into every fragment. |

F1 and F4 should still be reported to pdn-net (see [TODO.md](TODO.md)). AXLink's own bridge
filters by destination from the start (F1).

## Why the rewrite

The review of 2026-11-02 ([REVIEW_2026-11-02.md](REVIEW_2026-11-02.md)) found the prototype's
base unsuited to the product: a chain of three processes, a TUN on Linux only, young
dependencies, and a simulator that hid the limits of the radio's KISS app. The core has been
rewritten in Go since (decision D1 in [DECISIONS.md](DECISIONS.md)).

## What is still in the repository from v0.1

- `scripts/*.py`, `scripts/setup-host.sh`, `simulator/`, `tests/`, `tools/TraceCheck/`,
  `patches/`, `upstream.lock.json` and the evidence under `artifacts/`.
- It needs the .NET SDK 10.0.107, Docker and the pinned upstream checkouts
  (`bash scripts/setup-host.sh`, then `python3 scripts/test-two-nodes.py`). CI runs its Python
  unit tests.
- `internal/ax25/testdata` uses frames from `artifacts/m1/evidence` as interop vectors.

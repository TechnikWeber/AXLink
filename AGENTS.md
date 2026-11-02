# AXLink — Agent Instructions (Codex / other coding agents)

## Mission

Build an open, portable AX.25/KISS/IP radio mesh platform. Reuse rather than rewrite existing Packet.NET/pdn-net components. **Normal IP applications must keep working.** See `README.md`, `docs/PROJECT_HANDOFF.md`, `docs/ARCHITECTURE.md`, `docs/ROADMAP.md`, `docs/RESEARCH.md` and `docs/OPEN_QUESTIONS.md` before editing code.

## Current state

The original handoff contained documentation only. The current repository also
contains an executed upstream audit, a tested pdn-net destination-filter patch,
and an isolated two-node TUN/RHPv2/KISS/audio-simulator UDP/HTTP demo with trace
evidence and interruption/recovery checks. See `docs/TWO_NODE_DEMO.md` and
`docs/UPSTREAM_AUDIT.md`. Hardware, Windows, discovery/chat and mesh remain
unverified. Do not extrapolate simulator results into hardware claims.

## Priorities

1. Inspect upstream repositories, pin exact commits and execute builds/tests.
2. Validate Packet.NET/pdn-net/RHPv2 architecture and licenses.
3. Build a fully simulated two-node KISS/AX.25/IP test before changing device hardware.
4. Add direct Linux then Windows support, then discovery, chat, actual radio tests, then mesh.
5. Propose small upstream patches where possible; fork only as needed.

## Required engineering discipline

- **Do not** implement a new AX.25 stack, KISS protocol, TCP/IP stack or cryptography unless justified after code audit.
- Prefer C#/.NET 10 core with clean adapters; Linux TUN and Windows Wintun as separate OS implementations.
- Keep AX.25 on-air IPv4 in documented standard framing (UI/PID 0xCC); introduce any custom native app wire format only via a documented protocol extension.
- Separate *verified* facts, hypotheses and design proposals; write tests, especially real on-air interoperability and multi-hop packet forwarding.
- Ensure one process exclusively owns the USB serial/KISS port; design multiplexing explicitly.
- Simulate packet loss, delay, half-duplex collision and no direct A-C connectivity.
- Avoid broadcast storms on 1200 bit/s. Filter unrelated IPv6/mDNS/NetBIOS/etc traffic from radio unless specifically implemented.
- **No live amateur-radio TX by default.** Require explicit operator setup and regulatory compliance; do not send ordinary encrypted payloads over German amateur radio.
- No uncontrolled general-purpose internet routing, transparent public 44Net address assignment, or arbitrary privileged commands.
- Follow upstream copyright/license and AGPL/GPL obligations; record source licenses and modifications.
- Keep user-facing instructions in German by default; code identifiers can be English.
- Document exact commands and results in `docs/UPSTREAM_AUDIT.md` when phase M0 begins.

## Definition of done for first autonomous task

A reproducible **audit and simulator plan**, backed by code inspection and executed upstream tests where runtime/network access permits. Deliver precise gaps, proposed minimal additions, commands, and evidence. If blocked, document the blocker and create the smallest useful local test scaffold; do not fabricate green tests.

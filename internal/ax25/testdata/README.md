# Test vectors from Packet.NET

`packetnet-frames.jsonl` holds raw AX.25 frames (without FCS) that the Packet.NET node (`pdn`,
commit `bded4671`) sent and received in the two-node lab of 2026-10-31 (`pdn traffic export`,
station A). `packetnet-decoded.jsonl` is the same list, line by line, decoded by the strict
Packet.NET parser (`tools/TraceCheck`).

Source: `artifacts/m1/evidence/a-ax25.jsonl` and `a-decoded.jsonl` (tag `v0.1.0-alpha.1`). They
serve as a cross-check: our parser must read the same fields and re-encode the frames byte for
byte.

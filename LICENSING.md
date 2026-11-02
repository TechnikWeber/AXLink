# Licensing

## AXLink's own licence (since 2026-11-02)

- **Code** (Go core, scripts, tests, tools): **GPL-3.0-or-later**, text in [LICENSE](LICENSE).
  New source files start with `SPDX-License-Identifier: GPL-3.0-or-later`.
- **Protocol specifications** (in future under `docs/protocol/`): **CC BY 4.0**, text in
  [licenses/CC-BY-4.0.txt](licenses/CC-BY-4.0.txt). Any project can implement the protocol
  freely, firmware under Apache-2.0 included.
- Other documentation: like the code, unless stated otherwise.
- Tag `v0.1.0-alpha.1` predates this and was a private prerelease without a licence grant.
- Third-party code keeps its licence (tables below).

## Third-party sources included, and changes

| Path | Origin / licence |
|---|---|
| `artifacts/m0/sources/packet.net`, `references/0.txt`, `references/1.txt` | Packet.NET; AGPL-3.0-or-later according to its build metadata; full licence text in the source folder |
| `artifacts/m0/sources/pdn-net` | pdn-net; AGPL-3.0-or-later; full licence text in the source folder |
| `patches/pdn-net-destination-filter.patch` | changes to the pinned pdn-net; the original AGPL licence applies; change and tests described in `patches/README.md` |
| `artifacts/m0/firmware/kisstnc_app.c` | Johan Denoyer F4WAT / Armel F4HWN; Apache-2.0 according to the file header; licence text in `licenses/Apache-2.0.txt` |
| `artifacts/m0/references/4.txt` | LinBPQ/BPQ32, John Wiseman G8BPQ; GPL-3.0-or-later according to the file header; licence text in `licenses/GPL-3.0.txt` |
| `artifacts/m0/references/3.txt` | tncattach README; project licence MIT, text in `licenses/tncattach-MIT.txt` |
| `artifacts/m0/references/5.txt` | mod-orphan README; declared GPL-2.0, text in `licenses/GPL-2.0.txt` |
| `internal/ax25/testdata/packetnet-*.jsonl` | AX.25 frames that Packet.NET sent and received in our own lab (test data, not code) |

Source URLs and audited commit IDs are in `upstream.lock.json`. The saved source copies were not
changed; their checksums are in `artifacts/m0/source-sha256.json`. For the older alternative
references, branch URLs rather than reproducible commit pins are recorded. The tncattach licence
was retrieved on 2026-10-31 from
`https://raw.githubusercontent.com/markqvist/tncattach/master/LICENSE`.

`internal/sim/f4hwnkiss` is our own behavioural model of the KISS TNC app, written from its
source; it contains no firmware code.

## Go dependencies

According to `go.mod`; all compatible with GPL-3.0.

| Module | Licence | Used for |
|---|---|---|
| `go.bug.st/serial` v1.8.0 | BSD-3-Clause | serial ports, USB IDs |
| `golang.zx2c4.com/wireguard` (commit `2631ce99a06f`) | MIT | network adapter (only the `tun` package) |
| `golang.zx2c4.com/wintun` | MIT | Windows adapter (in future; `wintun.dll` is not shipped yet) |
| `golang.org/x/sys`, `golang.org/x/net` | BSD-3-Clause | system calls |

Planned: gVisor netstack (Apache-2.0) for gateways and for running without admin rights.

## Tools of the v0.1 lab

Packet.NET and pdn-net are built from pinned full repositories. The trace adapter
`tools/TraceCheck` links Packet.NET; a binary distribution would have to meet its AGPL terms.
No upstream binaries or images are shipped.

net-sim and the Microsoft ASP.NET runtime image are pulled separately with Docker; digests and
metadata are recorded. The pinned net-sim image's label says MIT, while the repository
`packethacking/net-sim` is now AGPL-3.0. We use net-sim only as a separate test tool and do not
distribute it. `scripts/inventory-package-licenses.py` records the declared NuGet metadata after
a restore; that list is no blanket approval.

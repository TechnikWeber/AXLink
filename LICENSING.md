# Licensing

## AXLink's own licence (since 2026-11-02)

- **Code** (Go core, scripts, tests, tools): **GPL-3.0-or-later**, text in [LICENSE](LICENSE).
  New source files start with `SPDX-License-Identifier: GPL-3.0-or-later`.
- **Protocol specifications** (under `docs/protocol/`, the first is the station beacon): **CC BY 4.0**, text in
  [licenses/CC-BY-4.0.txt](licenses/CC-BY-4.0.txt). Any project can implement the protocol
  freely, firmware under Apache-2.0 included.
- Other documentation: like the code, unless stated otherwise.
- **Additional permission for Wintun** (GPL-3.0 section 7, D8): AXLink may be passed on
  together with `wintun.dll`; text [below](#additional-permission-for-wintun).
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
| `golang.zx2c4.com/wintun` | MIT | Go binding of `wintun.dll` (Windows) |
| `golang.zx2c4.com/wireguard/windows` v1.1.1 | MIT | only `tunnel/winipcfg`: address, routes and MTU of the Windows adapter |
| `golang.org/x/sys`, `golang.org/x/net` | BSD-3-Clause | system calls |

## wintun.dll (Windows)

The Windows adapter needs `wintun.dll`, a prebuilt driver library of WireGuard LLC under its own
"Prebuilt Binaries License" (not an open-source licence): use is allowed; redistribution only
together with software that uses the DLL through the API in `wintun.h`, as AXLink does through
`golang.zx2c4.com/wintun`.

The Wintun sources are under GPL-2.0, which cannot be combined with GPL-3.0 either; the
prebuilt DLL is what Windows can load, because it carries a signed driver.

The Windows packages of the releases contain it. For builds from source,
`scripts/fetch-wintun.ps1` fetches the DLL: it downloads
Wintun 0.14.1 from `https://www.wintun.net/builds/`, checks SHA-256
`07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51` (checked on 2026-11-02) and
puts the licence next to the DLL as `wintun-LICENSE.txt`. A Windows package may contain
`wintun.dll` (permission below), always together with `wintun-LICENSE.txt`, and may name Wintun
but not advertise with the names WireGuard or Wintun (point 3e of its licence).

## Additional permission for Wintun

This permission is part of AXLink's licence terms for all of AXLink that is under
GPL-3.0-or-later; the SPDX headers in the files stay `GPL-3.0-or-later`. The wording follows the
template of the Free Software Foundation (GPL FAQ, "GPLIncompatibleLibs"):

> Additional permission under GNU GPL version 3 section 7
>
> If you modify this Program, or any covered work, by linking or combining it with Wintun (the
> library `wintun.dll` of WireGuard LLC, https://www.wintun.net) or a modified version of that
> library, containing parts covered by the terms of the Wintun Prebuilt Binaries License or of
> the GNU General Public License version 2, the licensors of this Program grant you additional
> permission to convey the resulting work.

As section 7 allows, whoever passes AXLink on may remove this permission from their copy.

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

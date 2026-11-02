# AXLink — decisions

This document takes precedence over every other document in the repository. Where two entries
conflict, the newer one applies. Older German planning documents are in tag `v0.1.0-alpha.1`
(see [HISTORY.md](HISTORY.md)).

## 2026-11-02

**D1 – Tech stack: Go.** The core is rebuilt as one Go program (`axlinkd`): wireguard-go/tun
(Linux, Windows, macOS), gVisor netstack (userspace TCP/IP for gateways and for running
without admin rights), go.bug.st/serial. We implement KISS, AX.25 UI frames and IP over AX.25
(PID `0xCC`) ourselves. Packet.NET, pdn and pdn-net remain peers in interop tests, not runtime
dependencies. Rationale: [REVIEW_2026-11-02.md](REVIEW_2026-11-02.md), section 6. The pdn/pdn-net
state is preserved as tag `v0.1.0-alpha.1`. Development: [DEVELOPMENT.md](DEVELOPMENT.md).

**D2 – No legal restrictions built in, no country profiles.** AXLink can be used worldwide and
is as open as possible. Every operator checks what is allowed on their frequency. The software
passes everything, HTTPS, TLS and VPN included, and blocks no service for legal reasons. There
are no rules per country or band.
- Disaster communication is one of the intended uses. The main goal for now is traffic between
  radio amateurs, but that is no restriction.
- Technical channel protection stays, because it keeps a 1200 bit/s channel usable: filters for
  broadcast and multicast chatter (e.g. mDNS, LLMNR, NetBIOS, SSDP), airtime fairness, queue
  limits. These filters can be configured and switched off; they are not legal rules.
- Legal information (e.g. for Germany § 5 AFuG, § 16 AFuV) appears only in the documentation.

**D3 – Placeholder callsigns only.** Code, tests and example configurations never contain a
real callsign. Placeholders: `N0CALL`; in the lab `TESTA-1`, `TESTB-1`, `TESTC-1`. Without its
own callsign configured, AXLink does not transmit (AX.25 needs a source address). For automatic
stations each operator enters their own callsign.

**D4 – Firmware extensions by the project owner.** The project owner builds the extensions of
the KISS TNC app (ACKMODE and more) himself, in the AXLink TNC repository (D4a). Upstream
(F4HWN/F4WAT) is informed through a feature request (`upstream/f4hwn-kisstnc-issue.md`). AXLink
keeps supporting every standard KISS TNC; the F4HWN profile also works with the unmodified app,
because AXLink paces on the host side.
- Status: the request was posted as issue #763, which Armel converted into
  [Discussion #764](https://github.com/armel/uv-k1-k5v3-firmware-custom/discussions/764)
  (Ideas). He does that with feature requests; it was not rejected.
- Purpose of the request: the official app should get the extensions so that every F4HWN user
  has them. USB back-pressure lives in the resident firmware and can only be fixed there, not
  in an app. And the command formats should be agreed.
- Our app may be built right away (against the `app_api.h` of v6.3). We point to it in #764 only
  once it is built and tested at least in the host tests and the AXLink lab. AXLink is ready:
  the profile `f4hwn-kisstnc-ackmode` and the model with ACKMODE.

**D4a – The own KISS app stands alone and is offered separately.**
- It does not need the messenger. The loader checks only the ABI major version, the API level,
  the capabilities and the link address. These are the same in F4HWN v6.3.0, the messenger fork
  and the v7 branch (ABI 1, address fixed at 0x20000280; API level 3 in v6.3, 4 in v7; the
  service table only grows). An app with `APP_API_MIN` 3 therefore runs on the original
  **Labs v6.3.0** and on **Fusion v7**, which carries the app platform there. If a v7 release
  changes the ABI, we rebuild.
- Not on Fusion v6.3 and not on the messenger v6.3 (no app loader).
- With the original firmware only the USB back-pressure is missing (resident part); AXLink paces
  anyway.
- The audience is larger than the messenger's: whoever only wants IP over AX.25 takes the
  original Labs (v6.3) or Fusion (v7) plus the app, without Multiboot.
- Distribution: GitHub releases with the `.app` file (UV Studio → Apps → Community → Install a
  .app) and later an entry in the Community tab (Apps → Community → Propose an app; only possible
  with the app running on a radio).
- Name: the Community tab compares names as lower-case letters and digits and does not accept
  official names. "KISS TNC+" would give "kisstnc" like the original and is not possible. Name:
  **AXLink TNC**; on-screen title "AXL TNC".
- Location: its own public repository
  [TechnikWeber/AXLink-TNC](https://github.com/TechnikWeber/AXLink-TNC), Apache-2.0 like the
  original. F4HWN is a submodule at the release tag (now v6.3.0); the app is built against the
  original firmware. `app/` keeps the file names of `App/apps/kisstnc`; the first commit is the
  unmodified app and every change is a separate commit, so that F4WAT can take changes over
  without a pull request. Open points: `TODO.md` there and here.

**D5 – Licence.** Code: GPL-3.0-or-later ([LICENSE](../LICENSE)). Protocol specifications:
CC BY 4.0 ([licenses/CC-BY-4.0.txt](../licenses/CC-BY-4.0.txt)). Details:
[LICENSING.md](../LICENSING.md).

**D6 – Firmware path: separate first, united with v7.** Guide for the interim:
[MULTIBOOT_V6.md](MULTIBOOT_V6.md). Everything about Multiboot is an interim solution and is
deleted after the migration to v7 (checklist there).
- Now (F4HWN v6.3): two firmwares via Multiboot. The messenger as Main, F4HWN Labs v6.3 (with
  the KISS TNC app) in a user slot; switch at power-on while holding `MENU`. According to the
  source the flash areas do not overlap (Multiboot slots 0x020000–0x0BFFFF, app slots
  0x102000–0x161FFF, messenger 0x1BA000–0x1C9FFF). Not checked on hardware yet.
- Our own KISS app (AXLink TNC: ACKMODE, a counter for dropped frames, identification, RSSI) as
  an overlay app, see D4a. It costs no MCU flash and runs on Labs v6.3 and on v7. Whoever only
  wants the TNC needs neither the messenger nor Multiboot.
- With F4HWN v7: move the messenger to v7; the app platform is then part of the firmware, and
  the KISS app runs in the same firmware as the messenger.
- Native KISS in the messenger only as a fallback, if v7 does not come or the app platform does
  not fit next to the messenger on v7.

**D7 – Language.** Everything in the repositories is English: code, comments, documentation,
commit messages and command-line output. The user interface (the planned web UI) offers English
and German; the default follows the system or browser language and falls back to English.
Commit messages and documents from before this decision are partly German and stay as they are.

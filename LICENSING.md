# Lizenzstand

## Eigene Lizenz (seit 2026-11-02)

- **Code** (Go-Kern, Skripte, Tests, Werkzeuge): **GPL-3.0-or-later**, Text in [`LICENSE`](LICENSE).
  Neue Quelldateien beginnen mit `SPDX-License-Identifier: GPL-3.0-or-later`.
- **Protokollspezifikationen** (künftig unter `docs/protocol/`): **CC BY 4.0**, Text in
  [`licenses/CC-BY-4.0.txt`](licenses/CC-BY-4.0.txt). So kann jedes Projekt das Protokoll frei
  umsetzen, auch Firmware unter Apache-2.0.
- Übrige Dokumentation: wie der Code, sofern nicht anders angegeben.
- Der Tag `v0.1.0-alpha.1` entstand vor dieser Festlegung und war ein privates Prerelease ohne
  Lizenzfreigabe.
- Fremdcode behält seine Lizenz (Tabellen unten).

## Enthaltene Fremdquellen und Änderungen

| Pfad | Herkunft / Lizenz |
|---|---|
| `artifacts/m0/sources/packet.net`, `references/0.txt`, `references/1.txt` | Packet.NET; AGPL-3.0-or-later laut Build-Metadaten, vollständiger Lizenztext im Quellenordner |
| `artifacts/m0/sources/pdn-net` | pdn-net; AGPL-3.0-or-later, vollständiger Lizenztext im Quellenordner |
| `patches/pdn-net-destination-filter.patch` | Änderungen am gepinnten pdn-net; ursprüngliche AGPL-Lizenz bleibt erhalten; Änderung und Tests beschrieben in `patches/README.md` |
| `artifacts/m0/firmware/kisstnc_app.c` | Johan Denoyer F4WAT / Armel F4HWN; Apache-2.0 gemäß erhaltenem Dateikopf; Lizenztext unter `licenses/Apache-2.0.txt` |
| `artifacts/m0/references/4.txt` | LinBPQ/BPQ32, John Wiseman G8BPQ; GPL-3.0-or-later gemäß erhaltenem Dateikopf; Lizenztext unter `licenses/GPL-3.0.txt` |
| `artifacts/m0/references/3.txt` | tncattach README; Projektlizenz MIT, Text unter `licenses/tncattach-MIT.txt` |
| `artifacts/m0/references/5.txt` | mod-orphan README; deklarierte GPL-2.0, Text unter `licenses/GPL-2.0.txt` |
| `internal/ax25/testdata/packetnet-*.jsonl` | AX.25-Frames, die Packet.NET in unserem eigenen Labor gesendet/empfangen hat (Testdaten, kein Code) |

Quell-URLs und geprüfte Commit-IDs stehen in `upstream.lock.json`. Die gesicherten Quellkopien
wurden nicht geändert; Prüfsummen stehen in `artifacts/m0/source-sha256.json`. Bei den älteren
Alternativen-Referenzen sind Branch-URLs statt reproduzierbarer Commit-Pins dokumentiert.
Die separat beigelegte tncattach-Lizenz wurde am 31.10.2026 aus
`https://raw.githubusercontent.com/markqvist/tncattach/master/LICENSE` abgerufen.

`internal/sim/f4hwnkiss` ist ein eigenes Verhaltensmodell der KISS-TNC-App, nach deren
Quelltext geschrieben; es enthält keinen Code aus der Firmware.

## Go-Abhängigkeiten

Laut `go.mod`; alle mit GPL-3.0 vereinbar.

| Modul | Lizenz | Wofür |
|---|---|---|
| `go.bug.st/serial` v1.8.0 | BSD-3-Clause | serielle Ports, USB-Kennungen |
| `golang.zx2c4.com/wireguard` (Commit `2631ce99a06f`) | MIT | Netzadapter (nur das Paket `tun`) |
| `golang.zx2c4.com/wintun` | MIT | Windows-Adapter (künftig; `wintun.dll` wird noch nicht mitgeliefert) |
| `golang.org/x/sys`, `golang.org/x/net` | BSD-3-Clause | Systemaufrufe |

Geplant: gVisor netstack (Apache-2.0) für Gateway und Betrieb ohne Admin-Rechte.

## Werkzeuge des v0.1-Labors

Packet.NET und pdn-net werden aus gepinnten vollständigen Repositories gebaut. Der Trace-Adapter
`tools/TraceCheck` bindet Packet.NET ein; bei einer Binärdistribution wären dessen
AGPL-Anforderungen mit zu berücksichtigen. Es werden keine Upstream-Binärdateien oder Images
mitgeliefert.

net-sim und das Microsoft-ASP.NET-Runtime-Image werden separat per Docker bezogen; Digests und
Metadaten sind dokumentiert. Das gepinnte net-sim-Image nennt im Label MIT, das Repository
`packethacking/net-sim` steht heute unter AGPL-3.0. Wir nutzen net-sim nur als getrenntes
Testwerkzeug und verteilen es nicht. `scripts/inventory-package-licenses.py` erfasst deklarierte
NuGet-Metadaten nach dem Restore; diese Liste ist keine pauschale Kompatibilitätsfreigabe.

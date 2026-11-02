# Lizenzstand

Die Eigenlizenz für den neu geschriebenen AXLink-Code ist noch nicht festgelegt.
Der erste Simulatorstand wird deshalb zunächst als **privates Prerelease**
geführt; daraus folgt keine allgemeine Lizenzfreigabe für den eigenen Code.
Die Lizenzentscheidung bleibt ausdrücklich offen. Fremdcode wird dadurch nicht
umgelizenziert.

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

Quell-URLs und geprüfte Commit-IDs stehen in `upstream.lock.json`. Die
gesicherten Quellkopien wurden nicht geändert; Prüfsummen stehen in
`artifacts/m0/source-sha256.json`. Bei den älteren Alternativen-Referenzen sind
Branch-URLs statt reproduzierbarer Commit-Pins dokumentiert; dies bleibt ein
Audit-Limit. Die separat beigelegte tncattach-Lizenz wurde am 31.10.2026 aus
`https://raw.githubusercontent.com/markqvist/tncattach/master/LICENSE` abgerufen.

## Zur Laufzeit geladene Komponenten

Packet.NET und pdn-net werden aus gepinnten vollständigen Repositories gebaut.
Der Trace-Adapter bindet Packet.NET ein; bei einer Binärdistribution sind
dessen AGPL-Anforderungen mit zu berücksichtigen. Der Prerelease liefert
Quellen/Testnachweise, keine gebündelten .NET-/Upstream-Binärdateien oder Images.

net-sim und das Microsoft-ASP.NET-Runtime-Image werden separat per Docker
bezogen; Digests und Metadaten sind dokumentiert. Das net-sim-Image deklariert
MIT. Ein vollständiger Image-/Asset-/Paketlizenzaudit steht noch aus.
`scripts/inventory-package-licenses.py` erfasst deklarierte NuGet-Metadaten
nach dem Restore; diese Liste ist keine pauschale Kompatibilitätsfreigabe.

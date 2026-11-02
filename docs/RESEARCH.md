# Quellen und Bestandsaufnahme (online geprüft am 2026-10-31)

Die folgenden Aussagen sind auf öffentlich sichtbare Repository-README-/Quelltextseiten gestützt. **Weder Builds noch Hardwaretests wurden in dieser Projektübergabe ausgeführt.** Kompatibilität und aktuelle Versionen im Codex-Projekt erneut überprüfen.

## Primärkomponenten

| Projekt | URL | Geprüft / Bedeutung | Risiken / Status |
|---|---|---|---|
| Packet.NET | https://github.com/packet-net/packet.net | .NET-10-AX.25-v2.2+-Stack, KISS-Serial/TCP, NET/ROM, Node-Server und `Packet.*`-Bibliotheken; AGPL-3.0 | Nicht mit vollständig fertigem Mesh-IP-Stack verwechseln |
| pdn-net | https://github.com/packet-net/pdn-net | Linux TUN `pdn0`, RHPv2 zum `pdn`-Node, AX.25 UI mit IPv4 PID `0xCC` und ARP PID `0xCD`, IP-/AX.25-Reassembly/Fragmentierung; AGPL-3.0-or-later laut README | Derzeit kein Discovery/Multi-Hop, TUN nur Linux, Hardware-End-to-End im CI nicht vollständig abgedeckt |
| Linux old AX.25 | https://github.com/linux-netdev/mod-orphan | Altes extern pflegbares Kernel-Netzwerkmodul-Repository | Kernelabhängige Wartung; Referenz und optionaler Interop-Test, nicht zwingend produktiver Kern |
| tncattach | https://github.com/markqvist/tncattach | MIT, KISS-TNC als Linux-TUN/TAP-Netzinterface | Die Funk-IP-Verkapselung ist nicht automatisch die klassische AX.25-IP-Datagrammkonvention; als alternative Technik studieren |
| LinBPQ/BPQ32 | https://github.com/g8bpq/linbpq | Quelle mit Chat-/Mailbox-/Knotenfunktionen; Codeabschnitte nennen GPL-3.0-or-later | GitHub laut Maintainer hauptsächlich zur Quellverteilung, Pull Requests/Issues werden womöglich nicht verfolgt; Integration und Gesamt-Lizenz verifizieren |

## Hardware

| Projekt | URL | Geprüft / Bedeutung | Offene Tests |
|---|---|---|---|
| Quansheng UV-K5 V3 F4HWN Fusion/Labs | https://github.com/armel/uv-k1-k5v3-firmware-custom | README nennt KISS-TNC als Overlay-App, APRS/AX.25 Bell-202 1200 baud und KISS über USB-C | Unterstützter Gerätetyp/Release, KISS-Framegrenzen, Dauerbetrieb, PTT/RX, Firmware-/App-ABI und bidirektionale AX.25-IP-Tauglichkeit |
| Dire Wolf | https://github.com/wb2osz/direwolf | Software-TNC/APRS/Daten | Fallback bei Firmware-Inkompatibilität; Sound/PTT-Interface erforderlich |

## Interoperabilitätsoptionen

| Projekt | URL | Strategie |
|---|---|---|
| Reticulum | https://github.com/markqvist/Reticulum | Optional als **separater** Stack/Sidecar; nicht mit IPv4 verwechseln |
| NomadNet | https://github.com/markqvist/NomadNet | Optionaler Seiten-/Clientadapter; eigenes Reticulum-Markup/Adressierung |
| LXMF | https://github.com/markqvist/LXMF | Optionaler Nachrichtenadapter / Bridge |
| AllStarLink | https://www.allstarlink.org | Sprachfunk-Relaisnetz, nicht das geplante IP-Mesh |
| Winlink | https://winlink.org | Nachrichten-/Store-and-Forward-Vorbild, nicht universelles TCP/IP |
| HAMNET/AREDN | https://hamnetdb.net / https://www.arednmesh.org | Beispiele bereits existierender IP-Funknetze; andere Hardware/Medien/Datenraten |

## Wichtige technische Details aus Upstream-README

- `pdn-net` ist **ein Client** des `pdn`-Nodes über RHPv2, standardmäßig `127.0.0.1:9000` (JSON-framed); Link: https://github.com/packet-net/pdn-net
- AX.25-On-Air-IPv4-Frames: UI-Frame mit **PID `0xCC`**, IP-Paket im Informationsfeld, **kein privates pdn-On-Air-Envelope**. AX.25-ARP PID `0xCD`. Das dient der Interoperabilität mit klassischen AX.25-IP-Implementierungen.
- Upstream nennt als offene Punkte: dynamisches Routing/Discovery, Windows Wintun hinter `ITunDevice`, Hardware-End-to-End.
- `Packet.NET` hat bereits umfangreiche Tests gegen weitere Packet-Radio-Systeme; keine Annahme einer vollständigen Chatprotokoll-Kompatibilität.
- `tncattach` Linux-KISS-Schnittstelle mit TUN/TAP und optionalem KISS über TCP; eigene Definition der Datenverkapselung vor jedem Interopversuch nachlesen.

## Lizenz-/Betriebshinweise

- Packet.NET und pdn-net: AGPL-3.0-Familie gemäß Repository. Bei Kombinieren/Netzdienst/Verteilen Pflichten prüfen; keine pauschale „GPL-kompatibel“-Garantie.
- LinBPQ/BPQ32: geprüfte Quelldateien `Bpq32.c` / `config.c` nennen **GPL v3 oder später**; restliche Dateien, Assets, Abhängigkeiten und Distribution trotzdem auditieren.
- Reticulum: aktuelle Lizenz der **Referenzimplementierung** gesondert evaluieren; Protokollkompatibilität ist nicht automatisch eine Erlaubnis zum Kopieren des Codes.
- Deutsches Amateurfunkrecht: https://www.gesetze-im-internet.de/afuv_2005/__13.html und https://www.gesetze-im-internet.de/afuv_2005/__16.html . §13 automatisch betriebene Stationen, §16 technische Bedingungen und Verbot der Inhaltsverschlüsselung im regulären Amateurfunkverkehr.
- Keine Rechtsberatung: vor realem Funkbetrieb aktuelle Amateurfunk-Frequenzen, Bandbreiten, Rufzeichenzuteilung und Betriebsmodus gegen offizielle Bestimmungen prüfen.

## Explizit zu verifizierende Behauptungen aus der vorangegangenen Chatdiskussion

- Konkrete Firmware-Version „6.3.0“ und KISS-Kommandounterstützung: gegen gewählten Release/Hardware-Revision prüfen, nicht nur README.
- Unter Windows funktionierende direkte KISS/TUN-Anbindung: **nicht fertig**; nur Ziel.
- Pfade/Protokolle einer NomadNet-Kompatibilität: zunächst isolierte Integration explorieren, keine perfekte Chat-/Seitenkompatibilität versprechen.
- Selbstorganisiertes Multihop-Netz: technische Designanforderung, noch keine nachgewiesene Implementierung.

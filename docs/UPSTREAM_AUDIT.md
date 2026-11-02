# AXLink — M0 Upstream-Audit

Stand: 2026-10-31. **Build-/Testausführung jetzt möglich; M0-Abnahme noch offen.**
Die früheren Sandbox-/SDK-Blockaden sind aufgehoben. F1 ist reproduziert,
der lokale Empfangsfilter-Patch besteht die pdn-net-Suite. Ein KISS-Simulator-
Smoke-Test besteht; die Zwei-Knoten-IP-Demo mit UDP/HTTP, Trace und
Unterbrechung/Recovery ist ebenfalls bestanden (siehe `TWO_NODE_DEMO.md`).
Die Abschnitte zu früheren Blockaden dokumentieren die vorherigen Sitzungen.

## Entscheidungen

- Projektname AXLink, vom Nutzer festgelegt. Normale IPv4-Anwendungen bleiben
  Kernfunktion; Linux zuerst, Windows über separaten Adapter.
- Packet.NET/pdn als separaten KISS-Eigentümer übernehmen, pdn-net als separaten
  RHPv2-IP-Adapter evaluieren. Kein umfassender Fork, kein eigener Protokollstack.
- Für pdn-net einen gezielten Patch für Empfangsfilter und später TUN-Fabrik/
  Wintun vorbereiten, aber erst nach reproduzierbarem Fehlertest verändern.
- Keine Funkhardware aktiviert, keine öffentlichen IPs oder Default-Routen.
- Eigenlizenz bleibt offen. Fremde Quellen behalten ihre Lizenz und Herkunft.

## Fixierte Quellen

| Repository | geprüfter Commit | SDK / Ziel | Lizenzbefund |
|---|---|---|---|
| [Packet.NET](https://github.com/packet-net/packet.net/tree/bded4671cc958808d4a2746d18e6636066767d42) | `bded4671cc958808d4a2746d18e6636066767d42` | global.json 10.0.107, latestFeature; net10.0 | LICENSE: AGPL v3; Directory.Build.props: AGPL-3.0-or-later |
| [pdn-net](https://github.com/packet-net/pdn-net/tree/91c412c91604c17bc28abd49e66443377292e5dc) | `91c412c91604c17bc28abd49e66443377292e5dc` | global.json 10.0.107, latestFeature; net10.0 | LICENSE: AGPL v3; Directory.Build.props: AGPL-3.0-or-later |

Hashes über GitHub `/commits/main` ermittelt. Dateien danach anhand des jeweiligen
Commit-Hashes über die GitHub-Anbindung gelesen. `upstream.lock.json` enthält
Repo-URLs, Lösungen, Quellpfade und Herkunftslinks. `artifacts/m0/sources` sind
**ausgewählte Audit-Kopien**, keine vollständigen oder baubaren Checkouts.
SHA-256-Prüfsummen dokumentieren ihre lokal abgelegten Bytes.

## Tatsächlich ausgeführte Versuche

| Befehl | Ergebnis |
|---|---|
| `git clone https://github.com/packet-net/packet.net.git upstream/packet.net` | Exit 128: Could not resolve host: github.com |
| `git clone https://github.com/packet-net/pdn-net.git upstream/pdn-net` | Exit 128: gleicher DNS-Fehler |
| `dotnet --info` | Exit 127: command not found |
| `ls -l /dev/net/tun` | nicht vorhanden |
| `git init` | Exit 128: .git/hooks ist schreibgeschützt |
| `mv /home/philipp/Downloads/RadioMesh /home/philipp/Downloads/AXLink` | Exit 1: Read-only file system |

`dotnet build` und `dotnet test` konnten mangels SDK/Checkouts nicht ausgeführt
werden. Es gibt keine bestandenen .NET-, IP-, Hardware- oder Interop-Tests dieser
Sitzung. Die Internetverbindung der GitHub-Anbindung ermöglicht Quellprüfung,
aber ersetzt weder lokalen Restore noch Build.

Reproduktion und maschinenlesbares Ergebnis:

```sh
python3 scripts/audit-upstream.py        # Quellenintegrität + Voraussetzungen
python3 scripts/audit-upstream.py --run  # nach Bereitstellung des SDK/Netzzugriffs
```

Das Skript klont die fixierten Commits, verweigert abweichende/verschmutzte
Checkouts und führt je Lösung `dotnet build --configuration Release` sowie
`dotnet test --configuration Release --no-build --logger trx` aus. Logs und
TRX liegen unter `artifacts/m0`. Übersprungene Tests sind kein Interop-Nachweis.

## Geprüfte Schnittstellen und Befunde

Die folgenden Pfade beziehen sich auf die oben fixierten Commits.

### RHPv2 und AX.25-IP

`pdn-net/src/Packet.Net/Rhp/RhpFraming.cs`: zwei Byte Big-Endian-Länge vor UTF-8-
JSON. `RhpCustomClient.cs`: socket(ax25/custom), bind, sendto und recv;
Nutzbytes als Latin-1-Codepunkte im JSON-String, **nicht Base64**. PID ist das
erste Datenbyte im lokalen RHP-custom-Payload.

`IpAx25Bridge.cs`: TUN-Paket → IPv4-Ziel → CallsignResolver → `[0xCC] + IP`.
Empfang entfernt PID 0xCC vor TUN. ARP 0xCD und Segment-Reassembly 0x08 haben
separate Zweige; einige Kommentare behaupten fälschlich noch, ARP sei TODO.
Die Implementierung ist maßgeblich. IPv4-Fragmentierung ist vorhanden, ihre
Grenzfälle müssen ausgeführt geprüft werden.

Packet.NET `src/Packet.Rhp2.Server/IRhpGateway.cs` und `RhpServer.cs` sowie
`src/Packet.Node/Rhp/SupervisorRhpGateway.cs` bestätigen den UI-Sendepfad mit
separatem PID/Info-Feld. Der Server verwaltet mehrere Clients. Custom-UI-
Subscriptions sind promiscuous; Empfang enthält tatsächliche Quelle/Zieladresse.
Somit sind parallele native Dienste grundsätzlich anschließbar; korrekte
Multiplexierung/Abmeldung und Protokollfilter bleiben Integrationstests.

**F1 — fehlender Empfangs-Zielfilter (statischer Befund, hohe Priorität):**
`RhpServer.OnUiReceivedAsync` liefert fremdadressierte Frames ausdrücklich mit.
`RhpCustomClient.Dispatch` reicht recv ungefiltert weiter. `IpAx25Bridge.HandleInboundAsync`
schreibt PID-0xCC-Inhalte ohne Prüfung von `dg.Dest` an TUN. Damit kann ein
mitgehörtes Paket für C auch am TUN von B ankommen. Auswirkungen auf Kernel-
Forwarding/Duplikate sind noch nicht gemessen. Vor M1 einen Dreistationen-
Negativtest ergänzen, dann Zielprüfung mit expliziter Broadcast-/ARP-Policy.

**F2 — Multiport:** Client bindet `port:null`, sendto setzt keinen Port.
Im Gateway kann dies beim Senden den ersten Port wählen, während Empfang alle
Ports hört. Für M1 exakt ein Port; vor Mehrportbetrieb Portwahl konfigurierbar machen.

**F3 — Lifecycle/Validierung:** Empfangenes IP wird ohne umfassende Validierung
an TUN gegeben; eingehende Event-Behandlung startet asynchron. Abbruch, Backpressure,
fehlerhafte Pakete und Disconnect getrennt testen; nicht aus der API allein Robustheit ableiten.

### Linux-TUN / Windows

**F4 — inkompatible Segment-Header zwischen den Kandidaten (neu, statisch):**
Der vollständige Checkout zeigt in Packet.NET
`src/Packet.Ax25/Session/Segmenter.cs` ein First-Bit `0x80` und einen
absteigenden 7-Bit-Restzähler. pdn-net
`src/Packet.Net/Bridge/Ax25Reassembler.cs` interpretiert dagegen `header >> 1`
als aufsteigende Segmentnummer und Bit 0 als Last-Bit. Diese Headerinterpretationen
sind nicht kompatibel. Bisherige pdn-net-Tests reproduzieren nur die eigene
Kodierung. Der neue F1-Segmenttest übernimmt sie bewusst ausschließlich zur
Prüfung des Zielfilters; er belegt keine Standardkonformität. Vor Segment-Interop
müssen echte Referenzvektoren ausgeführt und die vorhandene Implementierung
gezielt korrigiert/ersetzt werden. Kein neuer AX.25-Stack als Lösung vorgesehen.

**F5 — IPv4-Fragmentierungsgrenzen (neu, statisch):**
`Ipv4Fragmenter.Fragment` überschreibt Flags/Offset ausgehend von Offset 0,
ohne DF oder einen vorhandenen Fragmentoffset auszuwerten. Bei Optionsheadern
werden außerdem alle Optionen in jedes Fragment kopiert. Verhalten bei DF,
Refragmentierung und kopierbaren/nicht kopierbaren Optionen ist damit vor
allgemeinem Routing gesondert zu testen. Für die erste M1-Demo nur kleine
Datagramme innerhalb der expliziten TUN-/Frame-MTU verwenden. F4/F5 wurden
nicht im Zielfilter-Patch mit verändert und sind noch keine ausgeführten
Fehlertests.

`ITunDevice`: Name, ReadPacketAsync, WritePacketAsync, IDisposable; genau ein
Layer-3-Paket je Aufruf. Gute Test-/Wintun-Trennstelle. `TunDevice` nutzt Linux
`/dev/net/tun`. `src/pdn-net/Program.cs` erstellt konkret `TunDevice.Open`;
Windows benötigt neben der Implementierung auch eine OS-abhängige Fabrik und
separate Einrichtung von Adresse/MTU/Routen. Diese Einstellungen sind nicht Teil
von ITunDevice. Keine funktionierende Windows-Unterstützung nachgewiesen.

### KISS und vorhandene Simulation

Packet.NET bietet KissTcpClient und KissAx25Bridge. Letztere verbindet den
vorhandenen AX.25-Adapter mit Transporten; asynchrones Senden erfolgt dort
fire-and-forget, Fehler müssen über Transportdiagnose beobachtet werden.
`NetsimKissTcpInterop` enthält einen UI-Frame-Roundtrip über net-sim an
8100/8101. Das ist ein brauchbarer Ausgangspunkt, aber kein IP-/HTTP-Nachweis.
Der Test überspringt sich ohne gesunden net-sim-Dienst. Unser ergänzender Plan:
[SIMULATOR_PLAN.md](SIMULATOR_PLAN.md).

## Hardwareprüfung an Originalquellen

[F4HWN v6.3.0](https://github.com/armel/uv-k1-k5v3-firmware-custom/releases/tag/v6.3.0),
Quellstand `ca3a4271386049e7b459b215cef0ad0eea1a30ca` für Tag v6.3.0.
`App/apps/kisstnc/README.md` und `kisstnc_app.c` wurden gelesen und lokal gesichert.
Kandidat: UV-K5 V3 bzw. UV-K1 mit passender Labs-Overlay/API-Level-3-Umgebung.

- RX prüft und entfernt FCS; TX fügt FCS hinzu. Kein APRS-only-Filter im KISS-Pfad.
- Bell-202/1200; KISS Data, TXDELAY und Persistence implementiert.
- FRAME_MAX=330 inklusive FCS, Hostparser reserviert zwei Bytes für FCS.
  Für zunächst 256 Byte IP plus AX.25-Header reicht der vorgesehene Puffer;
  reale Grenzfälle mit Escape-Bytes/USB-Pacing bleiben offen.
- Nur ein wartender TX-Frame; weitere Hostframes können verloren gehen.
  Halbduplex, DCD erkennt laut README keine gewöhnliche Sprachträgerbelegung.
- Upstream berichtet USB-Resets durch HF-Einkopplung. Die beschriebenen TX-
  Beobachtungen beziehen sich auf UV-K1/APRS, nicht unseren UV-K5-V3-IP-Dauerlasttest.

Keine Hardware oder Firmware wurde verändert. Upstream-Testergebnisse wurden
nicht als eigene Ergebnisse übernommen.

## Alternativen und Lizenzgrenzen

| Komponente / Quelle | Quellbefund und Entscheidung | Noch offen |
|---|---|---|
| [Linux mod-orphan](https://github.com/linux-netdev/mod-orphan) | README nennt ausgelagerte AX.25/NET-ROM/ROSE-Module und GPL-2.0. Referenz für klassisches AX.25-IP; kein plattformübergreifender AXLink-Kern. | Kernelgebundener Build und echter Interop-Vergleich nicht ausgeführt. |
| [tncattach](https://github.com/markqvist/tncattach) | Linux-TUN/TAP über KISS laut README. Alternativansatz, keine ungeprüfte Übernahme als AX.25-IP-kompatibel. | Exakte Funkverkapselung und MIT-Lizenz aus Übergabe noch anhand Code/LICENSE bestätigen; kein Vergleichstest. |
| [LinBPQ](https://github.com/g8bpq/linbpq/blob/master/Bpq32.c) | Bpq32.c nennt GPL v3 oder später. Als separates Interop-/Chat-/Mailbox-Ziel evaluieren. | Gesamt-Lizenzinventar und IP-/Chat-Kompatibilität nicht geprüft. |
| Packet.NET / pdn-net | Als getrennte Prozesse evaluieren; AGPL-3.0-or-later in Build-Metadaten bestätigt. | Transitive Paket-/Asset-Lizenzen nach Restore inventarisieren. Keine pauschale Lizenzfreigabe. |

Die Alternativen-Referenzen sind Momentaufnahmen beweglicher Branches, keine
reproduzierbaren Build-Pins. Ihre Rohkopien und URLs sind in den Audit-Artefakten.
`Directory.Packages.props` von pdn-net erfasst direkte Paketversionen; es ersetzt
kein transitives Lizenzinventar. Packet.NET unterdrückt in Directory.Build.props
gezielt NuGet-Advisory GHSA-2m69-gcr7-jv3q; vor einer Distribution aktuellen
Restore-/Audit-Befund prüfen. Keine Änderung an Upstream-Suppressions vorgenommen.

## Nächste Abnahme

1. Umbenennen/Verschieben nach `/home/philipp/AXLink` erledigt;
   echtes Git-Repository noch außerhalb der Agent-Sandbox initialisieren
   (das aktuelle `.git` ist hier weiterhin schreibgeschützt).
2. .NET 10 und Netzwerkzugriff für GitHub/NuGet bereitstellen; Audit-Skript ausführen.
3. F1 mit vorhandenem Test-Fake reproduzieren und minimalen Patch erarbeiten.
4. Zwei echte pdn-/pdn-net-Paare isoliert verbinden, UDP plus Trace und
   Unterbrechungs-Negativkontrolle nachweisen; danach HTTP.
5. Kernel/BPQ-Interop und Hardwaregrenzen separat messen. Mesh bleibt unbestätigt.

## Fortsetzung nach dem Umzug (2026-10-31)

### Ursache der Umgebungsblockaden nachgewiesen

- `findmnt -T /home/philipp/AXLink/.git`: eigenes `tmpfs`, Optionen
  `ro,nosuid,nodev`, Modus 555. Der Projektordner selbst liegt auf schreibbarem
  btrfs. Das eingeblendete leere `.git` lässt keinen Schluss auf ein eventuell
  darunter vorhandenes Host-Repository zu. **Nicht löschen oder chmod verwenden.**
- `/etc/resolv.conf` verweist auf den systemd-resolved-Stub `127.0.0.53`.
  Ein Python-Aufruf von `socket.socket(AF_INET, SOCK_DGRAM)` sowie derselbe
  Aufruf mit `SOCK_STREAM` scheitern jeweils mit `EPERM` (Operation not permitted).
  Damit ist Netzwerkzugriff bereits vor DNS blockiert. Keine Änderung der
  Resolverkonfiguration gerechtfertigt; Host-DNS ist hier nicht abschließend geprüft.
- `ip -brief address`, `ip route` und `resolvectl status` scheitern ebenfalls
  an Berechtigungen. Die separat erlaubte `git ls-remote`-Abfrage gelingt weiter;
  ihr Erfolg ist kein Nachweis für Netzwerkrechte anderer Agent-Kommandos.

Abhilfe für die Voraussetzungen: im normalen Host-Terminal ohne sudo
`bash /home/philipp/AXLink/scripts/setup-host.sh` ausführen. Das Skript prüft
Sockets/DNS, initialisiert Git unter Erhalt vorhandener Historie, installiert
SDK 10.0.107 unter `.tools/dotnet` und führt Audit sowie F1-Test aus.
Es verändert weder Resolver noch Sandbox-Regeln und aktiviert keine Hardware.
Ein bestehender abweichender/verschmutzter Upstream-Checkout führt weiterhin
zum Abbruch des Audits. Der F1-Test wird am unveränderten Upstream voraussichtlich
rot; genaue Testausgaben prüfen. Das Host-Setup wurde hier nur syntaktisch
geprüft; sein Netzwerk-Preflight stoppt erwartungsgemäß mit Exit 2.

Für spätere Agent-Builds muss die Sitzung Netzwerkzugriff für Restore erhalten.
Git-Schreiboperationen benötigen separat eine passende Ausführungsfreigabe.
Die aktuellen Sitzungsregeln verbieten Sandbox-Eskalationen; der Agent kann
diese Regeln nicht selbst aufheben. Ein weiteres Verschieben des Ordners
behebt diese Sandbox-Einschränkungen nicht.

Der neue Arbeitsordner ist bestätigt. Erneut tatsächlich ausgeführt:

| Befehl | Ergebnis |
|---|---|
| `git status --short` | Exit 128: kein Git-Repository; `.git` ist ein schreibgeschützter leerer Ordner |
| `python3 scripts/audit-upstream.py --run` | Exit 2: `dotnet` fehlt; Quellenintegrität 29/29 |
| `git ls-remote https://github.com/packet-net/pdn-net.git HEAD` | Exit 0: HEAD entspricht weiterhin dem gepinnten Commit |
| `git clone --no-checkout https://github.com/packet-net/pdn-net.git upstream/pdn-net` | Exit 128: Could not resolve host: github.com |
| `curl -sS --connect-timeout 15 --max-time 45 https://dot.net/v1/dotnet-install.sh -L -o /tmp/axlink-dotnet-install.sh` | Exit 6: Could not resolve host: dot.net; keine SDK-Installation erfolgt |
| `ls -l /dev/net/tun` | nicht vorhanden |
| `python3 scripts/test-destination-filter.py` | Exit 2: `blocked: dotnet fehlt` |

Die erfolgreiche Remote-Abfrage belegt keinen allgemeinen Netzwerkzugriff für
Clone/Restore. Die GitHub-Anbindung erlaubt weiterhin das Lesen einzelner Dateien.

Für F1 wurden am fixierten pdn-net-Commit zusätzlich
`tests/Packet.Net.Tests/IpAx25BridgeTests.cs` sowie
`Fakes/FakeTunDevice.cs` und `Fakes/FakeRhpCustomClient.cs` gelesen.
Das neue [Testgerüst](../tests/upstream/AXLinkDestinationFilterTests.cs) nutzt
diese vorhandenen Fakes. Es prüft lokale Zustellung sowie Ablehnung eines
anderen Rufzeichens und einer anderen SSID, mit identischen IPv4-Nutzbytes.
Es ruft den echten Bridge-Empfangspfad direkt und ohne asynchrone Event-Rennen auf.

Der [Runner](../scripts/test-destination-filter.py) prüft Commit und sauberen
Checkout und exportiert ihn in ein temporäres Verzeichnis. Nur dort wird die
Testdatei ergänzt; die Audit-Baseline bleibt unverändert. Ergebnis und
Kommandoausgaben liegen in `artifacts/m0/destination-filter/result.json`, TRX
nach tatsächlicher Testausführung daneben. Voraussetzung: Python >= 3.12.

**Erwartung aus Quellprüfung, noch nicht ausgeführt:** eine positive Variante
besteht, zwei fremdadressierte Varianten schlagen fehl. Ein Build-/Restorefehler
ist kein reproduzierter F1-Nachweis. Es wurde bewusst noch kein Empfangsfilter-
Patch erstellt; ARP-Broadcasts, Segmentierung und Callsign-Normalisierung
benötigen eine explizite Policy und weitere Tests vor einer Korrektur.

## Ausführung nach Freigabe von Datei- und Netzwerkzugriff

Die Sitzung arbeitet nun mit freigegebenem Netzwerk/Dateisystem. Git ist ein
funktionsfähiges Repository. SDK 10.0.107 ist lokal installiert; `/dev/net/tun`
existiert. Rootlose User-/Netzwerk-Namespaces lassen sich anlegen (geprüft mit
`unshare --user --map-root-user --net sh -c 'ip link show lo'`).

### Korrektur des Audit-Runners

Der erste Host-Lauf baute Packet.NET erfolgreich, rief aber ungefiltert auch
Interop-/Hardwaretests auf. Das war ein Fehler im AXLink-Audit-Runner. In den
TRX-Dateien sind verweigerte Zugriffe auf einen FIBOCOM-Serial-Port dokumentiert;
eine erfolgreiche Funkaussendung ist nicht nachgewiesen. Der korrigierte
Standardlauf schließt Hardware und Interop ausdrücklich aus, gemäß dem Filter
in der gepinnten Upstream-CI:

```sh
dotnet test Packet.NET.slnx --configuration Release --no-build \
  --filter 'Category!=HardwareLoop&Category!=Interop' --logger trx \
  --results-directory /home/philipp/AXLink/artifacts/m0/test-results/unit/packet.net
```

Weitere Ursachen des ersten Fehllaufs: nicht initialisiertes APRS-Submodul
sowie vier kulturabhängige README-Beispiele unter deutscher Locale. Der Runner
initialisiert nun die gepinnten Submodule und verwendet explizit
`LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8`. Dies behebt die Testvoraussetzungen;
die Kulturabhängigkeit der Upstream-Tests bleibt als Befund bestehen.
APRS-Testvektoren: Commit `dc3854a796e4a34834f83ff69472cb63ca26c618`, zusätzlich
in `upstream.lock.json` erfasst. Der Release-Build meldete 68 Warnungen,
0 Fehler. Originaler Host-Report: `artifacts/m0/initial-host-run.json`;
ursprüngliches Testlog: `artifacts/m0/initial-host-test.log`.

### F1: Rot/Grün ausgeführt

| Befehl | Ergebnis |
|---|---|
| `python3 scripts/test-destination-filter.py` | Exit 1: 9 Tests, 5 bestanden, 4 fehlgeschlagen, 0 übersprungen |
| `python3 scripts/test-destination-filter.py --patched` | Exit 0: gesamte pdn-net-Suite mit Ergänzungen, 49 bestanden, 0 fehlgeschlagen/übersprungen |

Die ursprüngliche Dreifachprobe lieferte zuvor 1 bestanden / 2 fehlgeschlagen.
Die erweiterte Probe reproduziert zusätzlich fremdadressierte Segmentzustellung
und unerwünschte Broadcast-IP-Zustellung. Der Patch verwirft IPv4/Segmente für
andere Rufzeichen/SSIDs vor Reassembly und TUN. `CALL` und `CALL-0` sind
äquivalent; ARP-Lernen und Antworten nach Ziel-IP bleiben erhalten.
[Patch und genaue Grenzen](../patches/README.md). Baseline und Patch werden in
temporären Exporten getestet; die beiden Upstream-Checkouts bleiben sauber.
Die Tests wurden nicht zu GitHub übertragen und kein Fork angelegt.

### KISS-Simulator ausgeführt

```sh
docker compose -f simulator/compose.yaml up -d --wait --wait-timeout 90
python3 scripts/test-kiss-simulator.py
docker compose -f simulator/compose.yaml logs --no-color
docker compose -f simulator/compose.yaml down
```

Alle Befehle erfolgreich, Test **1 bestanden / 0 übersprungen**. Der Runner
verlangt dieses Ergebnis ausdrücklich. Die Image-Version ist per Digest
gepinnt; Metadaten und Logs sind unter `artifacts/m0/kiss-simulator` gesichert.
Container und temporäres Compose-Netz wurden danach entfernt. Die Konfiguration
verwendet ausschließlich Loopback-KISS-TCP, keine realen Geräte.

Der UI-Textframe-Test verwendet die Upstream-Parser. Er ist noch kein Beleg für
IP über PID 0xCC, UDP, HTTP, Durchsatz, kontrollierte Verluste oder Mehrhop.
Das MIT-Lizenzlabel des Images ist erfasst, kein vollständiger Image-Lizenzaudit.
Abrufe von README/LICENSE am gelabelten Quellcommit über die GitHub-Anbindung
lieferten keine Datei; diese zusätzliche Quellenprüfung bleibt offen.

### Abhängigkeiten nach Restore

```sh
DOTNET_ROOT="$PWD/.tools/dotnet" .tools/dotnet/dotnet list upstream/packet.net/Packet.NET.slnx package --include-transitive --format json
DOTNET_ROOT="$PWD/.tools/dotnet" .tools/dotnet/dotnet list upstream/packet.net/Packet.NET.slnx package --vulnerable --include-transitive --format json
python3 scripts/inventory-package-licenses.py
```

Die Paketlisten liegen unter `artifacts/m0/packet.net-packages.json` und
`packet.net-vulnerabilities.json`; die letztere Abfrage meldete keine
verwundbaren Pakete. Vorhandene Upstream-Suppressions wurden nicht verändert;
dies ist keine allgemeine Sicherheitsfreigabe. Die aus den wiederhergestellten
NuGet-Paketen gelesenen Lizenzmetadaten stehen in
`artifacts/m0/package-license-metadata.json`. `xunit.abstractions/2.0.3` hat nur
eine ältere Lizenz-URL statt eines `license`-Elements. Paketinhalt, Lizenztexte,
Assets und Distributionspflichten müssen vor Veröffentlichung weiter geprüft
werden; die Projektlizenz bleibt offen.

### Abschlussergebnisse dieses Audit-Laufs

`python3 scripts/audit-upstream.py --run` endet mit Exit 0:
Packet.NET **13.295 bestanden, 48 übersprungen**, pdn-net **40 bestanden,
0 übersprungen**. Die Skips betreffen nicht verfügbare externe Prüfwerkzeuge
(z.B. CDDL/Hamlib); die Namen sind in `artifacts/m0/unit-summary.json` erfasst.
Hardware-/Interop-Kategorien waren explizit ausgeschlossen, nicht bestanden.
M0 bleibt hinsichtlich vollständiger Lizenzprüfung und Gegenstellen-Interop offen.

`python3 -m unittest discover -s tests -p 'test_*.py' -v`: **9 bestanden**,
Audit-Runner-Guardrails und Negativprüfungen der gespeicherten IP-Trace-Nachweise.

Danach wurde der echte Zwei-Knoten-TUN-Pfad aufgebaut und ausgeführt:
`python3 scripts/prepare-ip-lab.py`, Docker-Image-Build, dann
`python3 scripts/test-two-nodes.py --no-build`. **UDP und HTTP beidseitig,
Kanalunterbrechung und Recovery bestanden**; alle sechs erfolgreichen
Anwendungsaustausche mit unveränderten IP-Bytes in AX.25 UI/PID 0xCC auf beiden
Seiten belegt. Konfiguration, Zeiten, Buildbefehle und Grenzen sind in
[TWO_NODE_DEMO.md](TWO_NODE_DEMO.md) dokumentiert. Alle Laborcontainer wurden
anschließend entfernt. Die Referenznachweise liegen unter `artifacts/m1/evidence`.

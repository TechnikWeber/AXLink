# RadioMesh — konsolidierte Projektübergabe

Alle Inhalte aus dem Übergabepaket, Stand 31.10.2026.



---

<!-- DATEI: README.md -->

# RadioMesh — Projektübergabe

**Stand:** 31. Oktober 2026  
**Status:** Konzept / technische Evaluierung — noch kein funktionsfähiger eigener Code  
**Arbeitstitel:** RadioMesh (Name vor Veröffentlichung prüfen)

## Ziel in einem Satz

Ein offenes, plattformübergreifendes Mesh-Datennetz über KISS-/AX.25-kompatible Funkmodems, das **herkömmliche IPv4-Anwendungen (TCP/UDP/HTTP)** ebenso ermöglicht wie **Stations-/Diensteerkennung, Chat, kleine Funkseiten und optionales Store-and-Forward**.

## Getroffene Richtungsentscheidungen

1. **Normales IP muss ein Kernfeature bleiben**, nicht bloß ein späteres Gateway. Das Netz soll bereits existierende Programme ohne Anwendungspatches nutzbar machen.
2. **Linux und Windows direkt per USB an das Funkgerät**; Raspberry Pi Zero (2 W) ist *optional* für Dauerbetrieb, Mailbox, WLAN-Bridge und Relais.
3. Primärer erster Hardwarekandidat: **Quansheng UV-K5 V3 mit F4HWN Fusion/Labs KISS-TNC über USB-C**, experimentell und vor Entwicklungsfreigabe real zu prüfen.
4. **Bestehende Projekte nutzen:** `Packet.NET` als AX.25-/KISS-Kandidat; `pdn-net` für standardkonformes IP-over-AX.25; LinBPQ/BPQ für Interoperabilität; Reticulum/NomadNet/LXMF als optionale Integration.
5. **Eigenes Hauptrepository** für Orchestrierung, Discovery, Mesh-Routing, UI und native Dienste. `pdn-net` bei Bedarf **gezielt forken** (etwa Windows-Wintun), möglichst upstream zusammenarbeiten; nicht unnötig alles duplizieren.
6. **C#/.NET 10** für den gemeinsamen Kern; Linux-TUN und Windows-Wintun als Betriebssystem-Adapter; lokale Browseroberfläche.
7. **Offenes Amateurfunkprofil ohne Inhaltsverschlüsselung**; optionales verschlüsseltes Profil nur für **rechtlich geeignete** andere Transportmedien. Nicht auf Basis einer Frequenzbezeichnung allein von Zulässigkeit ausgehen.
8. Konzeptionell **kompakter nativer Chat und Dienstekatalog** neben transparentem IPv4, weil 1200 bit/s für typische Webanwendungen stark begrenzt sind.

## Dokumente

| Datei | Zweck |
|---|---|
| [`docs/PROJECT_HANDOFF.md`](docs/PROJECT_HANDOFF.md) | Vollständige Chat-Zusammenfassung, Anforderungen und Entscheidungen |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Softwareebenen, Multi-Hop, IP-Zuordnung, Discovery, Sicherheit, Integration |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Meilensteine und **testbare Akzeptanzkriterien** |
| [`docs/RESEARCH.md`](docs/RESEARCH.md) | Repositories, verifizierte Funktionen, Lizenzen, Unsicherheiten |
| [`docs/OPEN_QUESTIONS.md`](docs/OPEN_QUESTIONS.md) | Noch zu klärende technische und strategische Punkte |
| [`docs/START_PROMPT.md`](docs/START_PROMPT.md) | Fertige erste Arbeitsanweisung für Codex/Claude |
| [`AGENTS.md`](AGENTS.md) | Repo-Anweisungen für Coding Agents (Codex) |
| [`CLAUDE.md`](CLAUDE.md) | Projektzugang für Claude Code |

## Empfohlener Einstieg lokal

```bash
# In diesem Ordner arbeiten
cat docs/START_PROMPT.md
# Codex oder Claude Code im Projektordner starten; den Prompt übergeben.
```

**Nicht mit einer kompletten Eigenimplementierung anfangen.** Zuerst `Packet.NET`/`pdn-net` auschecken, Versionen/Lizenzen prüfen, Builds/Tests ausführen, APIs inspizieren und ein reproduzierbares Zwei-Knoten-Labortestsetup erstellen. Keine echte Funkaussendung in der ersten Phase.

## Erwartete erste Demo

Zwei simulierte Stationen A und B haben unterschiedliche IPv4-Adressen, teilen einen simulierten KISS/AX.25-Kanal, und ein einfacher HTTP-Client auf A kann **nachweisbar über die virtuelle Funkstrecke** einen minimalistischen HTTP-Server auf B erreichen. Für den ersten Prototyp ist auch ein kleiner UDP/ICMP-Test zulässig, wenn die vorhandene TCP-Tauglichkeit noch ungeklärt ist. Danach reale Quansheng-Hardware, Stationserkennung, Mehrhop.

> Diese Dateien sind eine **Planungs-/Agentenübergabe**, keine fertige lauffähige Implementierung und keine Aussage, dass alle externen Komponenten bereits untereinander getestet wurden.


---

<!-- DATEI: docs/PROJECT_HANDOFF.md -->

# RadioMesh — Zusammenfassung des Planungsgesprächs

Stand: **2026-10-31**. Dieses Dokument soll ohne Zugriff auf den ursprünglichen Chat ausreichen, um das Projekt mit Codex/Claude Code fortzuführen.

## 1. Ursprung des Vorhabens

Ausgangspunkt waren AllStarLink/AllScan URI50 und Winlink an günstigen FM-Handfunkgeräten. Im Gespräch wurde zwischen diesen Systemen unterschieden:

- **AllStarLink/AllScan**: Vor allem sprachbasierte Relais-/Node-Vernetzung; AllScan URI50 ist ein USB-Radiointerface und kann je nach Konfiguration auch Datenanwendungen unterstützen.
- **Winlink**: Nachrichten-/E-Mail-Übertragung via Funk, **keine transparente TCP/IP-Netzwerkkarte**.
- **APRS**: Anwendungen für Positionen, kurze Nachrichten, Telemetrie; nutzt gewöhnlich AX.25-UI-Frames. **AX.25** ist die darunterliegende Link-Schicht und breiter einsetzbar.
- **KISS** = *Keep It Simple, Stupid*: einfaches Host-zu-TNC-Framing für Funkframes; KISS ist **nicht** selbst AX.25, IP oder ein Mesh-Protokoll.

Der Nutzer will über Funk gewöhnliche Netzwerkanwendungen benutzen, insbesondere selbst gehostete **HTTP-/HTML-Webseiten, TCP-/UDP-Server und Clients**, ohne Programme für Packet Radio umschreiben zu müssen.

## 2. Produktvision

Eine Amateurfunkstation soll ein kompatibles Handfunkgerät per USB-C an einen **Linux- oder Windows-Laptop** anschließen, eine Anwendung starten, andere Stationen entdecken und mit ihnen kommunizieren. Benutzer sollen kleine Informationen hosten und öffnen, andere Benutzer anchatten und normale lokale IP-Dienste erreichen können.

Langfristig ist das Netz **dezentral und mesh-artig**: Zielstation C soll auch dann erreichbar sein, wenn A nur Relais B direkt hören kann. Benutzer sehen die Topologie und Dienste in einer grafischen Übersicht. Ein Raspberry Pi Zero (2 W) darf als optionaler, dauerhaft aktiver Knoten Nachrichten zwischenspeichern und verteilen.

## 3. Muss-Anforderungen

- Gemeinsame Codebasis auf **Linux + Windows**, optional Raspberry Pi.
- Direkte Funkanbindung an **Quansheng UV-K5 V3** mit experimenteller KISS-Firmware per USB-C; austauschbarer Backend-Treiber für andere KISS-TNCs/Software-TNCs.
- **IPv4-over-AX.25**, möglichst standardkonform auf Funkseite (UI-Frames, PID `0xCC`; ARP PID `0xCD`, soweit gebraucht).
- Betriebssystem-Netzwerkschnittstelle (Linux TUN; Windows Wintun), sodass Standardprogramme ihre normalen Sockets verwenden.
- Kleine initiale Testkonfiguration mit **fest vergebenen privaten IPv4-Adressen**, z.B. `10.73.0.1` und `10.73.0.2`; später automatische Adressvorschläge, Konflikterkennung, Discovery.
- Automatische Erkennung direkt erreichbarer Stationen und ihrer **Dienste** (Chat, HTTP, Funkseite, Mailbox, Relay); nicht bloß APRS-Ortsbaken.
- Native kurze **Direkt- und ggf. Gruppen-Nachrichten** mit ACK, Duplikatkontrolle und optionaler Speicherung; Kompatibilitätsadapter zu BPQ-Chat prüfen.
- **Mehrhop-Routing** über freiwillig teilnehmende Knoten und nachvollziehbare Pfade; Pakete dürfen nicht endlos kreisen.
- Grafische **Stationsliste + Topologie** mit Routen/Hopzahl, Diensteverzeichnis, Nachrichtenfenster; geographische Karte optional nur mit freiwillig freigegebener Position.
- Einfache **Funk-Landingpage** zusätzlich zu gewöhnlichem HTTP; Inhalte klein, cachebar, offen dokumentiert.
- Offen dokumentierte Protokolle und gute Onboarding-Erfahrung; herstellerunabhängiges Projekt.

## 4. Soll-/Später-Funktionen

- Optionale Raspberry-Pi-Bridge und 24/7-Knoten; Store-and-Forward-Mailbox für offline Empfänger.
- BPQ32/LinBPQ-Knoten und Chat-/Mailboxdienste **integrieren statt neu implementieren**, wo Schnittstellen taugen; Fork nur bei Bedarf.
- Reticulum-/NomadNet-/LXMF-Adapter für kompatible Netze und Nachrichten; nicht als Ersatz für IPv4.
- Optionales Ende-zu-Ende-Verschlüsselungsprofil auf **anderen dafür legal geeigneten Transporten**; ohne Amateurfunkrufzeichen in Nicht-Amateurfunkumgebungen über andere Identitätsformen.
- Mehrere Funkports, unterschiedliche Geschwindigkeiten, spätere robustere Modems statt nur 1200-Baud-AFSK.
- Installation via deb/rpm und Windows-Installer; Logging, Netzwerkdiagnose, grafische Konfiguration.

## 5. Architekturentscheidung / Fork-Strategie

**Eigenes Hauptrepository `RadioMesh`** (Arbeitstitel). `Packet.NET` als Kandidat für AX.25/KISS/NET-ROM, `pdn-net` als Host-IP-Adapter. Aktuell beschreiben die Upstream-Repositories bereits viel Nutzbares. Daher nicht die gesamte Anwendung in einen Fork von `pdn-net` pressen. Änderungen an pdn-net (z.B. `ITunDevice`/Wintun) als separaten Branch/Fork entwickeln, dokumentieren und geeignete PRs upstream anbieten. Vor Abhängigkeiten/Lizenzierung die konkreten Schnittstellen in Quellcode und Tests prüfen.

Es ist **noch nicht entschieden**, ob das erste Release als Wrapper um separat laufende Dienste beginnt oder durch Bibliotheksintegration ein einziges Binary wird. Als Start ist Prozess-/Adapter-Trennung risikoärmer.

## 6. Grenzen der ursprünglichen Planung / wichtige Korrekturen

- **Nicht automatisch gegeben:** KISS-Kompatibilität des Quansheng bedeutet noch nicht, dass dessen Firmware beliebige AX.25-IP-Frames, wiederholtes Senden/Empfangen, große Frames und Dauerlast zuverlässig schafft. Erst labortesten.
- **Nicht automatisch gegeben:** NET/ROM-Routing und AX.25-IP-Weiterleitung sind **nicht dasselbe**. Mehrhop-IP muss separat nachgewiesen werden (TTL, Forwarding, Routen, Addressierung, Loops, Paketverluste).
- **Nicht automatisch gegeben:** Ein virtuelles IP-Interface macht 1200-bit/s-Funk nicht zu einem breitbandigen Internet. TCP kann extrem langsam, unzuverlässig und ineffizient werden.
- **Nicht automatisch gegeben:** Ein dauerhaft aktiver Knoten darf in Deutschland einfach so betrieben werden. Rechtsrahmen für automatisch arbeitende Amateurfunkstationen prüfen.
- **Nicht automatisch gegeben:** Reticulum/LXMF kann verschlüsselt im deutschen Amateurfunk laufen. Verschlüsselung gewöhnlicher Inhalte ist dort grundsätzlich untersagt.
- **Keine zufällige 44.x-Adresse verwenden:** AMPRNet/44Net hat einen eigenen Vergabeprozess. Anfangs RFC1918-Labornetz mit bewusst vergebenen Adressen.
- **Kein Broadcast-Sturm:** IPv6/mDNS/NetBIOS/SSDP/ARP-Broadcasts dürfen die Funkstrecke nicht fluten.

## 7. Erwartetes Benutzererlebnis (Beispiel)

1. Benutzer installiert RadioMesh auf Fedora oder Windows.
2. Er wählt das USB-KISS-TNC und konfiguriert ein legales Funkprofil plus Rufzeichen, ggf. stationsspezifische SSID.
3. RadioMesh zeigt die lokale Station und erkannte Nachbarn; Name und Dienste erscheinen nach Ankündigung/Abfrage.
4. Bei Auswahl eines Nachbarn kann ein **nativer kurzer Chat** gestartet werden.
5. Ein verfügbarer HTTP-Dienst kann über seine **private Funk-IP** mit Browser/`curl` aufgerufen werden; einfache Funkseiten sind separat optimiert.
6. Bei einem 2-Hop-Ziel wird die Route via vermittelndem Knoten gezeigt; Relay ist opt-in.
7. Optional: ständig laufender Pi-Node hält Nachrichten für Offline-Empfänger vor.

## 8. Vorläufiger Technologiestack

- Kern: C#/.NET 10, `Packet.NET`, `pdn-net`, Tests mit xUnit.
- Netzwerk: standardkonformes AX.25-IP; TUN/Wintun; RHPv2 zwischen `pdn-net` und `pdn` nach Upstream-Dokumentation.
- UI: lokaler HTTP-Dienst (ASP.NET Core denkbar) + schlankes TypeScript/HTML-Frontend.
- Test: virtuelle KISS-/Kanalemulation, Paketverlust/Latenz/Busy-Channel, 2- und 3-Knoten-Szenarien.
- Ports: USB-Serial-KISS, ggf. TCP-KISS, später Dire Wolf als TNC.

## 9. Nicht-Ziele der ersten Version

- Kein universeller Ersatz für das öffentliche Internet oder vollständiger Webbrowsing-Proxy.
- Kein automatisch weitergeleiteter unkontrollierter Internetverkehr.
- Keine eigene Krypto entwickeln.
- Keine stabile Mesh-Netz-Behauptung ohne Tests.
- Kein Fork sämtlicher Upstream-Repositories ohne konkreten Bedarf.
- Keine dauerhaft unbeaufsichtigte Funksendung ohne Prüfung der regulatorischen Voraussetzungen.

## 10. Erfolgskriterien

**MVP:** Linux↔Linux über simulierte AX.25/KISS-Übertragung mit zwei IP-Endpunkten, funktionierender nachgewiesener UDP/ICMP-Übertragung und – soweit Funkbandbreite und Stack es zulassen – kleinem HTTP-Request/Response; Stations-Erkennung plus kompakter Chat; reproduzierbare Installationsanleitung. Danach Hardwaretest und Windows.

**Produktziel:** Dieselbe RadioMesh-Anwendung arbeitet unter Linux/Windows mit realen KISS-Geräten; ab drei Stationen sind Mesh-Weiterleitung, Topologievisualisierung, Dienstekatalog und Standard-IP-Verkehr zuverlässig demonstriert.

Siehe `ARCHITECTURE.md`, `ROADMAP.md`, `RESEARCH.md` und `OPEN_QUESTIONS.md`.


---

<!-- DATEI: docs/ARCHITECTURE.md -->

# RadioMesh — Technische Zielarchitektur (Entwurf)

## Leitprinzipien

1. **Interoperabilität vor Neuerfindung:** Bestehende AX.25-IP-Datagrammkonventionen beibehalten; KISS ist nur der Modemtransport.
2. **Schichten entkoppeln:** Hardwaretreiber, AX.25, IPv4, Mesh, Dienste und UI testbar separieren.
3. **Optionales Mesh** darf Standard-IP-Dienste nicht verhindern. Bestehende Packet-Radio-Stationen ohne RadioMesh sollen weiter nutzbar sein, soweit die verwendeten Protokolle kompatibel sind.
4. **Durchsatz respektieren:** Bei 1200 bit/s maximal 150 Byte/s brutto vor Overhead; ein Mehrhop-Relais auf demselben Kanal erhöht die Funkzeit erheblich.
5. **Sichere Standards:** Keine unbeabsichtigten Pakete im Amateurfunk senden; Sende-/Relay-Freigaben, Filter und Profile.

## Schichtenmodell

```text
   GUI: Netzwerkgraph | Stationsverzeichnis | Chat | Funkseiten | Status
                         |                 |
          Native Dienste (Discovery, Chat, Cache, Mailbox)
                         |                 |
              Mesh-/Routing-Control-Plane  |   Standard-Sockets
                         |                 v
                         |            TCP / UDP / HTTP
                         |                 |
                         |       OS IPv4 Stack / radio0
                         |                 |
                         |        pdn-net TUN/Wintun
                         |                 |
                         +----> pdn / Packet.NET via RHPv2
                                     |
                             AX.25 UI / NET-ROM*
                                     |
                              KISS Serial/TCP
                                     |
                              Quansheng / TNC

* NET/ROM als zu evaluierende Routing-Option, NICHT automatisch IPv4-Mesh.
```

### Erste Implementierungsform (Adaptersystem)

- `pdn`/Packet.NET läuft als separater Prozess und besitzt den direkten KISS-Port.
- `pdn-net` läuft als separater IP-Adapter und redet mit `pdn` via **RHPv2** (laut Upstream lokales TCP mit JSON-Framing).
- `RadioMesh` läuft als eigener Dienst, liest Stationsereignisse, richtet konfigurierte IP-Routen ein, führt Dienste/Discovery/Chat und UI; detaillierte RHPv2-Fähigkeiten erst im Code prüfen.
- Keine konkurrierenden seriellen Portzugriffe: **ein einziger Hardware-Eigentümer**. Für Reticulum oder andere Protokolle ggf. Frame-Multiplexing mit klarer Rückkanalzuordnung / eigene Funkports.

## IPv4 und Adressierung

### Erste Phase: statisch, privat

Beispiel **nur für Labortests**:

| Station | AX.25-Identität | IPv4-Hostadresse |
|---|---|---|
| A | TESTA-1 | `10.73.0.1/32` |
| B | TESTB-1 | `10.73.0.2/32` |
| C | TESTC-1 | `10.73.0.3/32` |

Für den 2-Knoten-Test kann man alternativ `10.73.0.0/30` verwenden. Einzelne `/32`-Routen vermeiden unter Umständen die falsche Vorstellung eines normalen Ethernet-Broadcast-Domains. Tests auf den vorgesehenen Interfaces/Routen sorgfältig konfigurieren.

Die Ziel-IPv4-Adresse bleibt bei Mehrhop über die Strecke erhalten. Die nächste **AX.25-Zieladresse** kann sich von der endgültigen IP-Zielstation unterscheiden. IPv4-Forwarding muss TTL reduzieren, Loop-Schutz und Routen unterstützen. `pdn-net` bietet bereits IP->Callsign-Mapping und ARP-Lernen, aber bislang kein komplettes automatisches Multi-Hop-IP-Routing.

### Später: automatische Adressvergabe

Ein klar dokumentierter Bereich innerhalb RFC1918, **nicht** frei gewählte öffentliche 44Net-Adressen. Ersten Adresskandidaten aus Station-ID ableiten, **Kollisionen** durch Announcement/Probe und Adresskonfliktdetektion behandeln. Alternative zu eigener Logik: IPv4LL oder zentral zugewiesene DHCP-Leases für spezielle Topologien evaluieren; diese Mechanismen sind nicht ohne Weiteres für Mehrhop-Funk geeignet. Routing darf keine Verbindung zwischen allen Stationen auf Layer 2 voraussetzen.

## Discovery und Diensteverzeichnis

### Protokollkonzept — nicht final!

- Eigene, dokumentierte RadioMesh-Discovery-Nachrichten über passende AX.25-UI-Frames, nicht mit APRS-Inhalten verwechseln.
- Minimale Nutzdaten: Protokollversion, Rufzeichen/Node-ID, Sequenz, Ablaufzeit/TTL, Fähigkeiten-/Dienstebitset, optionale IPv4-Adresse, optional Anzeigename, optionale Service-Abfrage.
- Seltene, **zufällig verzögerte** Bekanntmachungen; passives Mithören bevorzugen; Antworten auf gezielte Anfrage; veraltete Einträge verfallen.
- Anzeigen/Diagnose dürfen eine zuletzt gesehene Station nicht mit **derzeit zuverlässig erreichbar** gleichsetzen.
- Gesonderte **Routing-Announcements**, deren Reichweite begrenzt und dedupliziert ist.
- Künftiges Multi-Hop braucht explizite Überlegungen zu versteckten Stationen, geteilten Rufzeichen/SSIDs, Routing-Stabilität, verzögerter Zustellung und Kosten pro Hop.

## Mesh-Routing

**Weder vorhandenes NET/ROM noch einfache AX.25-Digipeater automatisch als transparentes IP-Mesh betrachten.** Zu prüfen:

- Nutzung von vorhandenen NET/ROM-Nachbarschafts- und Routeninformationen als Input einer IPv4-Forwarding-Schicht (Interoperabilität vs. Protokolloverhead).
- Alternativ einfaches, quell-/zielorientiertes **reaktives Routing** mit begrenzter Routensuche statt periodischem Flooding. Keine voreilige Festlegung auf AODV o.ä.; erst simulieren und evaluieren.
- Jeder Relay-Node: separate Routingtabelle, Loop-Schutz, Hop-Limit / IPv4-TTL, Kanalzugriff, Wiederholungs-/Retry-Limits und Warteschlange.
- Weiterleitung **freiwillig** und pro Transport/pro Frequenz absicherbar.
- Routingmetriken: Hops, geschätzte Erfolgsrate, Belegungszeit, RTT / gemessene Paketverluste; kein unkritisches RSSI-Äquivalent behaupten.
- Tests A↔B↔C, A nicht direkt C; danach Linkausfall B und alternative Route bei 4 Knoten.

## Native Messaging / Chat

- Separat von transparentem TCP/IP, für kurze Nachrichten effizient.
- Nachrichten-ID, Sender, Empfänger/Gruppe, Zeitinformation wenn vorhanden, Payload-Limit, ACK und Retransmission mit Backoff; dedup und Idempotenz.
- Chat/Anwendungs-Zustellbestätigung von RF/AX.25-Frame-ACK unterscheiden.
- Store-and-Forward später: Empfang durch Zwischenknoten, sichere Persistenz, maximale Lebensdauer/Queue-Größe, Authentizität/Spam-/Relay-Policy. Normale TCP-Verbindungen **nicht** store-and-forward-fähig.
- BPQ-Chat/Mailbox als separates Gateway evaluieren; keine vollständige native Kompatibilität annehmen.

## Landingpages & HTTP

- Standard-HTTP: Jeder Teilnehmer kann auf `10.x.x.x:PORT` einen bestehenden Webserver hosten. Bei 1200 bit/s nur **minimalistische** Seiten sinnvoll, keine großen JS-Bundles, Bilder oder zahlreiche Requests.
- Funk-Landingpage: optionales kompaktes format-/versionsdefiniertes Protokoll, cachebar, ggf. textorientiertes Markup mit maximalem Inhaltsbudget und Abruf auf Anfrage.
- Service-Discovery trennt `http` und `radio-page`; keine stillschweigende Übersetzung aller externen Internetdienste.
- Bewusst keine Verschlüsselung/Inhaltsverschleierung über deutschen Amateurfunk; außerdem Dienste whitelist-basiert und lokal begrenzter Webserver-Zugriff.

## Windows/Linux

| Thema | Linux | Windows |
|---|---|---|
| USB-KISS | `/dev/ttyACM*` oder anderer tty | `COM*` |
| IP-Interface | `/dev/net/tun` | Wintun (noch zu implementieren/prüfen) |
| Rechte | `CAP_NET_ADMIN`/privilegierter Helper | Admin/Treiberinstallation |
| Dienst | systemd (optional) | Windows Service (optional) |
| UI | localhost HTTP | localhost HTTP |

Hinweis: Die Upstream-`pdn-net`-Implementierung ist derzeit **Linux-only** hinsichtlich TUN; Windows darf nicht als fertig beschrieben werden.

## Reticulum/NomadNet

- Separater optionaler Adapter/Sidecar; Reticulum/LXMF **nicht** als IP-Stack-Ersatz.
- Möglichkeiten: Reticulum über einen dafür geeigneten Nicht-Amateurfunk-IP-Transport; direkter KISS-Austausch nur mit kontrolliertem gemeinsamem Modemzugriff; LXMF-Nachrichten-Bridge; NomadNet-Seitenrendering.
- Über Amateurfunk in Deutschland **keine gewöhnlich verschlüsselten Reticulum-Nutzdaten** aussenden.
- Lizenz/Compliance der aktuellen Reticulum-Referenzimplementierung vor Bundle/Weiterverteilung prüfen.

## Ressourcen- und Rechtsgrenzen

- §13 AFuV für automatisch arbeitende Stationen und §16 AFuV (u.a. offener Verkehr/Bestimmbarkeit, Verbot der Inhaltsverschlüsselung, technische Pflichten) ausdrücklich berücksichtigen.
- „ISM“ bedeutet nicht automatisch, dass Quansheng-Firmware, Leistung, Frequenz und Verschlüsselung frei wählbar sind. Konforme Funkhardware und Bedingungen der Allgemeinzuteilung sind Voraussetzung.
- Nur bewusst konfigurierte Aussendungen; „Simulation only“ ist Default bis Betreiber eine legale Funkkonfiguration erstellt hat.
- Keine globalen Default-Routen ins Funknetz, kein unkontrolliertes VPN/HTTPS, keine offenen transparenten Internet-Gateways.

## Erste zu definierende Software-Interfaces (Vorschläge)

```text
IKissTransport       Open/Close/SendFrame/ReceiveFrame; Diagnostic
IAx25Gateway         SendUiFrame/SubscribeFrames; Callsign
IIPNetworkAdapter    Open/SetMtu/AddAddress/AddRoute/Close
IStationRegistry     Upsert/Expire/List/OnStationChanged
IRouteResolver       ResolveNextHop(destination)/UpdateNeighbors
IServiceDirectory    Announce/Query/ListByStation
IMessageService      Send/Ack/Inbox/Outbox
ITransportPolicy     CanTransmit/CanRelay/EncryptionAllowed
```

Der echte RHPv2-/Packet.NET-Code bestimmt später die tatsächlichen Interfaces. Dies ist **kein fertiges API-Design**.


---

<!-- DATEI: docs/ROADMAP.md -->

# RadioMesh — Umsetzungsroadmap mit Abnahmekriterien

Jede Phase erhält eine reproduzierbare Demo und Tests. Kein Funkversuch erforderlich, bis der Laborpfad stabil ist und die rechtlichen Voraussetzungen geklärt sind.

## M0 — Upstream-Audit / keine Eigenimplementierung (sofort)

**Aufgaben**
- Exakte `packet.net`- und `pdn-net`-Commit-IDs, Ziel-.NET-Version und Lizenzdateien dokumentieren.
- Repositories klonen und `dotnet build`, `dotnet test` ausführen; `Packet.NET`-RHPv2, KISS und `pdn-net`-`ITunDevice` lesen.
- Testen, ob `pdn-net` als eigenständiger Prozess mit `pdn` funktioniert; konkrete API-Seams dokumentieren.
- Quansheng F4HWN Firmware-Releasestand, Hardwarevariante und USB-KISS-Protokoll **an Originalquelle** prüfen.
- Vergleichstest alten Linux-AX.25-Kernel-Stacks, `tncattach`, BPQ, Packet.NET: Funktionen, Interoperabilität, Wartungsrisiko.
- AGPL-3.0, BPQ GPL-3.0-or-later und weitere Abhängigkeiten/Lizenzen vor Wiederverwendung dokumentieren.

**Abnahme:** `docs/UPSTREAM_AUDIT.md` mit Repo-URLs, Hashes, Reproduktionsbefehlen, Testergebnissen, Trennstellen, Risiken und Entscheidung „als Abhängigkeit / gezielter Fork / nicht übernehmen“.

## M1 — Virtuelle Funkstrecke und IP-Basis

**Aufgaben**
- Zwei simulierte Packet.NET/KISS-Knoten über einen konfigurierbaren Fake-RF-Kanal (Latenz, Drop-Rate, Halbduplex/Busy) verbinden.
- Reale Ethernet-/Internet-Route nicht versehentlich als Abkürzung verwenden; testbare Isolation.
- Private IPv4-Ziele und RHPv2/pdn-net konfigurieren, standardkonforme AX.25-IP-Frames im Trace nachweisen.
- ICMP oder UDP über den simulierten Funkkanal bestätigen; anschließend sehr kleinen HTTP-GET über TCP testen.
- Funk-Overhead, Größe, Fehler und Retransmission messen.

**Abnahme:** `./scripts/test-two-nodes.sh` oder gleichwertig mit reproduzierbarer Demo A→B, Parser-/Integrations-Tests und PCAP/KISS-/AX.25-Nachweis. Wenn HTTP fehlschlägt, Ursache dokumentieren statt Test als bestanden markieren.

## M2 — Quansheng-Funkhardware (bewusst kontrolliert)

**Aufgaben**
- Zwei UV-K5 V3 mit exakt unterstützter Firmware; USB-Port enumerieren; Framing/escaping, Baudrate, KISS-Parameter, Limits nachprüfen.
- KISS-RX/TX zunächst mit Testframes, danach AX.25-IP/UDP und HTTP in kleiner Last.
- Störverhalten USB/HF, PTT-Umschaltung, Squelch, Kanalbelegung, BER, max. Frame und Dauerlast messen.
- Funkbetrieb nur in entsprechend zulässiger, beaufsichtigter Konfiguration.

**Abnahme:** Gleiche minimalistische Anwendung funktioniert **nachweisbar** über zwei echte Handfunkgeräte; Leistungs-/Fehlerprotokoll vorhanden. Bei inkompatibler Firmware standardkonformes Alternativ-TNC einsetzen, statt Systemdesign zu verbiegen.

## M3 — Native Discovery, Diensteverzeichnis und Chat

**Aufgaben**
- Versioniertes kompaktes Protokoll spezifizieren; Frame IDs/PIDs/Subtypen korrekt auswählen und dokumentieren.
- Seltene Hello-/Service-Ankündigungen, gezielte Abfragen, Zufallsbackoff, TTL und Duplikatschutz.
- Einfache P2P-Nachrichten mit ID/ACK/Retry, Inbox/Outbox und Protokolltests.
- GUI-Prototyp: erkannte Stationen, Zustände, Dienste, Chat.

**Abnahme:** Zwei simulierte Stationen erkennen sich ohne manuelle Peer-Liste; Nachricht wird sicher einmal angezeigt, auch wenn Trägerframes doppelt ankommen; Kontrollverkehr bei 1200-bit/s-Budget begrenzt.

## M4 — Windows direkt mit KISS und Wintun

**Aufgaben**
- `pdn-net`-`ITunDevice`-Adapter mit Windows/Wintun implementieren oder bestehende Upstream-Lösung nutzen, wenn inzwischen vorhanden.
- COM-Port-/Treiber-Setup; Hilfsprogramm mit notwendigen Rechten; kein Installer erzwingt globales Routing.
- Linux-Windows-Simulator-Test, anschließend Hardwaretest.

**Abnahme:** Ein Windows-PC und ein Linux-PC können je mit KISS-Gerät und virtueller IPv4-Schnittstelle **normale lokale TCP-/UDP-Anwendungen** erreichen. Dokumentierter Installations- und Deinstallationspfad.

## M5 — 3+ Knoten, transparentes IPv4-Mesh

**Aufgaben**
- Szenario A↔B↔C, A nicht direkt C; B freiwilliger Router.
- NET/ROM-Routing auf Wiederverwendbarkeit prüfen; sonst schmalen eigenen Routingdienst in RadioMesh spezifizieren.
- IP-Ziel vs. Next-Hop-Callsign trennen; IP-Forwarding, TTL, Route Expiration, Loop/Duplication-Schutz.
- Link-Ausfall, Paketverlust und halbduplexbedingte Engpässe simulieren.

**Abnahme:** `curl`/UDP von A zu C geht ausschließlich via B; RF-Trace zeigt beide Teilstrecken; bei B-Ausfall keine Endlosschleife; Routenanzeige korrekt.

## M6 — Webseiten, Topologie, BPQ-Interoperabilität

**Aufgaben**
- Netzwerkgraph/Stationsliste, Filter für erreichbare Services, Browser-Einstieg für HTTP.
- Tiny-Radiopage in offen dokumentiertem Format, ggf. clientseitiger Cache.
- BPQ-Chat und Mailbox-Schnittstellen prüfen, vorhandene Protokolle verwenden oder Bridge schreiben.

**Abnahme:** HTTP und Radio-Page separat auswählbar; Services öffentlich dokumentiert; mindestens eine nachweislich kompatible BPQ-Verbindung oder ehrliche dokumentierte Einschränkung.

## M7 — 24/7 Pi, Offline-Mailbox, zusätzliche Transportprofile

**Aufgaben**
- Headless-Modus auf Pi Zero 2 W evaluieren (RAM/CPU/USB/WLAN, Robustheit), ggf. stärkeren Pi als Referenz nutzen.
- Persistente Queue, Ablaufzeit, Zustellbestätigung, Weiterleitungsfreigabe und Speicherlimits.
- Reticulum-/NomadNet-/LXMF-Integration separat testen, nur auf legal geeigneten Netzen; Verschlüsselungsprofil getrennt halten.
- Release-Pipeline für Linux/Windows, Paket-/Installersignatur, Nutzeranleitung.

**Abnahme:** Nachrichten werden bei offline Empfänger gespeichert und nach Wiederverbinden zugestellt; installierbare Beta mit dokumentierten Betriebsprofilen.

## Risiken und Mitigationen

| Risiko | Maßnahme |
|---|---|
| Firmware sendet nur eingeschränkte AX.25-Frames | Hardware-Matrix, KISS-Fallback über Dire Wolf oder anderes TNC |
| TCP bei 1200 bit/s unbrauchbar | Kein SLA versprechen; kleine Seiten, MTU/Timer kontrollieren, native Nachrichten/Radio-Page |
| Mesh-Kontrollverkehr belegt ganzen Kanal | Intervallbegrenzung, gerichtete Queries, Simulationsbudget, Sendezeitmetriken |
| BPQ- und NET/ROM-Protokolle passen nicht unmittelbar | Adapter statt invasive Forks, Interop-Tests |
| AGPL/GPL-Pflichten | Lizenz-Audit vor Distribution, Architekturlizenz festlegen |
| Regulatorische Konflikte | Simulationsdefault, Funkprofile, getrennte Verschlüsselungstransporte und Betreiberhinweise |


---

<!-- DATEI: docs/RESEARCH.md -->

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


---

<!-- DATEI: docs/OPEN_QUESTIONS.md -->

# RadioMesh — Offene Architekturentscheidungen

Diese Fragen bei der Code-Recherche klären. Bis dahin keine Annahmen in produktivem Code festschreiben.

## Sehr früh klären

1. **Realer `pdn-net`-Seam**: Genügt RHPv2 für Discovery-/Chat-Frames/Monitor-/Routinginformationen oder brauchen wir zusätzliche `Packet.NET`-APIs? Wird mehr als ein RHPv2-Client unterstützt? Wie werden AX.25-UI-Daten-PIDs exponiert?
2. **Windows**: Kann `pdn-net` über `ITunDevice` sauber Wintun nutzen? Rechte/MTU/Packet-Buffer-Richtung/Lebenszyklus prüfen.
3. **Hardware**: Ist F4HWN-Labs-KISS auf einem echten **Quansheng UV-K5 V3** bidirektional für andere AX.25-UI-PIDs als APRS robust? Wie hoch ist die echte maximale Framegröße? KISS CRC-Verhalten: FCS vom TNC entfernt/angehängt?
4. **Multi-Hop**: Kann vorhandenes NET/ROM mit IP-Layer gekoppelt werden oder sind eigene next-hop-gesteuerte AX.25-Frames nötig? Wie sehen getrennte RX-/TX-Queues und Hop-limit aus?
5. **Topologie**: Bevorzugen wir für das erste Mesh reaktives Routing, kurze periodische Updates, NET/ROM-Integration oder eine Kombination? Realistische Kanalbelegungsgrenzen testen.
6. **IPv4-Adressen**: Anfangs feste RFC1918-/32-Hostrouten. Ab welchem Meilenstein automatische Adressen und Kollisionserkennung? Wie verhindern wir doppelte IP bei nicht gegenseitig hörbaren Stationen?
7. **Mehrbenutzer**: Ist ein Rufzeichen/SSID eine Station/Node-Identität? Wie sollen mehrere Laptops hinter einem Node adressiert werden? Wann NAT vs. Routing? Welche Node-IDs gibt es außerhalb des Amateurfunks?
8. **Eigenlizenz**: Projektrepository-Lizenz nach Dependency-/Kompatibilitätsaudit entscheiden. Keine Lizenz voreilig in das Hauptprojekt kopieren.

## Danach klären

9. Chat: BPQ vorhandene Chat-Protokolle direkt unterstützen oder native App + begrenzter BPQ-Bridge? Welche versionierten Wire-IDs/PIDs?
10. Native Funkseiten: einfaches Markdown, begrenztes HTML oder NomadNet-ähnliches Markup? Wie sichere/regelkonforme Darstellung, Caching und Inhalte beschränken?
11. Reticulum-Nachrichten/Gateways: Welche offiziellen Schnittstellen? Wie rechtlich saubere Transportprofil-Trennung? Sidecar vs. direkte KISS-Frames? Lizenz vor Bundle prüfen.
12. Security: Vertrauensmodell für Bekanntmachungen und Routen (Spoofing, Spam, Replay, Re-Routing, falsche Service-Announcements) im offen lesbaren Amateurfunk.
13. Betriebsprofile: Amateurfunk DACH (offen), Simulationsmodus, berechtigter anderer Datenfunktransport mit optionaler Ende-zu-Ende-Verschlüsselung. Kein pauschaler ISM-Freibrief.
14. UI: Graphdarstellung ohne geographische Daten als Default; Positionen nur Opt-in. Persistenzdaten, Datenschutz, Log-Retention.
15. Bandbreite: Leistungsziel für 1/2/3 Hops, Packet-Size/Retry, Priority Queue, Token-/Airtime-Budget, gute Interaktivität.
16. Veröffentlichung: App-Installer, `dnf`/`deb`, Windows-Signing, automatische Updates ohne automatisches Funksenden.


---

<!-- DATEI: docs/START_PROMPT.md -->

# Fertiger Prompt für Codex oder Claude Code

```text
Du bist der leitende Softwarearchitekt und Implementierer für das Open-Source-Projekt RadioMesh.

Lies zuerst README.md, AGENTS.md, docs/PROJECT_HANDOFF.md, docs/ARCHITECTURE.md, docs/ROADMAP.md, docs/RESEARCH.md und docs/OPEN_QUESTIONS.md vollständig. Diese Dateien fassen frühere Planungsgespräche zusammen. In diesem Repository ist derzeit noch KEIN fertiger RadioMesh-Code.

Ziel ist ein plattformübergreifender AX.25/KISS-basierter Funk-Mesh-Netzwerkstack für Linux und Windows, der herkömmliche IPv4/TCP/UDP/HTTP-Anwendungen über reale Funkgeräte nutzbar macht und später Discovery, Chat, Landingpages, BPQ-/Reticulum-Interoperabilität, Mesh-Routing und einen Pi-Zero-Dauerknoten ergänzt.

STARTAUFGABE: Bearbeite Roadmap-Meilenstein M0, anschließend sofern ausführbar den kleinstmöglichen M1-Nachweis.

1. Klone oder inspiziere Packet.NET (https://github.com/packet-net/packet.net) und pdn-net (https://github.com/packet-net/pdn-net), dokumentiere Commit-IDs und Lizenzen.
2. Baue und teste beide Repositories mit .NET 10. Dokumentiere tatsächlich ausgeführte Befehle/Ergebnisse. Wenn kein Netz/SDK vorhanden ist, dokumentiere die Blockade und setze ein reproduzierbares lokales Testkonzept auf.
3. Untersuche die RHPv2-Verbindung (pdn-net zu pdn), KISS-Anbindung, AX.25-UI/PID 0xCC, Linux-TUN-Schnittstelle und mögliche Erweiterungspunkte für Windows-Wintun und eigene native Funkdienste.
4. Vergleiche vorhandene Basisfunktionen mit unseren Anforderungen; insbesondere Mehrhop-Routing und BPQ-Konnektivität dürfen nicht ohne Nachweis als implementiert gelten.
5. Erstelle docs/UPSTREAM_AUDIT.md mit Quellen, Befunden, offenen Risiken und einem minimalen Implementierungsplan für einen simulierten A↔B IP-Link.
6. Baue, falls ohne großen Eingriff möglich, eine erste isolierte Zwei-Knoten-Simulation mit nachvollziehbarem KISS-/AX.25-Traffic. Falls nicht, liefere ein überprüfbares Test-Harness-Konzept und rationale Next Steps.

WICHTIGE LEITLINIEN:
- Nicht unnötig neu erfinden oder einen gigantischen Fork starten. Unser Hauptrepo ist eigenständig, pdn-net wird ggf. gezielt geforkt; Upstream-Verbesserungen bevorzugen.
- Kein realer Funksendebetrieb in dieser Phase; nur Simulation und unkritische Softwaretests.
- Keine Inhaltsverschlüsselung gewöhnlicher Daten im deutschen Amateurfunk; andere Profile nur auf legal geeigneten Transporten und später.
- Kein offizielles 44Net-Netz ohne Zuteilung. Für Tests private IPv4-Adressen, etwa 10.73.0.1 / 10.73.0.2.
- Protokoll- und Betriebssystemunterschiede sauber kapseln. C#/.NET 10 bevorzugt.
- Schreib Tests. Unterscheide klar 'Bestand upstream', 'funktioniert in unserem Test', 'vorgeschlagen' und 'unbestätigt'.

Starte mit einer kompakten Zusammenfassung der bereits im Projekt getroffenen Entscheidungen und dann führe M0 aus. Stelle keine Rückfragen, sofern sich eine vernünftige reversible Entscheidung dokumentieren lässt.
```


---

<!-- DATEI: AGENTS.md -->

# RadioMesh — Agent Instructions (Codex / other coding agents)

## Mission

Build an open, portable AX.25/KISS/IP radio mesh platform. Reuse rather than rewrite existing Packet.NET/pdn-net components. **Normal IP applications must keep working.** See `README.md`, `docs/PROJECT_HANDOFF.md`, `docs/ARCHITECTURE.md`, `docs/ROADMAP.md`, `docs/RESEARCH.md` and `docs/OPEN_QUESTIONS.md` before editing code.

## Current state

This handoff **contains documentation only**. No project implementation, checked-out upstream sources, compiled binaries or hardware tests are included. Do not falsely claim anything works.

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


---

<!-- DATEI: CLAUDE.md -->

# Claude Code — RadioMesh

Lies zuerst `AGENTS.md` und dann `docs/START_PROMPT.md`. Die vollständige Aufgabenübergabe steht in `README.md` und `docs/PROJECT_HANDOFF.md`. Der aktuelle Ordner enthält ausschließlich **Konzeptdokumente**, noch keinen ausführbaren Code.

Arbeite in kleinen testbaren Schritten, verwende bestehende Packet.NET-/pdn-net-Komponenten, evaluiere Upstream vor Forks, und erfinde keine erfolgreichen Tests oder Hardware-Kompatibilität. Beachte die rechtlich eingeschränkten Funk- und Verschlüsselungsprofile sowie die AGPL-/GPL-Lizenzen.

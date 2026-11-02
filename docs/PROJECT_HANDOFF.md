# AXLink — Zusammenfassung des Planungsgesprächs

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

**Eigenes Hauptrepository `AXLink`** (Arbeitstitel). `Packet.NET` als Kandidat für AX.25/KISS/NET-ROM, `pdn-net` als Host-IP-Adapter. Aktuell beschreiben die Upstream-Repositories bereits viel Nutzbares. Daher nicht die gesamte Anwendung in einen Fork von `pdn-net` pressen. Änderungen an pdn-net (z.B. `ITunDevice`/Wintun) als separaten Branch/Fork entwickeln, dokumentieren und geeignete PRs upstream anbieten. Vor Abhängigkeiten/Lizenzierung die konkreten Schnittstellen in Quellcode und Tests prüfen.

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

1. Benutzer installiert AXLink auf Fedora oder Windows.
2. Er wählt das USB-KISS-TNC und konfiguriert ein legales Funkprofil plus Rufzeichen, ggf. stationsspezifische SSID.
3. AXLink zeigt die lokale Station und erkannte Nachbarn; Name und Dienste erscheinen nach Ankündigung/Abfrage.
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

**Produktziel:** Dieselbe AXLink-Anwendung arbeitet unter Linux/Windows mit realen KISS-Geräten; ab drei Stationen sind Mesh-Weiterleitung, Topologievisualisierung, Dienstekatalog und Standard-IP-Verkehr zuverlässig demonstriert.

Siehe `ARCHITECTURE.md`, `ROADMAP.md`, `RESEARCH.md` und `OPEN_QUESTIONS.md`.

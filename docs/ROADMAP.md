# AXLink — Umsetzungsroadmap mit Abnahmekriterien

Jede Phase erhält eine reproduzierbare Demo und Tests. Kein Funkversuch erforderlich, bis der Laborpfad stabil ist und die rechtlichen Voraussetzungen geklärt sind.

## M0 — Upstream-Audit / keine Eigenimplementierung (sofort)

**Stand 31.10.2026:** gepinnte Builds/Unit-Tests und F1-Rot/Grün ausgeführt;
vollständiger Lizenz-/Gegenstellen-Audit noch offen. Details: `UPSTREAM_AUDIT.md`.

**Aufgaben**
- Exakte `packet.net`- und `pdn-net`-Commit-IDs, Ziel-.NET-Version und Lizenzdateien dokumentieren.
- Repositories klonen und `dotnet build`, `dotnet test` ausführen; `Packet.NET`-RHPv2, KISS und `pdn-net`-`ITunDevice` lesen.
- Testen, ob `pdn-net` als eigenständiger Prozess mit `pdn` funktioniert; konkrete API-Seams dokumentieren.
- Quansheng F4HWN Firmware-Releasestand, Hardwarevariante und USB-KISS-Protokoll **an Originalquelle** prüfen.
- Vergleichstest alten Linux-AX.25-Kernel-Stacks, `tncattach`, BPQ, Packet.NET: Funktionen, Interoperabilität, Wartungsrisiko.
- AGPL-3.0, BPQ GPL-3.0-or-later und weitere Abhängigkeiten/Lizenzen vor Wiederverwendung dokumentieren.

**Abnahme:** `docs/UPSTREAM_AUDIT.md` mit Repo-URLs, Hashes, Reproduktionsbefehlen, Testergebnissen, Trennstellen, Risiken und Entscheidung „als Abhängigkeit / gezielter Fork / nicht übernehmen“.

## M1 — Virtuelle Funkstrecke und IP-Basis

**Stand 31.10.2026:** grundlegende Zwei-Knoten-UDP-/HTTP-Demo inklusive
TUN-Isolation, beidseitigem PID-0xCC-Trace und Kanalunterbrechung/Recovery
bestanden. Ausführung: `python3 scripts/test-two-nodes.py`.
Noch offen: deterministische Störungsmatrix, Fragmentierungsgrenzen und
quantitative Verlust-/Kollisionsabnahme. Details: `TWO_NODE_DEMO.md`.

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
- NET/ROM-Routing auf Wiederverwendbarkeit prüfen; sonst schmalen eigenen Routingdienst in AXLink spezifizieren.
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

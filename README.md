# AXLink — Projektübergabe

**Stand:** 31. Oktober 2026  
**Status:** Experimentelle Zwei-Knoten-IP-Demo: UDP/HTTP über TUN → RHPv2 → AX.25/KISS → Audio-Simulator nachgewiesen  
**Projektname:** AXLink (vom Nutzer am 31. Oktober 2026 festgelegt)

> **Neu (2026-11-02):** [Review](docs/REVIEW_2026-11-02.md) und [Entscheidungen](docs/DECISIONS.md).
> Der Kern wird in Go neu aufgebaut (Lizenz GPL-3.0-or-later). Erster Stand: KISS, AX.25-UI,
> Sendedauer, Taktung je TNC und ein Modell der KISS-App der F4HWN-Firmware, mit Tests und CI
> auf Linux, Windows und macOS. Start: `bash scripts/setup-go.sh`, dann `go test ./...`;
> Details in [DEVELOPMENT.md](docs/DEVELOPMENT.md). Die unten beschriebene Simulator-Demo mit
> pdn/pdn-net bleibt als Referenz erhalten (Tag `v0.1.0-alpha.1`).

## Aktueller Arbeitsstand

Die Übergabe ist in einzelne Projektdateien entpackt; unveränderte Originale
liegen unter `archive/`. Das Projekt wurde nach `/home/philipp/AXLink`
verschoben und umbenannt. Git ist initialisiert. Mit den geänderten
Sitzungsfreigaben sind Netzwerk und Git zugänglich; SDK 10.0.107 liegt
unter `.tools/dotnet`. Die Python-Runner finden dieses SDK automatisch.

- [M0-Audit](docs/UPSTREAM_AUDIT.md): fixierte Upstreams, Schnittstellen,
  Empfangsfilter-Befund und ausgeführte Prüfungen.
- [IP-Demo](docs/TWO_NODE_DEMO.md): ausgeführte UDP-/HTTP-Demo mit Trace,
  Kanalunterbrechung und Wiederherstellung.
- [Simulatorplan](docs/SIMULATOR_PLAN.md): weitere Störungs-/Mehrhop-Prüfungen.
- `upstream.lock.json`: genaue Quellstände und Herkunft der Audit-Kopien.
- `python3 scripts/audit-upstream.py`: Quellenintegrität und Voraussetzungen.
- `python3 scripts/audit-upstream.py --run`: gepinnte Checkouts bauen/testen,
  Hardware-/Interop-Tests ausgeschlossen, APRS-Testdaten gepinnt nachgeladen.
- `python3 scripts/test-destination-filter.py`: F1-Regression an der Baseline
  (erwartet rot); mit `--patched` läuft die gesamte pdn-net-Suite am lokalen Patch.
- `python3 scripts/test-two-nodes.py`: isolierte Zwei-Knoten-IP-Demo samt Cleanup.
- [Simulator-Anleitung](simulator/README.md): zusätzlicher KISS-UI-Smoke-Test.

29 lokale Quellenkopien sind per SHA-256 geprüft. Der F1-Regressionstest
reproduziert die fremdadressierte Zustellung. Mit Patch bestehen 49 pdn-net-Tests.
Der unveränderte Upstream besteht 13.295 Packet.NET-Tests (48 Skips) und
40 pdn-net-Tests. UDP und HTTP funktionieren im isolierten Zwei-Knoten-Labor
in beiden Richtungen, scheitern bei pausiertem Kanal und funktionieren danach
wieder. Die vollständigen IP-Pakete wurden auf beiden Kanalseiten als AX.25-UI
mit PID `0xCC` nachgewiesen. Reale Funkinteroperabilität und Mesh bleiben offen.

## Demo starten

Voraussetzungen: Linux, Docker mit Compose/Buildx und Zugriff auf den Docker-
Daemon, Python >= 3.12, Git, curl und `/dev/net/tun`.

```sh
bash scripts/setup-host.sh             # SDK, gepinnte Upstreams, Builds und Tests
python3 scripts/test-two-nodes.py      # baut das Labor, prüft UDP/HTTP, räumt auf
```

Details und Nachweise: [Zwei-Knoten-Demo](docs/TWO_NODE_DEMO.md).
Dies ist ein Simulator-Prerelease, kein Funkbetriebsprogramm oder fertiges Mesh.
Lizenzstand: [LICENSING.md](LICENSING.md).

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

> Die ursprüngliche Planungsübergabe liegt unter `archive/`. Inzwischen sind Audit und Simulator-Testgerüste ausführbar. Hardware, Windows, Discovery, Chat und Mesh sind weiterhin nicht abgenommen.

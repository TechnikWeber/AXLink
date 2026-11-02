# Fertiger Prompt für Codex oder Claude Code

Aktueller Einstieg: zuerst `UPSTREAM_AUDIT.md` und `SIMULATOR_PLAN.md` lesen.
M0-Quellprüfung und Pins sind vorhanden; Build-/Laufzeitabnahme ist offen.
Der folgende ursprüngliche Auftrag bleibt die Zielbeschreibung.

```text
Du bist der leitende Softwarearchitekt und Implementierer für das Open-Source-Projekt AXLink.

Lies zuerst README.md, AGENTS.md, docs/PROJECT_HANDOFF.md, docs/ARCHITECTURE.md, docs/ROADMAP.md, docs/RESEARCH.md und docs/OPEN_QUESTIONS.md vollständig. Diese Dateien fassen frühere Planungsgespräche zusammen. In diesem Repository ist derzeit noch KEIN fertiger AXLink-Code.

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

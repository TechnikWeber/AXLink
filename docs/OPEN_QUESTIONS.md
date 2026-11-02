# AXLink — Offene Architekturentscheidungen

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

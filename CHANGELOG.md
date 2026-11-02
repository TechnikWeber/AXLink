# Änderungen

## 0.1.0-alpha.1 — 2026-10-31

Erster ausführbarer Simulatorstand. Normale UDP-/HTTP-Anwendungen erreichen
eine zweite private IP-Adresse über zwei echte TUN/pdn-net/pdn-Prozessketten
und den virtuellen AFSK1200-Kanal. Bei Kanalunterbrechung scheitern die Requests;
nach Wiederaufnahme funktionieren sie erneut. AX.25-UI/PID-0xCC-Traces belegen
den Transport der unveränderten IP-Pakete auf beiden Seiten.

- Gepinnte Upstreams und SDK, reproduzierbarer Audit-Runner ohne Hardwaretests.
- Packet.NET: 13.295 Tests bestanden, 48 Skips; pdn-net: 40 bestanden.
- F1-Empfangsfehler reproduziert; lokaler Patch besteht alle 49 erweiterten Tests.
- Isoliertes Docker-Labor mit Cleanup, Rohframes, Routennachweisen und Negativkontrollen.
- Elf Python-Tests für Audit-Guardrails, begrenzte UDP-Wiederholungen und korrekte Bewertung der Trace-Nachweise.

Experimentell: keine Hardwarefreigabe, kein Windows/Wintun, kein Chat oder Mesh.
Fragmentierung und kontrollierte Störungsmatrizen bleiben offen. Eigenlizenz
noch nicht festgelegt; zunächst privates Prerelease, Fremdlizenzen unverändert.

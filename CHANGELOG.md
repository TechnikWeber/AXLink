# Änderungen

## Unreleased (0.2.0-dev)

Neuausrichtung nach dem Review vom 2026-11-02 (`docs/REVIEW_2026-11-02.md`,
`docs/DECISIONS.md`): Kern in Go, Lizenz GPL-3.0-or-later (Protokolltexte CC BY 4.0).

- `internal/kiss`: KISS-Framing mit Escapes, Parametern und G8BPQ-ACKMODE; Fuzz-Test.
- `internal/ax25`: AX.25-v2.2-Adressen und Frames ohne FCS, FCS (CRC-16/X.25). Gegenprobe mit
  123 Frames, die Packet.NET im v0.1-Labor erzeugt hat: gleiche Felder, byte-gleich neu codiert.
- `internal/airtime`: Bit-Stuffing und Sendedauer.
- `internal/sim/f4hwnkiss`: Verhaltensmodell der KISS-TNC-App der F4HWN-Firmware (256-Byte-Ring,
  16 Byte je 10 ms, ein Sendeplatz, gesperrte Interrupts beim Senden), optional mit ACKMODE.
- `internal/pacing`: Taktung je TNC-Profil. Im Modell: ein volles Paket ohne Taktung geht
  verloren; mit dem Profil `f4hwn-kisstnc` kommen alle an (~104 B/s); bei belegtem Kanal hilft
  nur ACKMODE.
- `cmd/axlinkd`: `version` und `decode` (KISS-Strom → JSON).
- CI auf Linux, Windows und macOS; Builds auch für ARMv6 (Pi Zero W).
- `scripts/setup-go.sh` installiert Go projektlokal mit Prüfsumme.

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

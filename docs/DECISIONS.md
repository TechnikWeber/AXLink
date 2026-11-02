# AXLink — Entscheidungen

Dieses Dokument geht allen älteren Dokumenten vor (README, AGENTS, CLAUDE, START_PROMPT,
PROJECT_HANDOFF, ARCHITECTURE, ROADMAP, RESEARCH, OPEN_QUESTIONS, UPSTREAM_AUDIT,
REVIEW_2026-11-02). Bei Widerspruch gilt der neueste Eintrag hier.

## 2026-11-02

**D1 – Tech-Stack: Go.** Der Kern wird neu als ein Go-Programm (`axlinkd`) gebaut:
wireguard-go/tun (Linux, Windows, macOS), gVisor netstack (Userspace-TCP/IP für Gateway und
Betrieb ohne Admin-Rechte), go.bug.st/serial. KISS, AX.25-UI-Frames und IP-über-AX.25
(PID `0xCC`) implementieren wir selbst. Packet.NET, pdn und pdn-net bleiben Gegenstellen im
Interop-Test, keine Laufzeitabhängigkeit. Begründung: `REVIEW_2026-11-02.md`, Abschnitt 6.
Der Stand mit pdn/pdn-net ist als Tag `v0.1.0-alpha.1` erhalten.

**D2 – Keine eingebauten Rechtsbeschränkungen.** AXLink ist weltweit einsetzbar. Jeder
Betreiber prüft selbst, was auf seiner Frequenz erlaubt ist. Die Software leitet alles durch,
auch HTTPS, TLS und VPN, und sperrt keine Dienste aus rechtlichen Gründen.
- Einsatzziel ist auch der Katastrophenfall. Hauptziel ist zunächst Verkehr von Funkamateur
  zu Funkamateur, aber das ist keine Beschränkung.
- Technische Kanalschutz-Filter bleiben, weil sie den 1200-Bd-Kanal nutzbar halten:
  Broadcast-/Multicast-Störverkehr (z. B. mDNS, LLMNR, NetBIOS, SSDP), Airtime-Fairness,
  Queue-Grenzen. Sie sind konfigurierbar und abschaltbar und sind keine Rechtsregeln.
- Rechtliche Hinweise (z. B. für Deutschland § 5 AFuG, § 16 AFuV) stehen nur in der Doku.

**D3 – Rufzeichen nur als Platzhalter.** Code, Tests und Beispielkonfigurationen enthalten
nie ein echtes Rufzeichen. Platzhalter: `N0CALL`; im Labor `TESTA-1`, `TESTB-1`, `TESTC-1`.
Ohne eingetragenes eigenes Rufzeichen sendet AXLink nicht (AX.25 braucht eine Absenderadresse).
Für automatische Stationen trägt jeder Betreiber sein eigenes Rufzeichen ein.

**D4 – Firmware.** Erweiterungen der KISS-TNC-App (ACKMODE und weitere) baut der
Projektinhaber selbst im Fork `TechnikWeber/uv-k1-k5v3-aprs-firmware`. Upstream (F4HWN/F4WAT)
wird per Feature-Request informiert (`upstream/f4hwn-kisstnc-issue.md`). AXLink unterstützt
weiterhin jeden Standard-KISS-TNC; das F4HWN-Profil funktioniert auch mit der unveränderten App,
weil AXLink auf Host-Seite taktet.

**D5 – Lizenz: offen.** Empfehlung: GPL-3.0-or-later für den Code, CC BY 4.0 für die
Protokollspezifikation. Bis zur Entscheidung bleibt das Repo privat, ohne Lizenzfreigabe.

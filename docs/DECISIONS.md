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
Der Stand mit pdn/pdn-net ist als Tag `v0.1.0-alpha.1` erhalten. Entwicklung: `DEVELOPMENT.md`.

**D2 – Keine eingebauten Rechtsbeschränkungen, keine Länderprofile.** AXLink ist weltweit
einsetzbar und so offen wie möglich. Jeder Betreiber prüft selbst, was auf seiner Frequenz
erlaubt ist. Die Software leitet alles durch, auch HTTPS, TLS und VPN, und sperrt keine Dienste
aus rechtlichen Gründen. Es gibt keine Regeln je Land oder Band.
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

**D4 – Firmware-Erweiterungen im eigenen Fork.** Erweiterungen der KISS-TNC-App (ACKMODE und
weitere) baut der Projektinhaber selbst im Fork `TechnikWeber/uv-k1-k5v3-aprs-firmware`.
Upstream (F4HWN/F4WAT) wird per Feature-Request informiert (`upstream/f4hwn-kisstnc-issue.md`).
AXLink unterstützt weiterhin jeden Standard-KISS-TNC; das F4HWN-Profil funktioniert auch mit
der unveränderten App, weil AXLink auf Host-Seite taktet.

**D5 – Lizenz.** Code: GPL-3.0-or-later (`LICENSE`). Protokollspezifikationen:
CC BY 4.0 (`licenses/CC-BY-4.0.txt`). Details: `LICENSING.md`.

**D6 – Firmware-Fahrplan: erst getrennt, mit v7 vereint.**
- Jetzt (F4HWN v6.3): zwei Firmwares per Multiboot. Messenger als Main, F4HWN Labs v6.3
  (mit der KISS-TNC-App) in einem Benutzer-Slot; Wechsel beim Einschalten mit gedrückter
  `MENU`-Taste. Die Flash-Bereiche überschneiden sich laut Quelltext nicht
  (Multiboot-Slots 0x020000–0x0BFFFF, App-Slots 0x102000–0x161FFF, Messenger
  0x1BA000–0x1C9FFF). Auf Hardware noch nicht geprüft.
- Eigene Variante der KISS-App („KISS TNC+“: ACKMODE, Zähler für verworfene Frames, Kennung,
  RSSI) als Overlay-App im selben Firmware-Fork, eigener App-Ordner und eigene Release-Tags.
  Sie kostet keinen MCU-Flash und läuft auf Labs v6.3 und später auf v7.
- Mit F4HWN v7: Messenger auf v7 heben; die App-Plattform ist dann in der Firmware, die eigene
  KISS-App läuft in derselben Firmware wie der Messenger.
- Natives KISS im Messenger nur als Notlösung, falls v7 ausbleibt oder die App-Plattform auf v7
  nicht neben den Messenger passt.

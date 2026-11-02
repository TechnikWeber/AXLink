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
- Stand 2026-11-02: Der Request steht als Issue #763, das Armel in die Discussion
  [#764](https://github.com/armel/uv-k1-k5v3-firmware-custom/discussions/764) (Ideas)
  umgewandelt hat. Das macht er mit Wünschen so, abgelehnt ist er nicht.
- Zweck des Requests: Die offizielle App soll die Erweiterungen bekommen, damit alle
  F4HWN-Nutzer sie haben. Der USB-Rückstau sitzt in der residenten Firmware und lässt sich
  nur dort beheben, nicht in einer App. Außerdem sollen die Befehlsformate abgestimmt sein.
- Die eigene App darf sofort gebaut werden (gegen das `app_api.h` von v6.3). In #764 verweisen
  wir erst darauf, wenn sie gebaut und mindestens im Host-Test und im AXLink-Labor geprüft ist.
  AXLink ist vorbereitet: Profil `f4hwn-kisstnc-ackmode` und das Modell mit ACKMODE.

**D4a – Die eigene KISS-App ist eigenständig und wird getrennt angeboten.** (2026-11-02)
- Sie braucht den Messenger nicht. Der Loader prüft nur ABI-Major, API-Level, Fähigkeiten und
  Ladeadresse. Die sind in F4HWN v6.3.0, im Messenger-Fork und im v7-Branch gleich (ABI 1,
  Adresse fest 0x20000280; v6.3 API 3, v7 API 4; die Tabelle wächst nur). Eine App mit
  `APP_API_MIN` 3 läuft also auf dem originalen **Labs v6.3.0** und auf **Fusion v7**, das dort
  die App-Plattform trägt. Ändert ein v7-Release das ABI, wird neu gebaut.
- Nicht auf Fusion v6.3 und nicht auf dem Messenger v6.3 (kein Loader).
- Mit Originalfirmware fehlt nur der USB-Rückstau (residenter Teil); AXLink taktet trotzdem.
- Zielgruppe ist größer als die des Messengers: Wer nur IP über AX.25 will, nimmt Original-Labs
  (v6.3) bzw. Original-Fusion (v7) plus die App, ohne Multiboot.
- Verteilung: GitHub-Releases mit `.app`-Datei (UV Studio → Apps → Community → Install a .app)
  und später ein Eintrag im Community-Tab (Apps → Community → Propose an app; geht nur mit der
  App laufend auf einem Funkgerät).
- Name: Der Community-Tab vergleicht Namen nur nach Kleinbuchstaben und Ziffern und lässt keine
  offiziellen Namen zu. „KISS TNC+“ ergäbe „kisstnc“ wie das Original und geht nicht.
  Arbeitstitel: **AXLink TNC**.
- Ablageort (entschieden 2026-11-02): eigenes Repo
  [TechnikWeber/AXLink-TNC](https://github.com/TechnikWeber/AXLink-TNC), öffentlich,
  Apache-2.0 wie das Original. F4HWN ist als Submodul auf dem Release-Tag eingebunden (jetzt
  v6.3.0), gebaut wird gegen die Originalfirmware. `app/` behält die Dateinamen von
  `App/apps/kisstnc`; der erste Commit ist die unveränderte App, jede Änderung ein eigener
  Commit, damit F4WAT sie ohne Pull Request übernehmen kann. Name „AXLink TNC“, Titel auf dem
  Display „AXL TNC“. Offene Punkte: `TODO.md` dort und hier.

**D5 – Lizenz.** Code: GPL-3.0-or-later (`LICENSE`). Protokollspezifikationen:
CC BY 4.0 (`licenses/CC-BY-4.0.txt`). Details: `LICENSING.md`.

**D6 – Firmware-Fahrplan: erst getrennt, mit v7 vereint.**
Anleitung für die Übergangszeit: [`MULTIBOOT_V6.md`](MULTIBOOT_V6.md). Alles zu Multiboot ist
eine Übergangslösung und wird nach der Migration auf v7 gelöscht (Checkliste dort).
- Jetzt (F4HWN v6.3): zwei Firmwares per Multiboot. Messenger als Main, F4HWN Labs v6.3
  (mit der KISS-TNC-App) in einem Benutzer-Slot; Wechsel beim Einschalten mit gedrückter
  `MENU`-Taste. Die Flash-Bereiche überschneiden sich laut Quelltext nicht
  (Multiboot-Slots 0x020000–0x0BFFFF, App-Slots 0x102000–0x161FFF, Messenger
  0x1BA000–0x1C9FFF). Auf Hardware noch nicht geprüft.
- Eigene KISS-App (Arbeitstitel „AXLink TNC“: ACKMODE, Zähler für verworfene Frames, Kennung,
  RSSI) als Overlay-App, siehe D4a. Sie kostet keinen MCU-Flash und läuft auf Labs v6.3 und
  auf v7. Wer nur den TNC will, braucht weder Messenger noch Multiboot.
- Mit F4HWN v7: Messenger auf v7 heben; die App-Plattform ist dann in der Firmware, die eigene
  KISS-App läuft in derselben Firmware wie der Messenger.
- Natives KISS im Messenger nur als Notlösung, falls v7 ausbleibt oder die App-Plattform auf v7
  nicht neben den Messenger passt.

# Messenger und KISS TNC per Multiboot (Übergang bis F4HWN v7)

> **Übergangslösung.** Diese Anleitung gilt nur, solange der Messenger auf F4HWN v6.3 beruht.
> Sobald der Messenger auf v7 umgezogen ist und die App-Plattform in derselben Firmware
> enthält, wird sie gelöscht. Siehe [Aufräumen nach v7](#aufräumen-nach-v7).

## Wofür

Der Messenger (APRS im Hintergrund) und die KISS-TNC-App (Funkgerät als Modem für AXLink,
APRSdroid und andere KISS-Programme) passen in v6.3 nicht in eine Firmware. Mit Multiboot
liegen beide auf dem Gerät; beim Einschalten wählst du, welche läuft.

| Multiboot-Platz | Firmware | Wofür |
|---|---|---|
| `M` (Main) | Messenger, `f4hwn.messenger.bin` | Alltag: Sprechfunk und APRS-Messenger |
| `1` | F4HWN Labs v6.3.0, `f4hwn.labs.v6.3.0.bin` | KISS-TNC-App für AXLink, APRSdroid |

Die Schritte folgen dem F4HWN-Wiki (Seiten „Multiboot and Multiconfig“, „UV Studio“,
„Overlay apps“, „App KISS TNC“), Stand 2026-11-02.

**Nur TNC, ohne Messenger:** Dafür braucht es kein Multiboot. Das originale F4HWN Labs v6.3.0
normal flashen (UV Studio → *Flash Firmware*), dann wie in Schritt 4 unten die KISS-TNC-App
installieren, später auch unsere eigene
([AXLink TNC](https://github.com/TechnikWeber/AXLink-TNC), als `.app` über UV Studio →
*Apps* → *Community* → *Install a .app*). Ab v7 geht dasselbe mit dem originalen Fusion.

## Was du brauchst

- UV-K1 oder UV-K5 V3 (PY32F071).
- USB-C-Datenkabel; für Sendebetrieb einen Ferritkern (VHF-Material) am Funkgerät-Ende.
- Einen Desktop-Browser mit Web Serial (Chrome, Edge, Brave, Opera, Firefox ab 151) für
  [UV Studio](https://armel.github.io/uvstudio/).

## Einmal einrichten

1. **Kalibrierung sichern.** UV Studio → *Dump Calibration*; die Datei nach der
   Seriennummer des Geräts benennen.
2. **Messenger als Main flashen.** Gerät in den Flash-Modus (DFU) bringen, UV Studio →
   *Flash Firmware* → lokale Datei `f4hwn.messenger.bin` (aus den Releases des Forks) →
   *Flash firmware*. Beim ersten Start zeigt das Gerät `Init Main`: nicht ausschalten.
3. **Labs in Platz 1 schreiben.** Gerät normal starten (nicht DFU), UV Studio →
   *Firmware Slots* → Tabelle aktualisieren → aus dem Katalog *F4HWN Labs v6.3.0* wählen
   (oder `f4hwn.labs.v6.3.0.bin` als Datei) → Platz `1`, Name z. B. `LABS` → *Write to slot*
   → bestätigen und warten, bis Löschen, Schreiben und CRC-Prüfung fertig sind. Der laufende
   Messenger bleibt dabei unverändert.
4. **Zu Labs wechseln** (siehe unten) und dort die App installieren: UV Studio → *Apps* →
   *Catalog* (UV Studio wählt den passenden v6.3.0-Katalog) → *KISS TNC* → *Install*.
   Später geht hier auch unsere eigene Variante als `.app`-Datei oder über den
   Community-Tab.
5. **In Labs die Frequenz einstellen.** Labs nutzt seine eigene Konfiguration `CFG 1`:
   VFO auf die Datenfrequenz (AXLink) oder 144,800 MHz (APRSdroid), FM, Leistung.

## Wechseln

- **Zu Labs:** Gerät aus, `MENU` gedrückt halten, einschalten, beim Bild
  `F4HWN MULTIBOOT` loslassen. Platz `1` wählen (UV-K1: LINKS/RECHTS, UV-K5: AUF/AB),
  `MENU`, bei `Restore ...?` noch einmal `MENU`. Während `Writing / Verify`
  **nicht ausschalten**; das Gerät startet danach neu.
- **KISS TNC starten:** in Labs `F` + `7`, *KISS TNC* wählen, `MENU`. Dann AXLink am PC
  starten oder APRSdroid verbinden. `EXIT` beendet die App.
- **Zurück zum Messenger:** wie oben, aber `M` wählen. Mailbox und Messenger-Einstellungen
  bleiben erhalten; sie liegen in einem eigenen Flash-Bereich.

## AXLink dazu (Linux)

```sh
./axlinkd ports      # zeigt das Funkgerät mit USB 36B7:FFFF
sudo ./axlinkd run -call <RUFZEICHEN>-1 -port auto -ip 10.73.0.1/24 -route 10.73.0.2=<GEGENSTELLE>-1
```

Für serielle Ports ist das Profil `f4hwn-kisstnc` Standard. Details: `DEVELOPMENT.md`.

## Gut zu wissen

- Jeder Wechsel schreibt die Firmware neu ins Gerät; das dauert einige Sekunden.
- Jeder Platz hat seine eigene Konfiguration (Kanäle, VFOs): Main `CFG M`, Labs `CFG 1`.
  Mit `SetCfg` lässt sich eine Bank teilen; laut F4HWN auf eigene Verantwortung, vorher sichern.
- Apps liegen im externen Flash und bleiben beim Wechsel erhalten; starten kann sie nur Labs.
- Die App-Version muss zur Firmware passen: Labs v6.3.0 → Apps aus dem v6.3.0-Katalog.
- Die Flash-Bereiche überschneiden sich laut Quelltext nicht: Multiboot-Plätze
  0x020000–0x0BFFFF, App-Plätze von Labs v6.3 0x102000–0x161FFF, Messenger
  0x1BA000–0x1C9FFF.
- Wer in UV Studio den kompletten externen Flash zurückspielt, überschreibt auch die
  Messenger-Daten.
- **Auf Hardware noch nicht geprüft.** Das ist der erste Test, wenn die UV-K1 da sind.

## Aufräumen nach v7

Sobald der Messenger auf F4HWN v7 läuft und die App-Plattform enthält:

- [ ] diese Datei löschen;
- [ ] in `DECISIONS.md` den Multiboot-Teil von D6 streichen und D6 als erledigt markieren;
- [ ] die Verweise in `README.md` und `DEVELOPMENT.md` entfernen;
- [ ] im Firmware-Fork den Multiboot-Hinweis im Messenger-README durch „KISS TNC in
      derselben Firmware“ ersetzen;
- [ ] die eigene KISS-App (AXLink TNC) auf v7 prüfen und neu bauen, falls v7 das ABI ändert.

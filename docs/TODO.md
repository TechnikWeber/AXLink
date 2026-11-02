# AXLink — offene Punkte

Hintergrund und Begründungen: [DECISIONS.md](DECISIONS.md). Stand: 2026-11-02.

## AXLink (PC)

- [ ] Hardwaretest mit zwei UV-K1, sobald sie da sind: Multiboot einrichten
      ([MULTIBOOT_V6.md](MULTIBOOT_V6.md)), `axlinkd run` mit der Original-KISS-App,
      Grenzen messen (Framegröße, USB-Ring, Taktung, USB-Reset), Durchsatztabelle.
- [ ] Netzadapter für Windows (Wintun) und macOS (utun).
- [ ] Start ohne `sudo` per `setcap cap_net_admin+ep` prüfen.
- [ ] Stationserkennung mit Markierungen (`RELAY`, `INET`, `LINK`, `AUTO`).
- [ ] Chat im APRS-Nachrichtenformat, damit Messenger-Geräte mitreden.
- [ ] Sende-Warteschlange: Prioritäten (kleine Pakete zuerst) und doppelte TCP-Wiederholungen
      verwerfen.
- [ ] Link-Gateway (AXUDP) und Internet-Gateway (Userspace, netstack).
- [ ] Mesh: Weiterleitung A↔B↔C, Routing mit Airtime-Budget.
- [ ] Segmentierung (PID 0x08) nach AX.25 2.2 und ARP (PID 0xCD), mit Tests gegen Packet.NET.

## AXLink TNC (eigene KISS-App)

Repo: [TechnikWeber/AXLink-TNC](https://github.com/TechnikWeber/AXLink-TNC); die Liste dort
([TODO.md](https://github.com/TechnikWeber/AXLink-TNC/blob/main/TODO.md)) ist maßgeblich.

- [ ] Erweiterungen bauen: ACKMODE, Zähler für verworfene Frames, abgeschnittenen Frame nach
      USB-Neuanmeldung verwerfen, Kennung, RSSI.
- [ ] Im AXLink-Labor gegen das Modell mit ACKMODE und danach auf der Bank prüfen.
- [ ] **Eintrag im UV-Studio-Community-Tab** (nach dem Hardwaretest): Apps → Community →
      Propose an app, mit der App laufend auf einem Funkgerät.
- [ ] In [Discussion #764](https://github.com/armel/uv-k1-k5v3-firmware-custom/discussions/764)
      antworten, wenn die Erweiterungen gebaut und getestet sind.

## Nach F4HWN v7

- [ ] Messenger auf v7 heben; Multiboot-Doku löschen (Checkliste in [MULTIBOOT_V6.md](MULTIBOOT_V6.md)).
- [ ] AXLink TNC gegen v7 bauen und auf Fusion v7 prüfen.

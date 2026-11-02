# AXLink — open points

Background and reasons: [DECISIONS.md](DECISIONS.md). As of 2026-11-02.

## AXLink (PC)

- [ ] Hardware test with two UV-K1 once they arrive: set up Multiboot
      ([MULTIBOOT_V6.md](MULTIBOOT_V6.md)), run `axlinkd run` with the original KISS app,
      measure the limits (frame size, USB ring, pacing, USB reset), a throughput table.
- [ ] Windows: test the adapter on a real PC with a radio (so far only CI pings through it),
      the firewall rules for incoming connections, a release package with `axlinkd.exe`,
      `wintun.dll` and its licence (allowed by D8), running as a service.
- [ ] Network adapter for macOS (utun).
- [ ] ICMP "fragmentation needed" for packets with DF that are larger than the radio MTU
      (path-MTU discovery); today they are dropped.
- [ ] Check running without `sudo` via `setcap cap_net_admin+ep`.
- [ ] Station discovery with tags (`RELAY`, `INET`, `LINK`, `AUTO`).
- [ ] Chat in the APRS message format, so that messenger radios can take part.
- [ ] Transmit queue: priorities (small packets first) and dropping duplicate TCP retransmissions
      (in the lab TCP repeats its SYN after 1 s because the answer takes ~1.7 s).
- [ ] Pacing for the original F4HWN app: hold the next frame while the radio hears other
      stations. In the lab (a 2 KB download: data one way, ACKs the other) the estimated end of
      a transmission passes while the channel is busy, and the app drops 7 frames; with ACKMODE
      none (44 s instead of 111 s).
- [ ] Link gateway (AXUDP) and internet gateway (userspace, netstack).
- [ ] Mesh: forwarding A↔B↔C, routing with an airtime budget.
- [ ] Segmentation (PID 0x08) per AX.25 2.2 and ARP (PID 0xCD), tested against Packet.NET.
- [ ] Web UI in English and German (D7).
- [ ] Report the v0.1 findings F1 and F4 to pdn-net ([HISTORY.md](HISTORY.md)).

## AXLink TNC (our KISS app)

Repository: [TechnikWeber/AXLink-TNC](https://github.com/TechnikWeber/AXLink-TNC); its list
([TODO.md](https://github.com/TechnikWeber/AXLink-TNC/blob/main/TODO.md)) is the reference.

- [ ] Build the extensions: ACKMODE, a counter for dropped frames, dropping a frame cut off by a
      USB re-enumeration, identification, RSSI.
- [ ] Test them in the AXLink lab against the model with ACKMODE, then on the bench.
- [ ] **Entry in the UV Studio Community tab** (after the hardware test): Apps → Community →
      Propose an app, with the app running on a radio.
- [ ] Reply in [Discussion #764](https://github.com/armel/uv-k1-k5v3-firmware-custom/discussions/764)
      once the extensions are built and tested.

## After F4HWN v7

- [ ] Move the messenger to v7; delete the Multiboot documentation (checklist in
      [MULTIBOOT_V6.md](MULTIBOOT_V6.md)).
- [ ] Build AXLink TNC against v7 and check it on Fusion v7.

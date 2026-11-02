# AXLink — open points

Background and reasons: [DECISIONS.md](DECISIONS.md). As of 2026-11-02.

## AXLink (PC)

- [ ] Hardware test with two UV-K1 once they arrive: set up Multiboot
      ([MULTIBOOT_V6.md](MULTIBOOT_V6.md)), run `axlinkd run` with the original KISS app,
      measure the limits (frame size, USB ring, pacing, USB reset), a throughput table; check
      the transmit queue's limits (128 bytes urgent, repeat window) and whether the app passes
      received frames on quickly enough for the longer wait after heard frames.
- [ ] Windows: test the adapter on a real PC with a radio (so far only CI pings through it),
      the firewall rules for incoming connections, a release package with `axlinkd.exe`,
      `wintun.dll` and its licence (allowed by D8), running as a service.
- [ ] Network adapter for macOS (utun).
- [ ] ICMP "fragmentation needed" for packets with DF that are larger than the radio MTU
      (path-MTU discovery); today they are dropped.
- [ ] Check running without `sudo` via `setcap cap_net_admin+ep`.
- [ ] Gateway, next steps: its rules in the beacon (services, rate, contact); a fair share per
      callsign; pages shrunk for 1200 bit/s; a transparent gateway in userspace (netstack) for
      other protocols.
- [ ] Web interface, next steps: the chat; a page that explains the proxy setting per browser;
      beacon interval and tags in the settings.
- [ ] Forwarding to the home network on purpose: publish a service of a LAN host (e.g. a web
      server on 192.168.1.10:80) on a port of the own radio address, with the same callsign
      rules as the firewall (port forwarding in userspace).
- [ ] Choosing among several gateways by link quality and load (needs RSSI from the TNC and
      load in the beacon); give up a gateway that stops answering before its beacon expires.
- [ ] Channel access without ACKMODE with three or more stations: two stations waiting for the
      same busy channel both send in the first clear slot (PERSIST 255); a random host backoff
      after a heard frame.
- [ ] Chat in the APRS message format, so that messenger radios can take part.
- [ ] Link gateway (AXUDP) and internet gateway (userspace, netstack).
- [ ] Mesh: forwarding A↔B↔C, routing with an airtime budget.
- [ ] Segmentation (PID 0x08) per AX.25 2.2 and ARP (PID 0xCD), tested against Packet.NET.
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
      once the extensions are built and tested; what to tell them is collected in
      [upstream/README.md](upstream/README.md).

## After F4HWN v7

- [ ] Move the messenger to v7; delete the Multiboot documentation (checklist in
      [MULTIBOOT_V6.md](MULTIBOOT_V6.md)).
- [ ] Build AXLink TNC against v7 and check it on Fusion v7.

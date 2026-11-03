# AXLink — open points

Background and reasons: [DECISIONS.md](DECISIONS.md). As of 2026-11-03.

## AXLink (PC)

- [ ] Hardware test with two UV-K1 once they arrive: set up Multiboot
      ([MULTIBOOT_V6.md](MULTIBOOT_V6.md)), run `axlinkd run` with the original KISS app,
      measure the limits (frame size, USB ring, pacing, USB reset), a throughput table; check
      the transmit queue's limits (128 bytes urgent, repeat window) and whether the app passes
      received frames on quickly enough for the longer wait after heard frames.
- [ ] Windows: test the adapter on a real PC with a radio (so far only CI pings through it),
      the firewall rules for incoming connections, running as a service. (The release package
      with `wintun.dll` and its licence exists since 0.2.0-alpha.1.)
- [ ] macOS: test with a radio (the utun adapter passes its ping test in CI); a signed package.
- [ ] ICMP "fragmentation needed" for packets with DF that are larger than the radio MTU
      (path-MTU discovery); today they are dropped.
- [ ] Check running without `sudo` via `setcap cap_net_admin+ep`.
- [ ] Gateway, next steps: its rules in the beacon (services, rate, contact); a fair share per
      callsign; pages shrunk for 1200 bit/s; a transparent gateway in userspace (netstack) for
      other protocols.
- [ ] Web interface, next steps: a page that explains the proxy setting per browser; beacon
      interval and tags in the settings; a notification for new chat messages.
- [ ] Port forwarding for UDP (TCP is done).
- [ ] Chat: a mailbox on an unattended station (tag `AUTO`) that keeps messages for stations
      that are away and delivers them when they are heard, so that sender and recipient need
      not be on the air at the same time; groups (`#NAME` as the messenger's channels); APRS
      bulletins.
- [ ] Choosing among several gateways by link quality and load (needs RSSI from the TNC and
      load in the beacon); give up a gateway that stops answering before its beacon expires.
- [ ] Channel access without ACKMODE with three or more stations: two stations waiting for the
      same busy channel both send in the first clear slot (PERSIST 255); a random host backoff
      after a heard frame.
- [ ] Chat on the air with the APRS messenger firmware (bench test: a message each way, CQ).
- [ ] Link gateway (AXUDP) and internet gateway (userspace, netstack).
- [ ] Mesh, next steps: fewer collisions of hidden stations at a relay (e.g. a short random
      wait after hearing a relay, or the relay announcing the channel as busy); choosing the way
      by link quality (needs RSSI); an airtime budget per relay; static routes over a relay
      (`-route ...` with a path).
- [ ] Segmentation (PID 0x08) per AX.25 2.2 and ARP (PID 0xCD), tested against Packet.NET.
- [ ] Report the v0.1 findings F1 and F4 to pdn-net ([HISTORY.md](HISTORY.md)).

## AXLink TNC (our KISS app)

Repository: [TechnikWeber/AXLink-TNC](https://github.com/TechnikWeber/AXLink-TNC); its list
([TODO.md](https://github.com/TechnikWeber/AXLink-TNC/blob/main/TODO.md)) is the reference.

- [x] ACKMODE, a counter for host frames lost, dropping a frame cut off by a USB reset:
      AXLink TNC 0.2, host-tested, prerelease v0.2.0-alpha.1 (2026-11-03, D4).
- [ ] Identification (KISS SetHardware `0x06`) and RSSI.
- [ ] Bench test on two radios: AXLink with `f4hwn-kisstnc-ackmode` against AXLink TNC, and
      with `f4hwn-kisstnc` against the original app.
- [ ] **Entry in the UV Studio Community tab** (after the hardware test): Apps → Community →
      Propose an app, with the app running on a radio.
- [ ] Reply in [Discussion #764](https://github.com/armel/uv-k1-k5v3-firmware-custom/discussions/764):
      drafted in [upstream/f4hwn-764-followup-2.md](upstream/f4hwn-764-followup-2.md), the owner
      posts it; what else to tell them is collected in [upstream/README.md](upstream/README.md).

## After F4HWN v7

- [ ] Move the messenger to v7; delete the Multiboot documentation (checklist in
      [MULTIBOOT_V6.md](MULTIBOOT_V6.md)).
- [ ] Build AXLink TNC against v7 and check it on Fusion v7.

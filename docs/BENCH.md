# Bench test with two radios

Everything so far ran in the lab and in CI. This plan takes AXLink, AXLink TNC and the chat with
the APRS messenger onto real radios, in an order where each step builds on the one before. Note
the results in the table at the end. The messenger's own plan is in its repository
(`App/messenger/BENCH.md`).

## What you need

- Two Quansheng UV-K1 or UV-K5 V3 with F4HWN Labs v6.3.0, the KISS TNC app and AXLink TNC
  installed; later one of them with the APRS messenger firmware.
- USB-C cables, a clip-on ferrite (VHF material) at the radio end of each.
- Two computers with axlinkd (or one Linux computer, each station in its own network namespace
  as `scripts/lab-netns.sh` does).
- Dummy loads, or low power on a free simplex frequency; the radios a few metres apart.
- Your callsign with two SSIDs, e.g. `<CALL>-1` and `<CALL>-2`, addresses `10.73.0.1/24` and
  `10.73.0.2/24`.
- On every run `-pcap station-a.pcap` (and b): Wireshark shows afterwards what went over the air.

## The tests

| # | Test | How | Expected |
|---|---|---|---|
| 1 | Radio found | `axlinkd ports` on each computer | the radio marked "F4HWN firmware" (USB `36B7:FFFF`) |
| 2 | USB link | KISS TNC app running, axlinkd started; key 2 on the radio | the test frame shows in the view Monitor; the web interface says "TNC connected" |
| 3 | Beacons | both stations started, original app, profile `f4hwn-kisstnc` | each lists the other within seconds; the Monitor shows both beacons |
| 4 | Ping | `ping -c 20 10.73.0.2` from A, then the other way | note loss and round trip (lab: about 2.1 s) |
| 5 | HTTP | open `http://10.73.0.2/` (welcome page), then a 2 KB file from a web server on B | note the times (lab: 9 s for a small file, about 40 s for 2 KB) |
| 6 | Refused | `curl http://10.73.0.2:9999/` from A | "connection refused" within seconds; B's overview lists A as refused |
| 7 | Chat | a message A → B, the answer B → A, a CQ | delivered at the first try (lab: 3-5 s); the CQ in B's room once |
| 8 | ACKMODE | AXLink TNC on both radios, profile `f4hwn-kisstnc-ackmode`; tests 4, 5 and 7 again | the same or faster; `lost` on the radio's screen stays 0 |
| 9 | Fast host | without pacing (`-profile generic-kiss`) a 2 KB download | with the original app: frames lost (its limit); with AXLink TNC: `lost N` counts them |
| 10 | USB reset | power high, antenna near the cable, without the ferrite; 20 pings | `re` on the radio's screen goes up; AXLink reconnects by itself; no truncated frame in Wireshark |
| 11 | Messenger | radio B with the messenger firmware, *Freq* on the AXLink frequency; message A → B's callsign, B answers on the keys, a CQ from each side | the messenger shows A's message and acknowledges it; A's chat shows B's answer as a message; CQ in both rooms |
| 12 | Gateway | B with internet: `-proxy 8080`; A chooses B and opens a small page through `127.0.0.1:7374` | the page arrives; B's overview counts the requests |

Mesh (three stations in a line) needs a third station, e.g. a third radio or Dire Wolf with a
sound card; it follows once the two-station tests pass.

## Results

| # | Result | Numbers / notes |
|---|---|---|
| 1 | | |
| 2 | | |
| 3 | | |
| 4 | | |
| 5 | | |
| 6 | | |
| 7 | | |
| 8 | | |
| 9 | | |
| 10 | | |
| 11 | | |
| 12 | | |

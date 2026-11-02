# pdn-net: receive destination filter (F1)

Base: `91c412c91604c17bc28abd49e66443377292e5dc`. The patch was prepared locally and has not
been submitted upstream. It changes AGPL-3.0-or-later sources; the upstream licence applies
(see `artifacts/m0/sources/pdn-net/LICENSE`). Part of the v0.1 lab ([HISTORY](../docs/HISTORY.md)).

`pdn-net-destination-filter.patch` checks the RHP destination before IPv4 or segments are handed
to the TUN or to reassembly. Case is ignored; a missing SSID and `-0` mean the same station.
Other SSIDs and QST-addressed IP are dropped. Broadcast IP is not used in the first unicast lab
profile. ARP deliberately keeps its existing learning and replies by target IP, also for QST;
hardening ARP is a separate open task.

An existing upstream test helper is set to `N0CALL-10`: its incoming test datagrams use that
address, while the helper so far configured the different default address `N0CALL`.

```sh
python3 scripts/test-destination-filter.py           # expected to fail on the baseline
python3 scripts/test-destination-filter.py --patched # the whole pdn-net test suite
```

Both runs use temporary exports of the clean pinned checkout. The additional tests are in
`tests/upstream/AXLinkDestinationFilterTests.cs`. No AX.25, KISS or IP stack of our own was
added.

Limits: the patch fixes the delivery of frames addressed to other stations to the TUN, not the
general validation of incoming IP data. The segment test checks the existing upstream behaviour,
not its conformance to the standard or its on-air interoperability.

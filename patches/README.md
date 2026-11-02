# pdn-net: Empfangs-Zielfilter (F1)

Basis: `91c412c91604c17bc28abd49e66443377292e5dc`.
Der Patch ist lokal vorbereitet, nicht upstream eingereicht. Er enthält
Änderungen an AGPL-3.0-or-later-Quellen; die Upstream-Lizenz bleibt maßgeblich
(siehe `artifacts/m0/sources/pdn-net/LICENSE`).

`pdn-net-destination-filter.patch` prüft das RHP-Ziel vor Übergabe von IPv4
oder Segmenten an TUN/Reassembly. Groß-/Kleinschreibung wird ignoriert;
fehlende SSID und `-0` bezeichnen dieselbe Station. Andere SSIDs und QST-IP
werden verworfen. Broadcast-IP wird im ersten Unicast-Laborprofil nicht genutzt.
ARP behält ausdrücklich das bisherige Lernen und Antworten nach Ziel-IP bei,
auch für QST. ARP-Härtung ist eine separate offene Aufgabe.

Ein bestehender Upstream-Test-Helper wird auf `N0CALL-10` gesetzt: Seine
eingehenden Testdatagramme verwenden diese Adresse, während der Helper
bisher die abweichende Standardadresse `N0CALL` konfiguriert hat.

```sh
python3 scripts/test-destination-filter.py           # erwartet rot an der Baseline
python3 scripts/test-destination-filter.py --patched # gesamte pdn-net-Testsuite
```

Beide Läufe verwenden temporäre Exporte des sauberen gepinnten Checkouts.
Die zusätzlichen Tests stehen in `tests/upstream/AXLinkDestinationFilterTests.cs`.
Kein eigener AX.25-, KISS- oder IP-Stack wurde ergänzt.

Grenzen: Der Patch behebt die fremdadressierte TUN-Zustellung, nicht die
allgemeine Validierung eingehender IP-Daten. Der Segmenttest prüft das vorhandene
Upstream-Verhalten, nicht dessen Standardkonformität oder On-Air-Interoperabilität.

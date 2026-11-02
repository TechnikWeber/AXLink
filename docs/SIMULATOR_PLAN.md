# AXLink — minimaler Simulatorplan

Stand 2026-10-31. **KISS-Smoke-Test und Zwei-Knoten-IP-Basispfad ausgeführt.**

Aktueller Nachweis: [UDP/HTTP-Demo mit Trace, Unterbrechung und Recovery](TWO_NODE_DEMO.md).
Die folgenden Stufen bleiben der weitergehende Testplan; Verlustmatrix,
Fragmentierungs-/Interop-Abnahme und Mehrhop sind noch offen.

Fortschritt: Der bestehende `NetsimKissTcpInterop`-UI-Test besteht mit der
eigenen Zwei-Knoten-Konfiguration aus `simulator/`. Reproduktion und genaue
Grenzen: [Simulator-Anleitung](../simulator/README.md).
F1 wurde reproduziert (vier von neun ergänzten Prüfungen schlagen an der
Baseline fehl); mit lokalem Patch bestehen alle 49 pdn-net-Tests.
`unshare --user --map-root-user --net sh -c 'ip link show lo'` ist erfolgreich;
TUN ist inzwischen vorhanden. Die echte TUN-Demo ist inzwischen bestanden.

## Stufe 1: vorhandene Komponenten ohne TUN-Rechte prüfen

Die fixierten Upstreams aus `upstream.lock.json` bauen und testen:

```sh
python3 scripts/audit-upstream.py --run
```

Ein erfolgreicher Prozess-Exit allein reicht nicht als Interop-Nachweis:
TRX-Ergebnisse auf übersprungene Tests prüfen. Packet.NETs vorhandener
`tests/Packet.Interop.Tests/Netsim/NetsimKissTcpInterop.cs` prüft einen UI-Frame
über zwei KISS-TCP-Ports (8100/8101), nicht zwei IP-Stacks. Ohne net-sim wird
sein `SkippableFact` übersprungen. Dieses Gerüst erweitern, keinen neuen
AX.25-/KISS-Parser schreiben.

Für den ersten eigenen Integrationstest `IpAx25Bridge` mit den vorhandenen
`FakeTunDevice`/`FakeRhpCustomClient` verbinden. Prüfungen:

Für Punkt 3 liegt inzwischen `tests/upstream/AXLinkDestinationFilterTests.cs`
mit Kontrollen für Fremdziele, SSID, ARP und Segmentierung vor. Ausführen mit
`python3 scripts/test-destination-filter.py` (Python >= 3.12). Der Runner nutzt
eine temporäre Kopie des sauberen gepinnten Checkouts. Der Fehler ist inzwischen
ausgeführt nachgewiesen; `--patched` prüft die komplette pdn-net-Suite mit Patch.

1. Gültiges IPv4/UDP von 10.73.0.1 nach 10.73.0.2 ergibt RHP-Daten
   `[0xCC] + unverändertes IP-Paket`, Zielrufzeichen TESTB-1.
2. Rückrichtung stellt exakt das IP-Paket am Fake-TUN zu.
3. Eingehendes UI an TESTC-1 darf bei TESTB-1 nicht am TUN erscheinen.
   Der aktuelle Quellstand lässt hier einen Fehler erwarten (Audit F1).
4. IPv6, fehlende Route, leere und verkürzte Pakete; danach
   Fragmentierungsgrenzen, DF, ungültige Segmente und Abbruch/Disconnect.
5. Zwei RHP-Clients gleichzeitig: getrennte Handles, keine vermischten Antworten;
   Chat-/IP-PIDs kontrolliert verteilen. Noch kein fertiges Chatprotokoll.

## Stufe 2: echte Prozesse und echte Kernel-IP-Stacks

```text
Namespace A                            Namespace B
UDP/HTTP ↔ TUN ↔ pdn-net                pdn-net ↔ TUN ↔ UDP/HTTP
                  ↕ RHP                RHP ↕
                 pdn                    pdn
                  ↕ KISS-TCP     KISS-TCP ↕
                  └──── virtueller Kanal ─┘
```

Pro Namespace nur Loopback + TUN für die Test-IP. Management-Verbindungen für
KISS bei Bedarf über ein separates Subnetz mit ausschließlich den notwendigen
TCP-Ports; niemals eine Route zu den Peer-Test-IPs über dieses Netz.
RHP bleibt je Namespace auf Loopback:9000. Nur `pdn` besitzt den KISS-Port.
Jeder Prozess erhält eine eigene explizite Simulatorkonfiguration ohne Serial-Port.
Keine vorhandene reale Stationskonfiguration verwenden.

| Parameter | A | B |
|---|---|---|
| Rufzeichen (nur Simulation) | TESTA-1 | TESTB-1 |
| TUN-IP | 10.73.0.1/32 | 10.73.0.2/32 |
| Peerroute | 10.73.0.2/32 über TUN | 10.73.0.1/32 über TUN |
| IP→Callsign | 10.73.0.2 → TESTB-1 | 10.73.0.1 → TESTA-1 |
| initiale MTU | 256 | 256 |

Ein UDP-Echo mit zufälligem Test-Nonce ist der erste Nachweis. Danach ein
HTTP-Server mit einer kleinen festen Textantwort. Beides mit begrenztem Timeout.
Test erfolgreich nur bei passender Nutzlast **und** Trace der zugehörigen
AX.25-UI-Frames mit PID 0xCC auf beiden Kanalseiten. KISS-Frames enthalten kein
FCS. PCAP oder rohe KISS-Traces plus dekodierter Bericht und Prozesslogs ablegen.

Negativkontrolle: Kanal unterbrechen; derselbe Request muss fehlschlagen.
Damit wird ein versehentlicher Host-/Ethernet-Bypass erkannt. Danach Kanal
wiederherstellen und neuen Nonce verwenden. Namespace-Routen und Interface-
Zähler vor/nach dem Test aufzeichnen. Cleanup auch bei Fehler/SIGINT.

## Störungen und spätere drei Stationen

- Fester Zufallsseed, deterministische Verlustfolge, begrenzte Queues.
- Verzögerung und Sendezeit bei 1200 bit/s separat modellieren; HDLC-/KISS-
  Overhead unterscheiden. USB-KISS-Escaping ist keine zusätzliche RF-Nutzlast.
- Halbduplex: kein RX während TX; überlappende Sendungen als Kollision markieren.
- Firmware hat nur einen wartenden TX-Frame: Queue-Verluste gesondert simulieren.
- A↔B↔C mit explizit fehlender A↔C-Kante. IP-TTL, doppelte Frames, B-Ausfall,
  Wiederanlauf und Ablauf von Routen prüfen. Erst das belegt Mehrhop-IP.

## Voraussetzungen und nächste Arbeit

.NET-10-SDK, Checkouts, Netzwerk, TUN-Gerät, User-/Netzwerk-Namespaces und
Docker/net-sim sind inzwischen verfügbar. TUN-Einrichtung, Prozesskonfiguration,
UDP/HTTP, Trace und Unterbrechungs-Negativkontrolle sind implementiert und im
Referenzlauf bestanden. Als Nächstes folgen die beschriebenen Störungsmatrizen
und die offenen F4/F5-Grenzfälle; Hardware und Mehrhop sind nicht freigegeben.

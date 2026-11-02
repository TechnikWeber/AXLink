# KISS-Simulator: erster ausgeführter Baustein

Voraussetzungen: Docker mit Compose, sauberer gepinnter Packet.NET-Checkout
und dessen Release-Build (`python3 scripts/audit-upstream.py --run`).

```sh
docker compose -f simulator/compose.yaml up -d --wait --wait-timeout 90
python3 scripts/test-kiss-simulator.py
docker compose -f simulator/compose.yaml down
```

Cleanup auch bei fehlgeschlagenem Test ausführen. Ports 8100, 8101 und 18080
auf Loopback müssen frei sein. Kein vorhandenes fremdes Containerprojekt stoppen.
Es werden keine USB-Geräte, Soundkarten oder seriellen Ports eingebunden.

Der Runner startet ausschließlich
`NetsimKissTcpInterop.UI_Frame_Sent_On_Node_A_Is_Received_On_Node_B`.
Er verlangt genau einen bestandenen Test; ein Skip zählt als Fehler.
Ergebnis, TRX und Testlog liegen in `artifacts/m0/kiss-simulator`.

Der Test nutzt die vorhandenen Packet.NET-KISS-/AX.25-Parser und den
Audio-Domain-Simulator `packethacking/net-sim` v0.2.0. Das Image ist per Digest
fixiert (aus dem gepinnten Packet.NET-Compose). Image-Metadaten nennen
Quellcommit `872d78a81b068dc6d96035c9cb85e23b21c8945b` und MIT als Lizenz;
ein vollständiger Audit des Images und seiner Abhängigkeiten steht aus.

Die eigene Konfiguration reduziert die Upstream-Topologie auf zwei AFSK1200-
Knoten. Kanalparameter für FM-Capture und Kollision sind gesetzt, aber deren
Fehlerszenarien sind mit diesem einzelnen Smoke-Test noch nicht abgenommen.

**Nachweisgrenze:** Der Upstream-Test sendet einen UI-Frame mit Textpayload.
Das belegt weder PID-0xCC-IP-Transport noch TUN, UDP, HTTP oder Mesh.
Nächste Stufe: zwei isolierte Kernel-IP-Stacks, pdn-/pdn-net-Prozesse,
PID-0xCC-Trace und Kanalunterbrechung als Negativkontrolle.

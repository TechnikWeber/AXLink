# Zwei-Knoten-IP-Demo — experimenteller Simulatorstand

Am 31.10.2026 ausgeführt: normale UDP-/HTTP-Sockets auf zwei getrennten Linux-
IP-Stacks. Jeder Knoten betreibt echtes `pdn-net` mit TUN und echten `pdn` als
alleinigen KISS-Eigentümer. Dazwischen liegt der gepinnte Audio-Domain-
Simulator mit AFSK1200. Es wird keine reale Funkhardware angesprochen.

```text
Container A                                  Container B
UDP / HTTP                                    UDP / HTTP
    ↕                                             ↕
radio0 10.73.0.1/32                        radio0 10.73.0.2/32
    ↕                                             ↕
pdn-net (F1-Patch)                         pdn-net (F1-Patch)
    ↕ RHPv2 loopback                           ↕ RHPv2 loopback
pdn TESTA-1                                   pdn TESTB-1
    ↕ KISS-TCP                                    ↕ KISS-TCP
    └──────────── net-sim AFSK1200 ────────────────┘
```

## Reproduktion

Linux mit Docker Compose/Buildx, Python >= 3.12, Git, curl und `/dev/net/tun`.
Der Benutzer muss Docker ausführen können. Setup installiert .NET 10.0.107
unter `.tools/dotnet`; keine systemweite SDK-Installation oder Shellprofiländerung.

```sh
bash scripts/setup-host.sh
python3 scripts/test-two-nodes.py
```

Der erste Audit-Lauf benötigt wegen langer Upstream-Tests mehrere Minuten.
Der Demo-Runner veröffentlicht den lokalen F1-Patch in einen temporären Build,
baut einen Laborcontainer und startet die drei Dienste. Das Testprogramm
benutzt Standard-Python-Sockets (`socket`, `http.client`), keine privaten
IP-Protokolle. Alle Prozesse/Netze des Laborprojekts werden im `finally`-Pfad
entfernt; ein bereits laufendes Projekt wird nicht übernommen oder gelöscht.

Nach bereits erfolgreicher Vorbereitung ist für lokale Diagnose
`python3 scripts/test-two-nodes.py --no-build` möglich. Dieser Modus verwendet
vorhandene lokale Binärdateien/Image; für eine frische Reproduktion den
Standardaufruf verwenden. Manuelles Cleanup nach einem harten Prozessabbruch:

```sh
docker compose -f simulator/compose.ip.yaml unpause channel
docker compose -f simulator/compose.ip.yaml down
```

## Isolation und Prüfkriterien

- Pro Knoten genau eine Peer-Hostroute über `radio0`, MTU 256. Vorher/nachher
  werden Adressen, Routen und TUN-Zähler aufgezeichnet; `ip route get` muss
  die Peer-IP über TUN führen. Keine Default-Route über Funk.
- Docker-Labornetz `internal: true`, keine veröffentlichten Ports. Es dient
  nur den KISS-Verbindungen. IPv6 und Kernel-Forwarding sind ausgeschaltet.
  Die Container bekommen nur zusätzlich `NET_ADMIN` für ihr eigenes TUN;
  kein privilegierter Container und keine seriellen-/Audio-Geräte.
- UDP-Echo und kleiner HTTP-GET in beiden Richtungen, je mit frischem Nonce
  und Prüfung der vollständigen Antwort. Budget: 30 Sekunden für UDP, 60 Sekunden Socket-Timeout für HTTP (75 Sekunden Prozessgrenze). UDP wiederholt bei fehlender Antwort höchstens alle fünf Sekunden innerhalb des Budgets; Nonce und Nutzlast bleiben gleich, die Versuchszahl wird protokolliert.
- Kanalcontainer pausieren: UDP und HTTP müssen innerhalb von fünf Sekunden
  in einen Timeout laufen. Gleichzeitig wird der Server lokal auf B geprüft.
- Kanal fortsetzen: UDP und HTTP müssen mit neuen Nonces wieder funktionieren.
- `pdn traffic export` liefert die Rohframes beider Stationen. Der kleine
  Trace-Adapter nutzt den unveränderten Packet.NET-AX.25-Parser im Strict-Modus.
  Für jeden erfolgreichen Request **und** jede Antwort verlangt der Runner
  identische vollständige IPv4-Bytes bei Sender-TX und Peer-RX, die passenden
  Rufzeichen und UI/PID `0xCC`. Anwendungserfolg allein reicht nicht.

## Gemessener Lauf

Referenzlauf `20261031T221825Z-9057`, Ergebnis **passed**, sechs erfolgreiche
Anwendungsaustausche, zwei erwartete Timeouts, alle sechs Trace-Prüfungen bestanden:

| Phase | Richtung | UDP-Echo | HTTP-GET |
|---|---|---:|---:|
| Kanal verbunden | A → B | 2,181 s | 11,218 s |
| Kanal verbunden | B → A | 6,309 s | 15,984 s |
| Kanal pausiert | A → B | Timeout | Timeout |
| Kanal wiederhergestellt | A → B | 11,867 s | 22,454 s |

Ein einzelner Lauf, **kein Leistungsversprechen**. Halbduplex-Kollisionen,
TCP-Wiederholungen und beim Pausieren gepufferte Frames beeinflussen die Zeiten.
Aufträge mit neuen Nonces verhindern, dass eine alte Antwort als Wiederherstellung
gezählt wird. Die Kanalpause modelliert eine Unterbrechung des gesamten Simulators,
keinen kalibrierten Funkverlust.

Gesicherter Nachweis: [`artifacts/m1/evidence`](../artifacts/m1/evidence/).
Neue Läufe schreiben vollständige Logs/Traces nach `artifacts/m1/runs/<lauf>`
und den letzten Bericht nach `artifacts/m1/latest.json`. Die elf Python-Tests
prüfen Audit-Guardrails und insbesondere, dass fehlende Empfangsframes,
veränderte IP-Bytes, falsche PIDs/Ziele oder alte Nonces nicht als Nachweis gelten:

```sh
python3 -m unittest discover -s tests -p 'test_*.py' -v
```

## Reproduzierbarkeit und Grenzen

Upstream-Commits, APRS-Submodul, .NET-SDK und beide Basis-/Simulator-Images
sind gepinnt. Ubuntu-Pakete im Laborimage werden beim Build aus den Paketquellen
installiert; ihre Versionsliste und die ausgeführte Image-ID liegen im Nachweis.
Der Build ist daher nicht als bitidentisch garantiert. Auf dem geprüften Host
brauchte der Docker-Build `--network=host --allow=network.host` wegen DNS-Problemen
im Build-Bridge-Netz. **Nur der Build** verwendet dieses Netz; das laufende
Labor bleibt isoliert. Es werden keine globalen DNS-/Firewallregeln geändert.

Dies bestätigt den grundlegenden M1-IP-Pfad samt Unterbrechung/Recovery.
Noch offen: deterministische Verlust-/Verzögerungsmatrix und Kollisionsabnahme,
größere/fragmentierte Pakete (Audit F4/F5), echte Gegenstellen/Hardware,
Windows-Wintun und Mehrhop. Der Zielfilter-Patch enthält keine neue
AX.25-/KISS-/TCP/IP-Implementierung und wurde nicht upstream veröffentlicht.

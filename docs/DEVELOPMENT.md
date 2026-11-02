# Entwicklung (Go-Kern)

## Einrichten

```sh
bash scripts/setup-go.sh                 # Go laut go.mod nach .tools/go, Prüfsumme von go.dev
export PATH="$PWD/.tools/go/bin:$PATH"
go test ./...                            # alle Tests
go vet ./...
go build -o axlinkd ./cmd/axlinkd
```

Ein systemweit installiertes Go in passender Version geht genauso.

## Programm

```text
axlinkd run          Funkgerät oder TNC verbinden, Netzadapter öffnen, IPv4 übertragen
axlinkd ports        serielle Ports; markiert Funkgeräte mit F4HWN-Firmware (USB 36B7:FFFF)
axlinkd decode       KISS-Strom von stdin als JSON je Frame
axlinkd sim-channel  nur Labor: simulierter Funkkanal
```

Beispiel mit Funkgerät (Linux; Rufzeichen sind Platzhalter, eigene eintragen):

```sh
./axlinkd ports
sudo ./axlinkd run -call <RUFZEICHEN>-1 -port auto -ip 10.73.0.1/24 -route 10.73.0.2=<GEGENSTELLE>-1
```

- `-port auto` wählt das eine angeschlossene F4HWN-Funkgerät; sonst `-port /dev/ttyACM0`.
  Andere TNCs: `-port` mit `-profile generic-kiss`, oder `-kiss-tcp host:port` (Dire Wolf).
- Der Netzadapter (`axl0`, MTU 256) braucht Root oder `CAP_NET_ADMIN`. Statt `sudo` geht
  `sudo setcap cap_net_admin+ep ./axlinkd`; diese Variante ist noch nicht geprüft.
- Der serielle Port braucht Leserechte, unter Fedora die Gruppe `dialout`.
- Nach einem USB-Reset (HF auf dem Kabel) verbindet AXLink von selbst neu und sendet die
  KISS-Parameter erneut. Ein Frame, der beim Abbruch nicht ganz ankam, geht einmal neu raus.
- Ohne Rufzeichen oder mit Platzhalter (`N0CALL`) startet `run` nicht.
- Broadcast und Multicast bleiben vom Funk fern; `-allow-broadcast` schaltet den Filter ab.
- Windows und macOS: serielle Ports gehen, der Netzadapter folgt (Wintun, utun).

Taktung je TNC: `-profile f4hwn-kisstnc` (Standard für serielle Ports),
`f4hwn-kisstnc-ackmode` (für die eigene App mit ACKMODE, Arbeitstitel AXLink TNC),
`generic-kiss` (Standard sonst).

## Labor ohne Funkgerät

```sh
bash scripts/lab-netns.sh                 # Modell der F4HWN-KISS-App, Profil f4hwn-kisstnc
bash scripts/lab-netns.sh f4hwn-ackmode   # dasselbe mit ACKMODE
bash scripts/lab-netns.sh ideal           # TNC mit Warteschlange
```

Zwei Stationen in eigenen User- und Netz-Namespaces (ohne root, ohne Docker) mit echten
Kernel-IP-Stacks und TUN-Geräten, verbunden über `axlinkd sim-channel`. Geprüft werden ping,
UDP und HTTP von A nach B. Der Trace des Kanals muss IPv4-Frames (UI, PID 0xCC) in beiden
Richtungen zeigen. Braucht `unshare`, `ip`, `python3`, `curl`, `ping`. Ergebnisse aus dem
Modell sind keine Hardware-Aussage.

## Aufbau

| Pfad | Inhalt |
|---|---|
| `cmd/axlinkd` | das Programm |
| `internal/kiss` | KISS-Framing, Parameter, ACKMODE (G8BPQ) |
| `internal/ax25` | AX.25-v2.2-Adressen und Frames (ohne FCS, wie über KISS), FCS |
| `internal/airtime` | Bit-Stuffing und Sendedauer eines Frames |
| `internal/pacing` | wie AXLink einen TNC füttert (Profile) |
| `internal/tnc` | Verbindung zum TNC: Serial, KISS-TCP, Unix-Socket; Neuverbinden; Sendetaktung |
| `internal/tun` | virtueller Netzadapter (Linux; Windows/macOS folgen) |
| `internal/ipbridge` | IPv4 ↔ AX.25 UI/PID 0xCC, Zielfilter, Routen, Broadcast-Filter |
| `internal/sim/f4hwnkiss` | Verhaltensmodell der KISS-TNC-App der F4HWN-Firmware |
| `internal/sim/channel` | Funkkanal für das Labor (Halbduplex, Kollisionen, Trace) |

Der Stand mit pdn/pdn-net (Python-Labor, Docker, `scripts/*.py`, `simulator/`, `tests/`) ist
das Labor von `v0.1.0-alpha.1`. Seine Python-Tests laufen in der CI weiter mit.

## Regeln

- Jede neue Quelldatei beginnt mit `// SPDX-License-Identifier: GPL-3.0-or-later`.
- Nur Platzhalter-Rufzeichen (`N0CALL`, `TESTA-1` …), nie echte (`DECISIONS.md`, D3).
- Keine eingebauten Rechtsbeschränkungen (D2); technische Kanalschutz-Filter sind abschaltbar.
- Ergebnisse aus Modell oder Simulator sind keine Hardware-Aussage. Hardware-Messungen werden
  getrennt dokumentiert.
- Parser sind streng: Was wir annehmen, codieren wir wieder gleich. Testvektoren anderer
  Implementierungen (z. B. `internal/ax25/testdata`) dienen als Gegenprobe.

## CI

`.github/workflows/ci.yml`:
- Linux: gofmt, vet, Tests mit `-race`, Builds für Linux amd64/arm64/ARMv6, Windows
  amd64/arm64 und macOS amd64/arm64, Python-Tests des v0.1-Labors.
- Labor: `lab-netns.sh` in den Modi `f4hwn` und `f4hwn-ackmode`.
- Windows- und macOS-Tests.

Bei privaten Repos zählen GitHub Windows-Minuten doppelt und macOS-Minuten zehnfach.

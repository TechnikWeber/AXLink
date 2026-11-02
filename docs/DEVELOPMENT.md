# Entwicklung (Go-Kern)

## Einrichten

```sh
bash scripts/setup-go.sh                 # Go laut go.mod nach .tools/go, Prüfsumme von go.dev
export PATH="$PWD/.tools/go/bin:$PATH"
go test ./...                            # alle Tests
go vet ./...
go run ./cmd/axlinkd version
```

Ein systemweit installiertes Go in passender Version geht genauso.

## Aufbau

| Pfad | Inhalt |
|---|---|
| `cmd/axlinkd` | das Programm; bisher `version` und `decode` (KISS-Strom von stdin → JSON je Frame) |
| `internal/kiss` | KISS-Framing, Parameter, ACKMODE (G8BPQ) |
| `internal/ax25` | AX.25-v2.2-Adressen und Frames (ohne FCS, wie über KISS), FCS |
| `internal/airtime` | Bit-Stuffing und Sendedauer eines Frames |
| `internal/pacing` | wie AXLink einen TNC füttert (Profile `generic-kiss`, `f4hwn-kisstnc`, `f4hwn-kisstnc-ackmode`) |
| `internal/sim/f4hwnkiss` | Verhaltensmodell der KISS-TNC-App der F4HWN-Firmware (aus ihrem Quelltext) |

Der Stand mit pdn/pdn-net (Python-Labor, Docker, `scripts/`, `simulator/`, `tests/`) ist das
Labor von `v0.1.0-alpha.1`. Seine Python-Tests laufen in der CI weiter mit, bis ein Go-Labor
es ersetzt.

## Regeln

- Jede neue Quelldatei beginnt mit `// SPDX-License-Identifier: GPL-3.0-or-later`.
- Nur Platzhalter-Rufzeichen (`N0CALL`, `TESTA-1` …), nie echte (`DECISIONS.md`, D3).
- Keine eingebauten Rechtsbeschränkungen (D2); technische Kanalschutz-Filter sind abschaltbar.
- Ergebnisse aus Modell oder Simulator sind keine Hardware-Aussage. Hardware-Messungen werden
  getrennt dokumentiert.
- Parser sind streng: Was wir annehmen, codieren wir wieder gleich. Testvektoren anderer
  Implementierungen (z. B. `internal/ax25/testdata`) dienen als Gegenprobe.

## CI

`.github/workflows/ci.yml`: Linux (gofmt, vet, Tests mit `-race`, Builds für Linux
amd64/arm64/ARMv6, Windows amd64/arm64, macOS amd64/arm64, Python-Tests des v0.1-Labors),
Windows- und macOS-Tests. Bei privaten Repos zählen GitHub Windows-Minuten doppelt und
macOS-Minuten zehnfach.

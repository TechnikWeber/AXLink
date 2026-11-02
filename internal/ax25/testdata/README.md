# Testvektoren aus Packet.NET

`packetnet-frames.jsonl` sind rohe AX.25-Frames (ohne FCS), die der Packet.NET-Knoten (`pdn`,
Commit `bded4671`) im Zwei-Knoten-Labor vom 31.10.2026 gesendet und empfangen hat
(`pdn traffic export`, Station A). `packetnet-decoded.jsonl` ist dieselbe Liste, Zeile für
Zeile vom strikten Packet.NET-Parser dekodiert (`tools/TraceCheck`).

Quelle: `artifacts/m1/evidence/a-ax25.jsonl` und `a-decoded.jsonl` (Tag `v0.1.0-alpha.1`).
Sie dienen als Gegenprobe: unser Parser muss dieselben Felder lesen und beim Neucodieren
byte-gleiche Frames erzeugen.

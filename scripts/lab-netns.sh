#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Two AXLink stations in their own network namespaces (no root, no Docker),
# connected through the simulated radio channel. The stations have no routes:
# they find each other by their beacons. Then the lab checks ping, UDP and HTTP
# from A to B with real kernel IP stacks. No radio hardware.
#
#   bash scripts/lab-netns.sh [f4hwn|f4hwn-ackmode|ideal] [ADAPTER_MTU]
#
# f4hwn (default): the model of the F4HWN KISS app in front of each station,
# AXLink with the profile f4hwn-kisstnc. Needs unshare (util-linux), ip
# (iproute2), python3, curl, ping and unprivileged user namespaces.
#
# ADAPTER_MTU (e.g. 576, the Windows minimum) gives both adapters a larger
# MTU than the radio's 256 bytes. The lab then also checks that the bridge
# lowers the TCP MSS and fragments: a 400-byte ping without DF and a 2 KB
# HTTP download.
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-f4hwn}"
ADAPTER_MTU="${2:-}"
if [[ -n "$ADAPTER_MTU" && ! "$ADAPTER_MTU" =~ ^[0-9]+$ ]]; then
    echo "ADAPTER_MTU: a number, e.g. 576" >&2
    exit 2
fi
case "$MODE" in
    f4hwn) CHANNEL=(-mode f4hwn); PROFILE=f4hwn-kisstnc ;;
    f4hwn-ackmode) CHANNEL=(-mode f4hwn -ackmode); PROFILE=f4hwn-kisstnc-ackmode ;;
    ideal) CHANNEL=(-mode ideal); PROFILE=generic-kiss ;;
    *) echo "mode: f4hwn, f4hwn-ackmode or ideal" >&2; exit 2 ;;
esac
for tool in unshare ip python3 curl ping; do
    command -v "$tool" >/dev/null || { echo "missing: $tool" >&2; exit 2; }
done
GO="$ROOT/.tools/go/bin/go"
[[ -x "$GO" ]] || GO=go

WORK="$(mktemp -d "${TMPDIR:-/tmp}/axlink-lab.XXXXXX")"
# Unix socket paths may have at most 108 bytes (104 on macOS): keep them short.
SOCK="$(mktemp -d /tmp/axl.XXXXXX)"
PIDS=()
cleanup() {
    touch "$WORK/stop"
    sleep 1
    for pid in "${PIDS[@]}"; do kill "$pid" 2>/dev/null || true; done
    wait 2>/dev/null || true
    rm -rf "$SOCK"
}
trap cleanup EXIT

echo "lab: $WORK (mode $MODE${ADAPTER_MTU:+, adapter MTU $ADAPTER_MTU})"
(cd "$ROOT" && "$GO" build -o "$WORK/axlinkd" ./cmd/axlinkd)
NONCE="$(python3 -I -c 'import secrets; print(secrets.token_hex(8))')"
mkdir -p "$WORK/www"
echo "AXLink $NONCE" > "$WORK/www/hello.txt"
# 2 KB in 32 lines: more than ten TCP segments at the radio's MSS of 216.
python3 -I -c 'import secrets, sys
for i in range(32): print(f"{i:02d} {sys.argv[1]} " + secrets.token_hex(22))' "$NONCE" > "$WORK/www/big.txt"

cat > "$WORK/udp_echo.py" <<'PY'
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind((sys.argv[1], 7070))
while True:
    data, peer = s.recvfrom(512)
    s.sendto(data, peer)
PY
cat > "$WORK/udp_probe.py" <<'PY'
import socket, sys, time
payload = ("AXLink-udp-" + sys.argv[2]).encode()
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.connect((sys.argv[1], 7070))
start, deadline, tries = time.monotonic(), time.monotonic() + 60, 0
while time.monotonic() < deadline:
    tries += 1
    s.send(payload)
    s.settimeout(min(15, deadline - time.monotonic()))
    try:
        if s.recv(512) == payload:
            print(f"ok {time.monotonic() - start:.1f}s tries={tries}")
            raise SystemExit(0)
    except TimeoutError:
        pass
print("timeout")
raise SystemExit(1)
PY

# One script per station, run inside its own user + network namespace.
cat > "$WORK/node.sh" <<'SH'
set -eu
WORK=$1; NAME=$2; CALL=$3; IP=$4; PEER_IP=$5; PEER_CALL=$6; PROFILE=$7; SOCK=$8; ADAPTER_MTU=${9:-}
ip link set lo up
started=$(date +%s%N)
"$WORK/axlinkd" run -call "$CALL" -kiss-unix "$SOCK/$NAME.sock" -profile "$PROFILE" \
    -ip "$IP/24" ${ADAPTER_MTU:+-adapter-mtu "$ADAPTER_MTU"} 2> "$WORK/node-$NAME.log" &
NODE=$!
for _ in $(seq 100); do ip -4 addr show axl0 2>/dev/null | grep -q "$IP" && break; sleep 0.1; done
ip -4 addr show axl0 | grep -q "$IP" || { echo "axl0 did not come up" >&2; kill $NODE; exit 1; }
if [ "$NAME" = b ]; then
    python3 -I "$WORK/udp_echo.py" "$IP" &
    python3 -I -m http.server 8080 --bind "$IP" --directory "$WORK/www" > /dev/null 2>&1 &
    sleep 1
    touch "$WORK/b.ready"
    while [ ! -e "$WORK/stop" ]; do sleep 0.2; done
    kill $(jobs -p) 2>/dev/null || true
    exit 0
fi
for _ in $(seq 300); do [ -e "$WORK/b.ready" ] && break; sleep 0.1; done
status=0
# No routes: wait until both stations have heard each other's beacon.
found=0
for _ in $(seq 600); do
    if grep -q "msg=\"station heard\" call=$PEER_CALL " "$WORK/node-$NAME.log" &&
        grep -q "msg=\"station heard\" call=$CALL " "$WORK/node-b.log"; then
        found=1
        break
    fi
    sleep 0.1
done
echo $(( ($(date +%s%N) - started) / 1000000 )) > "$WORK/discovery.ms"
[ $found = 1 ] || status=1
LC_ALL=C ping -c 3 -i 4 -W 30 "$PEER_IP" > "$WORK/ping.log" 2>&1 || status=1
python3 -I "$WORK/udp_probe.py" "$PEER_IP" "$NONCE_ENV" > "$WORK/udp.log" 2>&1 || status=1
start=$(date +%s%N)
curl -sS --max-time 180 "http://$PEER_IP:8080/hello.txt" > "$WORK/http.out" 2> "$WORK/http.err" || status=1
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/http.ms"
if [ -n "$ADAPTER_MTU" ]; then
    # 428-byte packets: the bridges fragment them in both directions.
    LC_ALL=C ping -c 2 -i 8 -W 60 -M dont -s 400 "$PEER_IP" > "$WORK/bigping.log" 2>&1 || status=1
    start=$(date +%s%N)
    curl -sS --max-time 300 "http://$PEER_IP:8080/big.txt" > "$WORK/big.out" 2> "$WORK/big.err" || status=1
    echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/big.ms"
fi
kill $NODE 2>/dev/null || true
wait $NODE 2>/dev/null || true
exit $status
SH

"$WORK/axlinkd" sim-channel -station "a=$SOCK/a.sock" -station "b=$SOCK/b.sock" "${CHANNEL[@]}" \
    -trace "$WORK/trace.jsonl" 2> "$WORK/channel.log" &
PIDS+=($!)
for _ in $(seq 50); do [[ -S "$SOCK/a.sock" && -S "$SOCK/b.sock" ]] && break; sleep 0.1; done
if [[ ! -S "$SOCK/a.sock" || ! -S "$SOCK/b.sock" ]]; then
    echo "the channel did not start:" >&2
    cat "$WORK/channel.log" >&2
    exit 1
fi

unshare -Urn sh "$WORK/node.sh" "$WORK" b TESTB-1 10.73.0.2 10.73.0.1 TESTA-1 "$PROFILE" "$SOCK" "$ADAPTER_MTU" &
PIDS+=($!)
started=$(date +%s)
status=0
NONCE_ENV="$NONCE" unshare -Urn sh "$WORK/node.sh" "$WORK" a TESTA-1 10.73.0.1 10.73.0.2 TESTB-1 "$PROFILE" "$SOCK" "$ADAPTER_MTU" || status=1

echo
echo "== Result ($MODE${ADAPTER_MTU:+, adapter MTU $ADAPTER_MTU}, $(( $(date +%s) - started )) s) =="
if grep -q "call=TESTB-1 " "$WORK/node-a.log" && grep -q "call=TESTA-1 " "$WORK/node-b.log"; then
    echo "found: both stations by their beacons, $(( $(cat "$WORK/discovery.ms") / 1000 )).$(( $(cat "$WORK/discovery.ms") % 1000 / 100 )) s after A started"
else
    echo "found: FAILED, the stations did not hear each other's beacons"
    status=1
fi
echo "ping:  $(grep -E 'packets transmitted|rtt' "$WORK/ping.log" | tr '\n' ' ')"
echo "UDP:   $(cat "$WORK/udp.log")"
if grep -qx "AXLink $NONCE" "$WORK/http.out" 2>/dev/null; then
    echo "HTTP:  ok ($(( $(cat "$WORK/http.ms") / 1000 )).$(( $(cat "$WORK/http.ms") % 1000 / 100 )) s)"
else
    echo "HTTP:  FAILED $(cat "$WORK/http.err" 2>/dev/null)"
    status=1
fi
if [[ -n "$ADAPTER_MTU" ]]; then
    echo "ping 400 B without DF: $(grep -E 'packets transmitted' "$WORK/bigping.log" 2>/dev/null)"
    if cmp -s "$WORK/www/big.txt" "$WORK/big.out" 2>/dev/null; then
        echo "HTTP 2 KB: ok ($(( $(cat "$WORK/big.ms") / 1000 )).$(( $(cat "$WORK/big.ms") % 1000 / 100 )) s)"
    else
        echo "HTTP 2 KB: FAILED $(cat "$WORK/big.err" 2>/dev/null)"
        status=1
    fi
    # Station A's last statistics line, written when it stops.
    counters="$(grep -o 'Clamped:[0-9]* Fragmented:[0-9]* TooBig:[0-9]*' "$WORK/node-a.log" | tail -n 1)"
    echo "bridge A: ${counters:-no statistics}"
    if [[ ! "$counters" =~ Clamped:[1-9].*Fragmented:[1-9] ]]; then
        echo "FAILED: station A neither lowered an MSS nor fragmented a packet"
        status=1
    fi
fi
python3 -I - "$WORK/trace.jsonl" <<'PY' || status=1
import json, sys
tx = {"a": 0, "b": 0}; ip = {"a": 0, "b": 0}; beacons = {"a": 0, "b": 0}; rx = 0; lost = {}; dropped = {}
air = 0.0
for line in open(sys.argv[1]):
    e = json.loads(line)
    if e["ev"] == "tx":
        tx[e["from"]] += 1
        air += e["dur"]
        if e["frame"][30:32] == "cc":
            ip[e["from"]] += 1
        dest = bytes(b >> 1 for b in bytes.fromhex(e["frame"][:12])).decode("ascii", "replace")
        if dest == "AXLINK" and e["frame"][30:32] == "f0":
            beacons[e["from"]] += 1
    elif e["ev"] == "rx":
        rx += 1
    elif e["ev"] == "drop":  # inside the TNC model, never on the air
        key = e["from"].upper() + " " + e["why"]
        dropped[key] = dropped.get(key, 0) + 1
    else:
        lost[e["why"]] = lost.get(e["why"], 0) + 1
print(f"channel: {tx['a']} frames from A, {tx['b']} from B (IPv4/PID 0xCC: {ip['a']} / {ip['b']},"
      f" beacons {beacons['a']} / {beacons['b']}), {rx} received, lost on the air {lost or 0}, airtime {air:.0f} s")
print(f"TNC:     frames dropped before the air {dropped or 0}")
if not (ip["a"] and ip["b"]):
    print("FAILED: no IP frames in both directions on the channel")
    raise SystemExit(1)
PY
echo "logs and trace: $WORK"
exit $status

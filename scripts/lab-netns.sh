#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Two AXLink stations in their own network namespaces (no root, no Docker),
# connected through the simulated radio channel. Checks ping, UDP and HTTP
# from A to B with real kernel IP stacks. No radio hardware.
#
#   bash scripts/lab-netns.sh [f4hwn|f4hwn-ackmode|ideal]
#
# f4hwn (default): the model of the F4HWN KISS app in front of each station,
# AXLink with the profile f4hwn-kisstnc. Needs unshare (util-linux), ip
# (iproute2), python3, curl, ping and unprivileged user namespaces.
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-f4hwn}"
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

echo "lab: $WORK (mode $MODE)"
(cd "$ROOT" && "$GO" build -o "$WORK/axlinkd" ./cmd/axlinkd)
NONCE="$(python3 -I -c 'import secrets; print(secrets.token_hex(8))')"
mkdir -p "$WORK/www"
echo "AXLink $NONCE" > "$WORK/www/hello.txt"

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
WORK=$1; NAME=$2; CALL=$3; IP=$4; PEER_IP=$5; PEER_CALL=$6; PROFILE=$7; SOCK=$8
ip link set lo up
"$WORK/axlinkd" run -call "$CALL" -kiss-unix "$SOCK/$NAME.sock" -profile "$PROFILE" \
    -ip "$IP/24" -route "$PEER_IP=$PEER_CALL" 2> "$WORK/node-$NAME.log" &
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
LC_ALL=C ping -c 3 -i 4 -W 30 "$PEER_IP" > "$WORK/ping.log" 2>&1 || status=1
python3 -I "$WORK/udp_probe.py" "$PEER_IP" "$NONCE_ENV" > "$WORK/udp.log" 2>&1 || status=1
start=$(date +%s%N)
curl -sS --max-time 180 "http://$PEER_IP:8080/hello.txt" > "$WORK/http.out" 2> "$WORK/http.err" || status=1
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/http.ms"
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

unshare -Urn sh "$WORK/node.sh" "$WORK" b TESTB-1 10.73.0.2 10.73.0.1 TESTA-1 "$PROFILE" "$SOCK" &
PIDS+=($!)
started=$(date +%s)
status=0
NONCE_ENV="$NONCE" unshare -Urn sh "$WORK/node.sh" "$WORK" a TESTA-1 10.73.0.1 10.73.0.2 TESTB-1 "$PROFILE" "$SOCK" || status=1

echo
echo "== Result ($MODE, $(( $(date +%s) - started )) s) =="
echo "ping:  $(grep -E 'packets transmitted|rtt' "$WORK/ping.log" | tr '\n' ' ')"
echo "UDP:   $(cat "$WORK/udp.log")"
if grep -qx "AXLink $NONCE" "$WORK/http.out" 2>/dev/null; then
    echo "HTTP:  ok ($(( $(cat "$WORK/http.ms") / 1000 )).$(( $(cat "$WORK/http.ms") % 1000 / 100 )) s)"
else
    echo "HTTP:  FAILED $(cat "$WORK/http.err" 2>/dev/null)"
    status=1
fi
python3 -I - "$WORK/trace.jsonl" <<'PY' || status=1
import json, sys
tx = {"a": 0, "b": 0}; ip = {"a": 0, "b": 0}; rx = 0; lost = {}
air = 0.0
for line in open(sys.argv[1]):
    e = json.loads(line)
    if e["ev"] == "tx":
        tx[e["from"]] += 1
        air += e["dur"]
        if e["frame"][30:32] == "cc":
            ip[e["from"]] += 1
    elif e["ev"] == "rx":
        rx += 1
    else:
        lost[e["why"]] = lost.get(e["why"], 0) + 1
print(f"channel: {tx['a']} frames from A, {tx['b']} from B (IPv4/PID 0xCC: {ip['a']} / {ip['b']}),"
      f" {rx} received, lost {lost or 0}, airtime {air:.0f} s")
if not (ip["a"] and ip["b"]):
    print("FAILED: no IP frames in both directions on the channel")
    raise SystemExit(1)
PY
echo "logs and trace: $WORK"
exit $status

#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Three AXLink stations in their own network namespaces on one simulated
# channel, in a line: A hears B, B hears C, A and C do not hear each other.
# B is a relay (-relay). The stations have no routes: A and C find each other
# by their beacons, which B passes on (flood alias AXHOP-2), and then reach
# each other over B: ping, HTTP and a chat message from A to C, a CQ from A
# in C's room. No radio hardware.
#
#   bash scripts/lab-mesh.sh [f4hwn|f4hwn-ackmode|ideal]
#
# AXLINK_LAB_SCREENSHOTS=DIR saves screenshots of A's web interface there
# (mesh-overview, mesh-chat; English and German), if Google Chrome is
# installed.
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-f4hwn-ackmode}"
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

WORK="$(mktemp -d "${TMPDIR:-/tmp}/axlink-mesh.XXXXXX")"
SOCK="$(mktemp -d /tmp/axl.XXXXXX)" # Unix socket paths may have at most 108 bytes
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
echo "AXLink over a relay $NONCE" > "$WORK/www/hello.txt"
# Locators about 11 km apart in a line, announced in the beacons: the
# station lists show distance and direction.
declare -A LOC=([a]=JO62qm [b]=JO62sm [c]=JO62um)
for n in a b c; do
    cat > "$SOCK/settings-$n.json" <<JSON
{"welcome": {"enabled": true, "port": 80, "name": "Lab station ${n^^}", "qth": "a network namespace",
 "locator": "${LOC[$n]}", "locator_in_beacon": true,
 "equipment": "the model of the F4HWN KISS app", "text": "A station of the AXLink mesh lab."}}
JSON
done

cat > "$WORK/node.sh" <<'SH'
set -eu
WORK=$1; NAME=$2; CALL=$3; IP=$4; PROFILE=$5; SOCK=$6; EXTRA=$7
ip link set lo up
shoot() {
    [ -n "${AXLINK_LAB_SCREENSHOTS:-}" ] && command -v google-chrome > /dev/null || return 0
    for lang in en de; do
        locale=en_US.UTF-8
        [ "$lang" = de ] && locale=de_DE.UTF-8
        TMPDIR="$SOCK" LANGUAGE="$lang" LANG="$locale" google-chrome --headless=new --no-sandbox --disable-gpu \
            --hide-scrollbars --no-first-run --user-data-dir="$WORK/chrome-$NAME-$lang" --lang="$lang" \
            --window-size=1280,"$3" --virtual-time-budget=6000 \
            --screenshot="$AXLINK_LAB_SCREENSHOTS/$1-$lang.png" "http://127.0.0.1:7373/?lang=$lang$2" \
            > "$WORK/chrome-$NAME-$1-$lang.log" 2>&1 || true
    done
}
"$WORK/axlinkd" run -call "$CALL" -kiss-unix "$SOCK/$NAME.sock" -profile "$PROFILE" -ip "$IP/24" \
    -config "$SOCK/settings-$NAME.json" $EXTRA 2> "$WORK/node-$NAME.log" &
NODE=$!
for _ in $(seq 100); do ip -4 addr show axl0 2>/dev/null | grep -q "$IP" && break; sleep 0.1; done
ip -4 addr show axl0 | grep -q "$IP" || { echo "axl0 did not come up" >&2; kill $NODE; exit 1; }
ui="http://127.0.0.1:7373"
if [ "$NAME" != a ]; then
    [ "$NAME" = c ] && python3 -I -m http.server 8080 --bind "$IP" --directory "$WORK/www" > /dev/null 2>&1 &
    touch "$WORK/$NAME.ready"
    while [ ! -e "$WORK/stop" ]; do
        if [ -e "$WORK/$NAME.dump" ]; then        # A has finished: what this station saw
            rm -f "$WORK/$NAME.dump"
            curl -s --max-time 10 "$ui/api/status" > "$WORK/$NAME-status.json" 2> /dev/null || true
            curl -s --max-time 10 "$ui/api/chat/messages?peer=TESTA-1" > "$WORK/$NAME-chat.json" 2> /dev/null || true
            curl -s --max-time 10 "$ui/api/chat/messages?peer=CQ" > "$WORK/$NAME-cq.json" 2> /dev/null || true
            touch "$WORK/$NAME.dumped"
        fi
        sleep 0.2
    done
    kill $(jobs -p) 2>/dev/null || true
    exit 0
fi
for n in b c; do
    for _ in $(seq 300); do [ -e "$WORK/$n.ready" ] && break; sleep 0.1; done
done
status=0
started=$(date +%s%N)
# A hears C only over B: wait for C's beacon, passed on by B.
found=0
for _ in $(seq 1200); do
    if grep -q 'msg="station heard" call=TESTC-1 .*via=TESTB-1' "$WORK/node-a.log" &&
        grep -q 'msg="station heard" call=TESTA-1 .*via=TESTB-1' "$WORK/node-c.log"; then
        found=1
        break
    fi
    sleep 0.1
done
echo $(( ($(date +%s%N) - started) / 1000000 )) > "$WORK/discovery.ms"
[ $found = 1 ] || status=1
LC_ALL=C ping -c 3 -i 6 -W 40 10.73.0.3 > "$WORK/ping.log" 2>&1 || status=1
start=$(date +%s%N)
curl -sS --max-time 240 "http://10.73.0.3:8080/hello.txt" > "$WORK/http.out" 2> "$WORK/http.err" || status=1
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/http.ms"
chat() { curl -s --max-time 10 -X POST -H 'Content-Type: application/json' -d "$2" "$ui/api/chat/$1"; }
start=$(date +%s%N)
chat send "{\"to\":\"TESTC-1\",\"text\":\"Hello C, over B. $NONCE_ENV\"}" > /dev/null 2>&1 || status=1
for _ in $(seq 1800); do
    curl -s --max-time 5 "$ui/api/chat/messages?peer=TESTC-1" > "$WORK/a-chat.json" 2> /dev/null || true
    grep -q '"state":"delivered"' "$WORK/a-chat.json" && break
    sleep 0.1
done
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/chat.ms"
chat send '{"to":"CQ","text":"CQ from A: who hears me over the relay?"}' > /dev/null 2>&1 || status=1
sleep 8
curl -s --max-time 10 "$ui/api/status" > "$WORK/a-status.json" 2> /dev/null || true
shoot mesh-overview / 1450
shoot mesh-chat "#chat/TESTC-1" 820
for n in b c; do touch "$WORK/$n.dump"; done
for n in b c; do
    for _ in $(seq 300); do [ -e "$WORK/$n.dumped" ] && break; sleep 0.1; done
done
kill $NODE 2>/dev/null || true
wait $NODE 2>/dev/null || true
exit $status
SH

"$WORK/axlinkd" sim-channel -station "a=$SOCK/a.sock" -station "b=$SOCK/b.sock" -station "c=$SOCK/c.sock" \
    -link a-b -link b-c "${CHANNEL[@]}" -trace "$WORK/trace.jsonl" 2> "$WORK/channel.log" &
PIDS+=($!)
for _ in $(seq 50); do [[ -S "$SOCK/a.sock" && -S "$SOCK/b.sock" && -S "$SOCK/c.sock" ]] && break; sleep 0.1; done
unshare -Urn sh "$WORK/node.sh" "$WORK" b TESTB-1 10.73.0.2 "$PROFILE" "$SOCK" "-relay" &
PIDS+=($!)
unshare -Urn sh "$WORK/node.sh" "$WORK" c TESTC-1 10.73.0.3 "$PROFILE" "$SOCK" "-open 8080/tcp" &
PIDS+=($!)
started=$(date +%s)
status=0
NONCE_ENV="$NONCE" unshare -Urn sh "$WORK/node.sh" "$WORK" a TESTA-1 10.73.0.1 "$PROFILE" "$SOCK" "" || status=1

secs() { echo "$(( $(cat "$1") / 1000 )).$(( $(cat "$1") % 1000 / 100 )) s"; }
echo
echo "== Result (mesh, $MODE, $(( $(date +%s) - started )) s) =="
if grep -q 'call=TESTC-1 .*via=TESTB-1' "$WORK/node-a.log" && grep -q 'call=TESTA-1 .*via=TESTB-1' "$WORK/node-c.log"; then
    echo "found: A and C each other over B, $(secs "$WORK/discovery.ms") after A started"
else
    echo "found: FAILED, A and C did not learn each other over B"
    status=1
fi
echo "ping:  $(grep -E 'packets transmitted|rtt' "$WORK/ping.log" | tr '\n' ' ')"
if grep -qx "AXLink over a relay $NONCE" "$WORK/http.out" 2>/dev/null; then
    echo "HTTP:  ok, A from C over B ($(secs "$WORK/http.ms"))"
else
    echo "HTTP:  FAILED $(cat "$WORK/http.err" 2>/dev/null)"
    status=1
fi
if python3 -I - "$WORK" "$NONCE" <<'PY'
import json, os, sys
work, nonce = sys.argv[1], sys.argv[2]
def load(name, default):
    try:
        with open(os.path.join(work, name)) as f:
            return json.load(f)
    except (OSError, ValueError):
        return default
a, c, cq = load("a-chat.json", []), load("c-chat.json", []), load("c-cq.json", [])
sent = [m for m in a if m.get("out") and nonce in m["text"]]
got = [m for m in c if not m.get("out") and nonce in m["text"]]
room = [m for m in cq if m.get("from") == "TESTA-1"]
astat, bstat = load("a-status.json", {}), load("b-status.json", {})
listed = [s for s in astat.get("stations") or [] if s["call"] == "TESTC-1"]
relay = {c["key"]: c["value"] for c in bstat.get("counters") or [] if c["group"] == "mesh"}
ok = (sent and sent[0]["state"] == "delivered" and len(got) == 1 and len(room) == 1
      and listed and listed[0].get("via") == ["TESTB-1"] and bstat.get("relay")
      and relay.get("relay_repeated", 0) > 0 and relay.get("relay_flooded", 0) > 0)
print(f"chat:  A → C {sent[0]['state'] if sent else 'not sent'} after {sent[0]['tries'] if sent else 0} tries,"
      f" C got it {len(got)}x, A's CQ in C's room {len(room)}x")
print(f"relay: B repeated {relay.get('relay_repeated', 0)}, passed on {relay.get('relay_flooded', 0)}"
      f" beacons and CQ, held back {relay.get('relay_duplicates', 0)} duplicates;"
      f" A lists C via {listed[0].get('via') if listed else None}")
raise SystemExit(0 if ok else 1)
PY
then
    echo "mesh:  ok (chat delivered over B in $(secs "$WORK/chat.ms"))"
else
    echo "mesh:  FAILED"
    status=1
fi
python3 -I - "$WORK/trace.jsonl" <<'PY' || true
import json, sys
tx, lost = {}, {}
air = 0.0
for line in open(sys.argv[1]):
    e = json.loads(line)
    if e["ev"] == "tx":
        tx[e["from"]] = tx.get(e["from"], 0) + 1
        air += e["dur"]
    elif e["ev"] == "lost":
        lost[e["why"]] = lost.get(e["why"], 0) + 1
print(f"channel: frames sent {tx}, lost {lost or 0}, airtime {air:.0f} s")
PY
echo "logs and trace: $WORK"
exit $status

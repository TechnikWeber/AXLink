#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Two AXLink stations in their own network namespaces (no root, no Docker),
# connected through the simulated radio channel. The stations have no routes:
# they find each other by their beacons. Then the lab checks ping, UDP and HTTP
# from A to B with real kernel IP stacks, and the internet through B as a
# gateway: B's HTTP proxy reaches a web server on 192.0.2.80, an address A has
# no route to. Then A chooses B in its web interface and fetches the same
# file through its local proxy, opens B's welcome page over the radio, and
# finds B's firewall blocking a port B has not opened. No radio hardware.
#
# AXLINK_LAB_SCREENSHOTS=DIR saves screenshots of both web interfaces there
# (A as a client, B as a gateway; English and German), if Google Chrome is
# installed.
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
mkdir -p "$WORK/www" "$WORK/inet" "$WORK/lan"
echo "AXLink $NONCE" > "$WORK/www/hello.txt"
echo "from the internet $NONCE" > "$WORK/inet/inet.txt"
echo "from B's own network $NONCE" > "$WORK/lan/lan.txt"
# Each station keeps its settings in the lab's short socket folder (it shows in
# the screenshots), never in the user's own settings.
cat > "$SOCK/settings-a.json" <<'JSON'
{"welcome": {"enabled": true, "port": 80, "name": "Lab station A", "qth": "a network namespace",
 "equipment": "the model of the F4HWN KISS app",
 "text": "A client of the AXLink lab. It reaches the internet through TESTB-1."}}
JSON
cat > "$SOCK/settings-b.json" <<'JSON'
{"welcome": {"enabled": true, "port": 80, "name": "Lab station B", "qth": "a network namespace",
 "equipment": "the model of the F4HWN KISS app",
 "text": "The gateway of the AXLink lab. It shares a stand-in internet (192.0.2.80) with the radio network.\n\nEvery station has a page like this one on its radio address."}}
JSON
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
# shoot NAME PAGE HEIGHT: screenshots of a page in English and German; PAGE
# is a path of this station's web interface, or a full URL.
shoot() {
    [ -n "${AXLINK_LAB_SCREENSHOTS:-}" ] && command -v google-chrome > /dev/null || return 0
    for lang in en de; do
        case "$2" in
            http*) url=$2 ;;
            *) url="http://127.0.0.1:7373/?lang=$lang${2#/}" ;;
        esac
        # Chrome puts a socket into TMPDIR: the lab's short socket directory fits.
        # On Linux it takes the language from LANGUAGE/LANG, not from --lang.
        locale=en_US.UTF-8
        [ "$lang" = de ] && locale=de_DE.UTF-8
        TMPDIR="$SOCK" LANGUAGE="$lang" LANG="$locale" google-chrome --headless=new --no-sandbox --disable-gpu \
            --hide-scrollbars --no-first-run --user-data-dir="$WORK/chrome-$NAME-$lang" --lang="$lang" \
            --window-size=1280,"$3" --virtual-time-budget=6000 \
            --screenshot="$AXLINK_LAB_SCREENSHOTS/$1-$lang.png" "$url" > "$WORK/chrome-$NAME-$1-$lang.log" 2>&1 || true
    done
}
started=$(date +%s%N)
GATEWAY=
if [ "$NAME" = b ]; then
    # B shares its "internet", opens the ports of the lab's test services and
    # forwards 8022 (for TESTA only) and 8023 (for TESTC only) to a server of
    # its own network that listens on 127.0.0.1 only.
    GATEWAY="-proxy 3128 -open 8080/tcp -open 7070/udp -forward 8022=127.0.0.1:8090=TESTA -forward 8023=127.0.0.1:8090=TESTC"
    ip addr add 192.0.2.80/32 dev lo            # TEST-NET-1: the internet stand-in
fi
"$WORK/axlinkd" run -call "$CALL" -kiss-unix "$SOCK/$NAME.sock" -profile "$PROFILE" \
    -ip "$IP/24" ${ADAPTER_MTU:+-adapter-mtu "$ADAPTER_MTU"} $GATEWAY -config "$SOCK/settings-$NAME.json" \
    2> "$WORK/node-$NAME.log" &
NODE=$!
for _ in $(seq 100); do ip -4 addr show axl0 2>/dev/null | grep -q "$IP" && break; sleep 0.1; done
ip -4 addr show axl0 | grep -q "$IP" || { echo "axl0 did not come up" >&2; kill $NODE; exit 1; }
if [ "$NAME" = b ]; then
    python3 -I "$WORK/udp_echo.py" "$IP" &
    python3 -I -m http.server 8080 --bind "$IP" --directory "$WORK/www" > /dev/null 2>&1 &
    python3 -I -m http.server 8081 --bind 192.0.2.80 --directory "$WORK/inet" > /dev/null 2>&1 &
    python3 -I -m http.server 8090 --bind 127.0.0.1 --directory "$WORK/lan" > /dev/null 2>&1 &
    sleep 1
    touch "$WORK/b.ready"
    while [ ! -e "$WORK/stop" ]; do
        if [ -e "$WORK/a.chat" ]; then          # A has written: answer it
            rm -f "$WORK/a.chat"
            curl -s --max-time 10 -X POST -H 'Content-Type: application/json' \
                -d "{\"to\":\"$PEER_CALL\",\"text\":\"Hi, welcome to the lab. 73\"}" \
                http://127.0.0.1:7373/api/chat/send > /dev/null 2>&1 || true
        fi
        if [ -e "$WORK/b.shoot" ]; then         # A has finished: B's status and screenshots
            rm -f "$WORK/b.shoot"
            curl -s --max-time 10 http://127.0.0.1:7373/api/status > "$WORK/b-status.json" 2> /dev/null || true
            curl -s --max-time 10 "http://127.0.0.1:7373/api/chat/messages?peer=$PEER_CALL" > "$WORK/b-chat.json" 2> /dev/null || true
            curl -s --max-time 10 "http://127.0.0.1:7373/api/chat/messages?peer=CQ" > "$WORK/b-cq.json" 2> /dev/null || true
            shoot gateway-overview / 1700
            shoot gateway-settings "#settings" 1250
            shoot gateway-chat "#chat/$PEER_CALL" 860
            touch "$WORK/b.shot"
        fi
        sleep 0.2
    done
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
# The internet through B: a request through the proxy, the same through a
# CONNECT tunnel (as HTTPS uses it), the gateway's own network refused, and
# no direct way to the server.
proxy="http://$PEER_IP:3128"
start=$(date +%s%N)
curl -sS --max-time 180 -x "$proxy" "http://192.0.2.80:8081/inet.txt" > "$WORK/gw.out" 2> "$WORK/gw.err" || status=1
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/gw.ms"
start=$(date +%s%N)
curl -sS --max-time 180 -p -x "$proxy" "http://192.0.2.80:8081/inet.txt" > "$WORK/gwc.out" 2> "$WORK/gwc.err" || status=1
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/gwc.ms"
curl -s --max-time 120 -o /dev/null -w '%{http_code}' -x "$proxy" "http://127.0.0.1:8081/" > "$WORK/gwlan.code" 2> /dev/null || true
if curl -s --max-time 5 -o /dev/null "http://192.0.2.80:8081/inet.txt" 2> /dev/null; then
    echo reachable > "$WORK/gwdirect"
else
    echo unreachable > "$WORK/gwdirect"
fi
# The web interface: choose B as the gateway there, then fetch through the
# local proxy on this computer, which forwards to B over the radio.
ui="http://127.0.0.1:7373"
curl -s --max-time 10 "$ui/api/status" > "$WORK/ui-before.json" 2> /dev/null || status=1
curl -s --max-time 10 -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
    -d "{\"mode\":\"station\",\"call\":\"$PEER_CALL\"}" "$ui/api/gateway" > "$WORK/ui-choose.code" 2> /dev/null || true
start=$(date +%s%N)
curl -sS --max-time 180 -x http://127.0.0.1:7374 "http://192.0.2.80:8081/inet.txt" > "$WORK/local.out" 2> "$WORK/local.err" || status=1
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/local.ms"
start=$(date +%s%N)
curl -sS --max-time 180 -p -x http://127.0.0.1:7374 "http://192.0.2.80:8081/inet.txt" > "$WORK/localc.out" 2> "$WORK/localc.err" || status=1
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/localc.ms"
curl -s --max-time 10 "$ui/api/status" > "$WORK/ui-after.json" 2> /dev/null || status=1
# B's welcome page over the radio: directly, and by name through the local proxy.
curl -s --compressed --max-time 120 "http://$PEER_IP/" > "$WORK/welcome.out" 2> /dev/null || status=1
curl -s --compressed --max-time 120 -x http://127.0.0.1:7374 "http://testb-1.axl/" > "$WORK/welcome-name.out" 2> /dev/null || status=1
# B's firewall: a port B has not opened is refused at once (a TCP reset).
start=$(date +%s%N)
code=0
curl -s --max-time 60 -o /dev/null "http://$PEER_IP:9999/" 2> /dev/null || code=$?
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/fw.ms"
case $code in
    0) echo open > "$WORK/fw.result" ;;
    7) echo refused > "$WORK/fw.result" ;;   # could not connect: the reset came back
    *) echo "blocked, curl $code" > "$WORK/fw.result" ;;
esac
# B's port forwarding: 8022 reaches B's local server, 8023 is for TESTC only.
start=$(date +%s%N)
curl -sS --max-time 120 "http://$PEER_IP:8022/lan.txt" > "$WORK/fwd.out" 2> "$WORK/fwd.err" || status=1
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/fwd.ms"
code=0
curl -s --max-time 60 -o /dev/null "http://$PEER_IP:8023/lan.txt" 2> /dev/null || code=$?
echo $code > "$WORK/fwd-other.code"
# Chat: a message to B, acknowledged; B answers; a message to CQ. No IP.
chat() { curl -s --max-time 10 -X POST -H 'Content-Type: application/json' -d "$2" "$ui/api/chat/$1"; }
start=$(date +%s%N)
chat send "{\"to\":\"$PEER_CALL\",\"text\":\"Hello from A, $NONCE_ENV\"}" > /dev/null 2>&1 || status=1
for _ in $(seq 600); do
    curl -s --max-time 5 "$ui/api/chat/messages?peer=$PEER_CALL" > "$WORK/a-chat.json" 2> /dev/null || true
    grep -q '"state":"delivered"' "$WORK/a-chat.json" && break
    sleep 0.1
done
echo $(( ($(date +%s%N) - start) / 1000000 )) > "$WORK/chat.ms"
touch "$WORK/a.chat"
for _ in $(seq 600); do
    curl -s --max-time 5 "$ui/api/chat/messages?peer=$PEER_CALL" > "$WORK/a-chat.json" 2> /dev/null || true
    grep -q 'welcome to the lab' "$WORK/a-chat.json" && break
    sleep 0.1
done
chat send '{"to":"CQ","text":"CQ from the lab: anyone around?"}' > /dev/null 2>&1 || status=1
sleep 6
shoot client-overview / 1560
shoot client-settings "#settings" 1250
shoot welcome "http://$PEER_IP/" 640
shoot client-chat "#chat/$PEER_CALL" 860
touch "$WORK/b.shoot"
for _ in $(seq 1200); do [ -e "$WORK/b.shot" ] && break; sleep 0.1; done
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
secs() { echo "$(( $(cat "$1") / 1000 )).$(( $(cat "$1") % 1000 / 100 )) s"; }
want="from the internet $NONCE"
if grep -qx "$want" "$WORK/gw.out" 2>/dev/null && grep -qx "$want" "$WORK/gwc.out" 2>/dev/null &&
    [[ "$(cat "$WORK/gwlan.code" 2>/dev/null)" == 403 && "$(cat "$WORK/gwdirect" 2>/dev/null)" == unreachable ]] &&
    grep -q 'msg=gateway client=TESTA-1 ' "$WORK/node-b.log"; then
    echo "gateway: ok (proxy $(secs "$WORK/gw.ms"), CONNECT $(secs "$WORK/gwc.ms"); own network refused; no direct route)"
else
    echo "gateway: FAILED (proxy: $(cat "$WORK/gw.err" 2>/dev/null) CONNECT: $(cat "$WORK/gwc.err" 2>/dev/null)" \
        "own network: $(cat "$WORK/gwlan.code" 2>/dev/null) direct: $(cat "$WORK/gwdirect" 2>/dev/null))"
    status=1
fi
if python3 -I - "$WORK" "$want" <<'PY'
import json, os, sys
work, want = sys.argv[1], sys.argv[2]
def read(name):
    with open(os.path.join(work, name)) as f:
        return f.read()
before, after = json.loads(read("ui-before.json")), json.loads(read("ui-after.json"))
listed = [s for s in before.get("stations") or [] if s["call"] == "TESTB-1"]
ok = (listed and "INET" in (listed[0].get("tags") or []) and listed[0].get("proxy") == "10.73.0.2:3128"
      and read("ui-choose.code") == "204"
      and read("local.out").strip() == want and read("localc.out").strip() == want
      and after["internet"].get("using") == "TESTB-1")
raise SystemExit(0 if ok else 1)
PY
then
    echo "web UI: ok (B listed as a gateway, chosen there; local proxy $(secs "$WORK/local.ms"), CONNECT $(secs "$WORK/localc.ms"))"
else
    echo "web UI: FAILED (choose: $(cat "$WORK/ui-choose.code" 2>/dev/null) local: $(cat "$WORK/local.err" 2>/dev/null)" \
        "CONNECT: $(cat "$WORK/localc.err" 2>/dev/null))"
    status=1
fi
if python3 -I - "$WORK" <<'PY'
import json, os, sys
work = sys.argv[1]
def read(name):
    try:
        with open(os.path.join(work, name)) as f:
            return f.read()
    except OSError:
        return ""
pages = read("welcome.out"), read("welcome-name.out")
b = json.loads(read("b-status.json") or "{}")
fw = b.get("firewall", {})
blocked = [r for r in fw.get("recent") or [] if r.get("call") == "TESTA-1" and r.get("port") == 9999]
ok = (all("TESTB-1" in p and "Lab station B" in p and "8080/tcp" in p for p in pages)
      and read("fw.result").strip() == "refused" and fw.get("mode") == "protect" and blocked
      and fw.get("answered", 0) >= 1)
raise SystemExit(0 if ok else 1)
PY
then
    echo "welcome: ok (B's page over the radio with its open ports, also as testb-1.axl);" \
        "firewall: ok (TESTA-1 → tcp 9999 refused after $(secs "$WORK/fw.ms"))"
else
    echo "welcome/firewall: FAILED (page $(wc -c < "$WORK/welcome.out" 2>/dev/null) bytes, by name" \
        "$(wc -c < "$WORK/welcome-name.out" 2>/dev/null) bytes, port 9999: $(cat "$WORK/fw.result" 2>/dev/null))"
    status=1
fi
if grep -qx "from B's own network $NONCE" "$WORK/fwd.out" 2>/dev/null && [[ "$(cat "$WORK/fwd-other.code")" == 7 ]]; then
    echo "forward: ok (B's port 8022 → its local server in $(secs "$WORK/fwd.ms"); 8023, for TESTC only, refused)"
else
    echo "forward: FAILED ($(cat "$WORK/fwd.err" 2>/dev/null); 8023: curl $(cat "$WORK/fwd-other.code" 2>/dev/null))"
    status=1
fi
if python3 -I - "$WORK" "$NONCE" <<'PY'
import json, os, sys
work, nonce = sys.argv[1], sys.argv[2]
def load(name):
    try:
        with open(os.path.join(work, name)) as f:
            return json.load(f)
    except (OSError, ValueError):
        return []
a, b, cq = load("a-chat.json"), load("b-chat.json"), load("b-cq.json")
mine = [m for m in a if m.get("out") and nonce in m["text"]]
answer = [m for m in a if not m.get("out") and "welcome to the lab" in m["text"]]
got = [m for m in b if not m.get("out") and nonce in m["text"]]
room = [m for m in cq if m.get("from") == "TESTA-1"]
ok = (mine and mine[0]["state"] == "delivered" and mine[0]["tries"] == 1 and len(got) == 1 and answer
      and len(room) == 1)
if not ok:
    print(f"A: {a}\nB: {b}\nCQ at B: {cq}")
raise SystemExit(0 if ok else 1)
PY
then
    echo "chat:  ok (A → B delivered and acknowledged in $(secs "$WORK/chat.ms") at the first try; B's answer arrived;" \
        "A's CQ in B's room once)"
else
    echo "chat:  FAILED"
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

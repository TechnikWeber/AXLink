#!/usr/bin/env bash
# Run in a normal user terminal, without sudo.
set -euo pipefail
AXLINK_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$AXLINK_ROOT"

for tool in git curl python3; do
    command -v "$tool" >/dev/null || { echo "Missing: $tool" >&2; exit 2; }
done

# Check before changing anything; do not work around sandbox restrictions.
python3 - <<'PY'
import socket
import sys
try:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM):
        pass
    for host in ('github.com', 'dot.net', 'api.nuget.org'):
        socket.getaddrinfo(host, 443)
        print(f'DNS OK: {host}')
except OSError as exc:
    print(f'Network check failed: {exc}', file=sys.stderr)
    print('Start it in a normal host terminal; on EPERM check the execution permissions first.', file=sys.stderr)
    sys.exit(2)
PY

# Re-initialising keeps the existing history; no commits or pushes.
git init
git status --short

AXLINK_SDK="$AXLINK_ROOT/.tools/dotnet"
AXLINK_VERSION="$(python3 -c 'import json; print(json.load(open("artifacts/m0/sources/pdn-net/global.json"))["sdk"]["version"])')"
mkdir -p "$AXLINK_ROOT/.tools"
if [[ ! -x "$AXLINK_SDK/dotnet" ]] || ! "$AXLINK_SDK/dotnet" --list-sdks | awk '{print $1}' | grep -Fxq "$AXLINK_VERSION"; then
    curl --fail --show-error --silent --location --connect-timeout 15 --max-time 120 \
        https://dot.net/v1/dotnet-install.sh -o "$AXLINK_ROOT/.tools/dotnet-install.sh"
    bash "$AXLINK_ROOT/.tools/dotnet-install.sh" --version "$AXLINK_VERSION" \
        --install-dir "$AXLINK_SDK" --no-path
fi
export DOTNET_ROOT="$AXLINK_SDK"
export PATH="$AXLINK_SDK:$PATH"
dotnet --info
python3 scripts/audit-upstream.py --run
echo 'Upstream audit finished. Checking the receive-filter patch.'
python3 scripts/test-destination-filter.py --patched
echo 'Reproduce F1 if needed: python3 scripts/test-destination-filter.py (expects exit 1).'

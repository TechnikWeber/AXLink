#!/usr/bin/env bash
# Im normalen Benutzerterminal ausführen, ohne sudo.
set -euo pipefail
AXLINK_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$AXLINK_ROOT"

for tool in git curl python3; do
    command -v "$tool" >/dev/null || { echo "Fehlt: $tool" >&2; exit 2; }
done

# Prüfen vor jeder Änderung: keine Sandbox-Sperren umgehen.
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
    print(f'Netzwerkprüfung fehlgeschlagen: {exc}', file=sys.stderr)
    print('Im normalen Host-Terminal starten; bei EPERM zuerst die Ausführungsrechte prüfen.', file=sys.stderr)
    sys.exit(2)
PY

# Reinitialisierung erhält vorhandene Historie; keine Commits oder Pushes.
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
echo 'Upstream-Audit beendet. Prüfe den Empfangsfilter-Patch.'
python3 scripts/test-destination-filter.py --patched
echo 'F1 bei Bedarf reproduzieren: python3 scripts/test-destination-filter.py (erwartet Exit 1).'

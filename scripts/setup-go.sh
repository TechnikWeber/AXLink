#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Installs the Go toolchain from go.mod into .tools/go (no system changes)
# and checks the download against the SHA-256 that go.dev publishes.
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="go$(awk '/^go /{print $2}' "$ROOT/go.mod")"
case "$(uname -s)/$(uname -m)" in
    Linux/x86_64) PLATFORM=linux-amd64 ;;
    Linux/aarch64) PLATFORM=linux-arm64 ;;
    Linux/armv6l|Linux/armv7l) PLATFORM=linux-armv6l ;;
    Darwin/arm64) PLATFORM=darwin-arm64 ;;
    Darwin/x86_64) PLATFORM=darwin-amd64 ;;
    *) echo "Platform $(uname -s)/$(uname -m) not covered; install Go from https://go.dev/dl/" >&2; exit 2 ;;
esac
if [[ -x "$ROOT/.tools/go/bin/go" ]] && [[ "$("$ROOT/.tools/go/bin/go" env GOVERSION)" == "$VERSION" ]]; then
    echo "$VERSION is already installed in .tools/go."
    exit 0
fi
FILE="$VERSION.$PLATFORM.tar.gz"
SUM="$(curl -fsS --max-time 30 'https://go.dev/dl/?mode=json&include=all' | python3 -I -c '
import json, sys
name = sys.argv[1]
for release in json.load(sys.stdin):
    for f in release["files"]:
        if f["filename"] == name:
            print(f["sha256"])
            raise SystemExit
raise SystemExit("not listed on go.dev: " + name)' "$FILE")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
curl -fsSL --max-time 600 -o "$TMP/$FILE" "https://go.dev/dl/$FILE"
if command -v sha256sum >/dev/null; then
    echo "$SUM  $TMP/$FILE" | sha256sum -c -
else
    echo "$SUM  $TMP/$FILE" | shasum -a 256 -c -   # macOS
fi
mkdir -p "$ROOT/.tools"
rm -rf "$ROOT/.tools/go"
tar -C "$ROOT/.tools" -xzf "$TMP/$FILE"
"$ROOT/.tools/go/bin/go" version
echo 'Use: export PATH="$PWD/.tools/go/bin:$PATH"'

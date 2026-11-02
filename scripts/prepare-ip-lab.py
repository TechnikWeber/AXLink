#!/usr/bin/env python3
"""Build the patched pdn-net and upstream-based trace reader for the isolated lab."""
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'artifacts/m1'
SDK = ROOT / '.tools/dotnet'
ENV = {**os.environ, 'DOTNET_ROOT': str(SDK), 'PATH': str(SDK) + os.pathsep + os.environ['PATH']}


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    lock = json.loads((ROOT / 'upstream.lock.json').read_text())
    for name, repo in lock['repositories'].items():
        checkout = ROOT / 'upstream' / name
        head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=checkout, text=True).strip()
        dirty = subprocess.check_output(['git', 'status', '--porcelain'], cwd=checkout)
        if head != repo['commit'] or dirty.strip():
            raise RuntimeError(f'{name}: a clean checkout at the pinned commit is required')
    if not (ROOT / 'upstream/packet.net/src/Packet.Node/bin/Release/net10.0/packetnet.dll').is_file():
        raise RuntimeError('Run python3 scripts/audit-upstream.py --run first')
    source = ROOT / 'upstream/pdn-net'
    archive = subprocess.check_output(['git', 'archive', 'HEAD'], cwd=source)
    with tempfile.TemporaryDirectory(prefix='axlink-ip-build-') as temp:
        with tarfile.open(fileobj=io.BytesIO(archive)) as bundle:
            bundle.extractall(temp, filter='data')
        subprocess.run(['git', 'apply', str(ROOT / 'patches/pdn-net-destination-filter.patch')],
                       cwd=temp, check=True)
        subprocess.run(['dotnet', 'publish', 'src/pdn-net/pdn-net.csproj', '-c', 'Release',
                        '-o', str(OUT / 'pdn-net')], cwd=temp, env=ENV, check=True)
    subprocess.run(['dotnet', 'publish', 'tools/TraceCheck/TraceCheck.csproj', '-c', 'Release',
                    '-o', str(OUT / 'trace-check')], cwd=ROOT, env=ENV, check=True)


if __name__ == '__main__':
    main()

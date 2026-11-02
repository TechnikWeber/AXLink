#!/usr/bin/env python3
"""Run the F1 regression in an isolated copy of the pinned pdn-net checkout."""
import io
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'artifacts/m0/destination-filter'
if (ROOT / '.tools/dotnet/dotnet').is_file():
    os.environ['DOTNET_ROOT'] = str(ROOT / '.tools/dotnet')
    os.environ['PATH'] = str(ROOT / '.tools/dotnet') + os.pathsep + os.environ['PATH']


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--patched', action='store_true', help='Apply the F1 patch and run all pdn-net tests')
    args = parser.parse_args()
    global OUT
    OUT = OUT / ('patched' if args.patched else 'baseline')
    OUT.mkdir(parents=True, exist_ok=True)
    report = {'status': 'blocked', 'commands': []}
    exit_code = 2

    def run(argv, cwd):
        result = subprocess.run(argv, cwd=cwd, capture_output=True, text=True, timeout=600)
        report['commands'].append({'argv': argv, 'exit': result.returncode,
                                   'stdout': result.stdout, 'stderr': result.stderr})
        if result.returncode:
            raise RuntimeError(f'Exit {result.returncode}: {argv}')
        return result.stdout.strip()

    try:
        for tool in ('git', 'dotnet'):
            if not shutil.which(tool):
                raise FileNotFoundError(f'{tool} fehlt')
        repo = json.loads((ROOT / 'upstream.lock.json').read_text())['repositories']['pdn-net']
        source = ROOT / 'upstream/pdn-net'
        if not source.is_dir():
            raise FileNotFoundError('Gepinnter Checkout upstream/pdn-net fehlt')
        if run(['git', 'rev-parse', 'HEAD'], source) != repo['commit']:
            raise RuntimeError('Checkout stimmt nicht mit upstream.lock.json überein')
        if run(['git', 'status', '--porcelain'], source):
            raise RuntimeError('Checkout enthält lokale Änderungen')
        # Keep the baseline pristine for audit-upstream.py. Export tracked files only.
        archive = subprocess.run(['git', 'archive', repo['commit']], cwd=source,
                                 capture_output=True, check=True, timeout=60).stdout
        with tempfile.TemporaryDirectory(prefix='axlink-f1-') as directory:
            work = Path(directory)
            with tarfile.open(fileobj=io.BytesIO(archive)) as bundle:
                bundle.extractall(work, filter='data')
            test_dir = work / 'tests/Packet.Net.Tests'
            shutil.copy2(ROOT / 'tests/upstream/AXLinkDestinationFilterTests.cs', test_dir)
            if args.patched:
                run(['git', 'apply', '--check', str(ROOT / 'patches/pdn-net-destination-filter.patch')], work)
                run(['git', 'apply', str(ROOT / 'patches/pdn-net-destination-filter.patch')], work)
            run(['dotnet', 'test', str(test_dir / 'Packet.Net.Tests.csproj'),
                 '--configuration', 'Release', '--filter',
                 ('Category!=HardwareLoop&Category!=Interop' if args.patched else
                  'FullyQualifiedName~AXLinkDestinationFilterTests'), '--logger', 'trx',
                 '--results-directory', str(OUT)], work)
        report['status'] = 'passed'
        exit_code = 0
    except FileNotFoundError as exc:
        report['reason'] = str(exc)
    except (RuntimeError, OSError, subprocess.SubprocessError, tarfile.TarError) as exc:
        report['status'] = 'failed'
        report['reason'] = str(exc)
        exit_code = 1
    (OUT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
    print(f"{report['status']}: {report.get('reason', 'siehe TRX-Ergebnisse')}")
    return exit_code


if __name__ == '__main__':
    sys.exit(main())

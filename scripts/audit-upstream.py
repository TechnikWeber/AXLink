#!/usr/bin/env python3
"""AXLink M0: prerequisites/evidence check; --run clones pinned sources and builds/tests."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'artifacts/m0'

# Prefer the project SDK installed by setup-host.sh; no shell-profile changes.
if (ROOT / '.tools/dotnet/dotnet').is_file():
    os.environ['DOTNET_ROOT'] = str(ROOT / '.tools/dotnet')
    os.environ['PATH'] = str(ROOT / '.tools/dotnet') + os.pathsep + os.environ['PATH']


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', action='store_true', help='Fetch pinned upstreams and run .NET builds/tests')
    args = parser.parse_args()
    lock = json.loads((ROOT / 'upstream.lock.json').read_text())
    report = {'mode': 'run' if args.run else 'preflight', 'checks': [], 'commands': [],
              'test_filter': 'Category!=HardwareLoop&Category!=Interop',
              'test_locale': 'en_US.UTF-8'}
    for rel, expected in json.loads((OUT / 'source-sha256.json').read_text()).items():
        path = OUT / rel
        ok = path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest() == expected
        report['checks'].append({'source': rel, 'ok': ok})
    if not all(x['ok'] for x in report['checks']):
        report['status'] = 'failed-source-integrity'
    elif not shutil.which('dotnet') or not shutil.which('git'):
        report['status'] = 'blocked'
        report['reason'] = 'Required tools missing: ' + ', '.join(x for x in ('dotnet', 'git') if not shutil.which(x))
    elif not args.run:
        report['status'] = 'ready-for-build-attempt'
    else:
        report['status'] = 'passed'
        def run(command, cwd, label):
            print(f'Running: {label}', flush=True)
            result = subprocess.run(command, cwd=cwd, text=True, stdout=subprocess.PIPE,
                                    stderr=subprocess.STDOUT, timeout=1200,
                                    env={**os.environ, 'LANG': 'en_US.UTF-8', 'LC_ALL': 'en_US.UTF-8'})
            (OUT / (label + '.log')).write_text(result.stdout)
            report['commands'].append({'argv': command, 'cwd': str(cwd), 'exit': result.returncode, 'log': label + '.log'})
            print(f'Finished: {label} (exit {result.returncode})', flush=True)
            if result.returncode:
                raise RuntimeError(label)
            return result.stdout.strip()
        try:
            run(['dotnet', '--info'], ROOT, 'dotnet-info')
            for name, repo in lock['repositories'].items():
                dest = ROOT / 'upstream' / name
                dest.parent.mkdir(exist_ok=True)
                if not dest.exists():
                    run(['git', 'clone', '--no-checkout', repo['url'], str(dest)], ROOT, name + '-clone')
                    run(['git', 'checkout', '--detach', repo['commit']], dest, name + '-checkout')
                # Existing checkouts are never reset or silently advanced.
                head = run(['git', 'rev-parse', 'HEAD'], dest, name + '-head')
                dirty = run(['git', 'status', '--porcelain'], dest, name + '-status')
                if head != repo['commit'] or dirty:
                    raise RuntimeError(name + ': existing checkout differs from lock or is dirty')
                if (dest / '.gitmodules').is_file():
                    run(['git', 'submodule', 'update', '--init', '--recursive'], dest, name + '-submodules')
                    run(['git', 'submodule', 'status', '--recursive'], dest, name + '-submodule-status')
                run(['dotnet', 'build', repo['solution'], '--configuration', 'Release'], dest, name + '-build')
                run(['dotnet', 'test', repo['solution'], '--configuration', 'Release', '--no-build',
                     '--filter', report['test_filter'], '--logger', 'trx',
                     '--results-directory', str(OUT / 'test-results' / 'unit' / name)], dest, name + '-test')
        except (RuntimeError, OSError, subprocess.TimeoutExpired) as exc:
            report['status'] = 'failed'
            report['reason'] = str(exc)
    (OUT / 'preflight.json').write_text(json.dumps(report, indent=2) + '\n')
    print(f"{report['status']}: {report.get('reason', 'see artifacts/m0/preflight.json')}")
    print(f"Source integrity: {sum(x['ok'] for x in report['checks'])}/{len(report['checks'])}")
    return 2 if report['status'] == 'blocked' else 1 if report['status'].startswith('failed') else 0


if __name__ == '__main__':
    sys.exit(main())

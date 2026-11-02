#!/usr/bin/env python3
"""Run only the audited KISS simulator smoke test; a skipped test is a failure."""
import json
import os
from pathlib import Path
import subprocess
import urllib.request
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'artifacts/m0/kiss-simulator'


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    report = {'status': 'failed'}
    try:
        repo = ROOT / 'upstream/packet.net'
        pin = json.loads((ROOT / 'upstream.lock.json').read_text())['repositories']['packet.net']['commit']
        head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip()
        if head != pin or subprocess.check_output(['git', 'status', '--porcelain'], cwd=repo).strip():
            raise RuntimeError('Packet.NET-Checkout weicht vom sauberen Pin ab')
        with urllib.request.urlopen('http://127.0.0.1:18080/healthz', timeout=3) as response:
            if response.read().strip() != b'ok':
                raise RuntimeError('Simulator nicht bereit')
        sdk = ROOT / '.tools/dotnet'
        env = {**os.environ, 'DOTNET_ROOT': str(sdk), 'PATH': str(sdk) + os.pathsep + os.environ['PATH']}
        trx = OUT / 'kiss.trx'
        trx.unlink(missing_ok=True)
        command = [str(sdk / 'dotnet'), 'test', 'tests/Packet.Interop.Tests/Packet.Interop.Tests.csproj',
                   '--configuration', 'Release', '--no-build', '--filter',
                   'FullyQualifiedName=Packet.Interop.Tests.Netsim.NetsimKissTcpInterop.UI_Frame_Sent_On_Node_A_Is_Received_On_Node_B',
                   '--logger', 'trx;LogFileName=kiss.trx', '--results-directory', str(OUT)]
        report['command'] = command
        result = subprocess.run(command, cwd=repo, env=env, capture_output=True, text=True, timeout=120)
        (OUT / 'test.log').write_text(result.stdout + result.stderr)
        report['exit'] = result.returncode
        results = ET.parse(trx).findall('.//{http://microsoft.com/schemas/VisualStudio/TeamTest/2010}UnitTestResult')
        report['outcomes'] = [r.get('outcome') for r in results]
        if result.returncode or report['outcomes'] != ['Passed']:
            raise RuntimeError('Genau ein bestandener Test erforderlich; Skip zählt nicht')
        report['status'] = 'passed'
    except (OSError, RuntimeError, subprocess.SubprocessError, ET.ParseError) as exc:
        report['reason'] = str(exc)
    (OUT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
    print(f"{report['status']}: {report.get('reason', 'KISS UI-Frame A → B; noch kein IP-Test')}")
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

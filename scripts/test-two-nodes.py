#!/usr/bin/env python3
"""Real UDP/HTTP through two TUN/pdn-net/pdn stacks and the audio KISS simulator."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
SDK = ROOT / '.tools/dotnet'
ENV = {**os.environ, 'DOTNET_ROOT': str(SDK), 'PATH': str(SDK) + os.pathsep + os.environ['PATH']}
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'simulator/compose.ip.yaml')]


def verify_traces(exchanges, captures):
    """Require byte-identical UI/PID-0xCC IP packets at TX/RX for every nonce."""
    for exchange in exchanges:
        origin = exchange['origin']
        peer = 'b' if origin == 'a' else 'a'
        needle = exchange['nonce'].encode().hex().upper()
        protocol = 17 if exchange['protocol'] == 'udp' else 6
        for sender, receiver in ((origin, peer), (peer, origin)):
            source = 'TESTA-1' if sender == 'a' else 'TESTB-1'
            destination = 'TESTA-1' if receiver == 'a' else 'TESTB-1'
            received = {f['ipHex'] for f in captures[receiver] if f['direction'] == 'rx'
                        and f['source'] == source and f['destination'] == destination
                        and f['pid'] == 204 and f['protocol'] == protocol}
            matches = [f for f in captures[sender] if f['direction'] == 'tx'
                       and f['protocol'] == protocol and needle in f['ipHex']
                       and f['ipHex'] in received and f['pid'] == 204
                       and f['source'] == source and f['destination'] == destination]
            if not matches:
                raise RuntimeError(f'Missing PID 0xCC trace: {exchange}, {sender}->{receiver}')
        exchange['trace_verified'] = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--no-build', action='store_true', help='Use already prepared lab binaries/image')
    args = parser.parse_args()
    stamp = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(2)
    out = ROOT / 'artifacts/m1/runs' / stamp
    out.mkdir(parents=True)
    report = {'status': 'failed', 'run': str(out.relative_to(ROOT)), 'commands': [], 'exchanges': [],
              'negative_controls': []}
    owns_stack = False
    paused = False

    def run(argv, label, check=True, timeout=120):
        result = subprocess.run(argv, cwd=ROOT, env=ENV, capture_output=True, text=True, timeout=timeout)
        path = out / f'{len(report["commands"]):03}-{label}.log'
        path.write_text(result.stdout + result.stderr)
        report['commands'].append({'argv': argv, 'exit': result.returncode, 'log': path.name})
        if check and result.returncode:
            raise RuntimeError(f'{label}: exit {result.returncode}; see {path}')
        return result

    def node(station, argv, label, **kwargs):
        return run([*COMPOSE, 'exec', '-T', station, *argv], label, **kwargs)

    def snapshot(phase):
        for station, peer in (('a', '10.73.0.2'), ('b', '10.73.0.1')):
            for name, argv in (('addresses', ['-j', 'address']), ('routes', ['-j', 'route']),
                               ('counters', ['-j', '-s', 'link', 'show', 'radio0'])):
                result = node(station, ['ip', *argv], f'{phase}-{station}-{name}')
                (out / f'{phase}-{station}-{name}.json').write_text(result.stdout)
            route = json.loads(node(station, ['ip', '-j', 'route', 'get', peer],
                                    f'{phase}-{station}-peerroute').stdout)
            if len(route) != 1 or route[0]['dev'] != 'radio0':
                raise RuntimeError('The peer IP has a route that bypasses the TUN')

    def probe(station, protocol, phase, expect_timeout=False, local=False):
        target = {'a': '10.73.0.2', 'b': '10.73.0.1'}[station]
        if local:
            target = {'a': '10.73.0.1', 'b': '10.73.0.2'}[station]
        nonce = secrets.token_hex(8)
        result = node(station, ['python3', '/lab/node.py', 'probe', protocol, target, nonce,
                               '5' if expect_timeout else ('30' if protocol == 'udp' else '60')],
                      f'{phase}-{station}-{protocol}', check=False, timeout=75)
        payload = json.loads(result.stdout)
        if expect_timeout:
            if result.returncode != 2 or payload.get('error') != 'TimeoutError':
                raise RuntimeError(f'Channel interruption not demonstrated: {payload}')
            report['negative_controls'].append({'protocol': protocol, 'nonce': nonce, **payload})
        else:
            if result.returncode or payload.get('reply') != 'AXLink-' + nonce:
                raise RuntimeError(f'{phase}/{protocol}: {payload}')
            if not local:
                report['exchanges'].append({'origin': station, 'phase': phase, **payload})
        print(f'{phase}: {station} {protocol} OK', flush=True)

    try:
        existing = run([*COMPOSE, 'ps', '-aq'], 'existing').stdout.strip()
        if existing:
            raise RuntimeError('The AXLink IP lab is already running; not replaced')
        if not args.no_build:
            run([sys.executable, str(ROOT / 'scripts/prepare-ip-lab.py')], 'prepare', timeout=600)
            run(['docker', 'buildx', 'build', '--load', '--allow=network.host', '--network=host',
                 '-f', 'simulator/node.Dockerfile', '-t', 'axlink-ip-lab-node:local', 'simulator'],
                'image-build', timeout=600)
        owns_stack = True
        run([*COMPOSE, 'up', '-d', '--no-build'], 'start', timeout=180)
        for station in ('a', 'b'):
            for attempt in range(90):
                ready = node(station, ['test', '-f', '/tmp/ready'], f'ready-{station}', check=False)
                if ready.returncode == 0:
                    break
                time.sleep(1)
            else:
                raise RuntimeError(f'Station {station} not ready')
        snapshot('before')
        for station in ('a', 'b'):
            for protocol in ('udp', 'http'):
                probe(station, protocol, 'connected')
        run([*COMPOSE, 'pause', 'channel'], 'pause-channel')
        paused = True
        for protocol in ('udp', 'http'):
            probe('b', protocol, 'server-alive', local=True)
            probe('a', protocol, 'channel-paused', expect_timeout=True)
        run([*COMPOSE, 'unpause', 'channel'], 'resume-channel')
        paused = False
        time.sleep(3)  # Let buffered old frames drain; recovery always uses fresh nonces.
        for protocol in ('udp', 'http'):
            probe('a', protocol, 'recovered')
        snapshot('after')
        time.sleep(2)  # Traffic-store batch flush, then export through the upstream CLI.
        captures = {}
        for station in ('a', 'b'):
            raw = node(station, ['dotnet', '/opt/pdn/packetnet.dll', 'traffic', 'export',
                                '--traffic-db', '/tmp/traffic.db'], f'trace-{station}').stdout
            capture = out / f'{station}-ax25.jsonl'
            capture.write_text(raw)
            decoded = run(['dotnet', str(ROOT / 'artifacts/m1/trace-check/TraceCheck.dll'), str(capture)],
                          f'decode-{station}').stdout
            (out / f'{station}-decoded.jsonl').write_text(decoded)
            captures[station] = [json.loads(line) for line in decoded.splitlines() if line]
        # Every successful application nonce must cross the channel in BOTH directions,
        # with identical complete IP bytes at sender TX and peer RX, UI/PID 0xCC.
        verify_traces(report['exchanges'], captures)
        report['status'] = 'passed'
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as exc:
        report['reason'] = str(exc)
    finally:
        if owns_stack:
            try:
                if paused:
                    run([*COMPOSE, 'unpause', 'channel'], 'cleanup-unpause', check=False)
                logs = run([*COMPOSE, 'logs', '--no-color'], 'container-logs', check=False)
                (out / 'containers.log').write_text(logs.stdout + logs.stderr)
                run([*COMPOSE, 'down', '--timeout', '10'], 'cleanup')
            except (OSError, RuntimeError, subprocess.SubprocessError) as exc:
                report['status'] = 'failed'
                report['cleanup_error'] = str(exc)
        (out / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
        (ROOT / 'artifacts/m1/latest.json').write_text(json.dumps(report, indent=2) + '\n')
    print(f"{report['status']}: {report.get('reason', out)}")
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    raise SystemExit(main())

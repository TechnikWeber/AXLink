"""Exercise audit failure handling and the no-hardware default without network/SDK."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class AuditRunnerTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / 'scripts').mkdir()
        shutil.copy2(ROOT / 'scripts/audit-upstream.py', self.root / 'scripts')
        self.out = self.root / 'artifacts/m0'
        self.out.mkdir(parents=True)
        (self.out / 'source').write_bytes(b'audited bytes')
        (self.out / 'source-sha256.json').write_text(json.dumps(
            {'source': hashlib.sha256(b'audited bytes').hexdigest()}))
        (self.root / 'upstream/example').mkdir(parents=True)
        (self.root / 'upstream.lock.json').write_text(json.dumps({'repositories': {
            'example': {'commit': 'pinned', 'url': 'unused', 'solution': 'example.slnx'}}}))
        tools = self.root / 'tools'
        tools.mkdir()
        fake = '''import os, sys
tool = os.path.basename(sys.argv[0])
args = sys.argv[1:]
if tool == 'git' and args == ['rev-parse', 'HEAD']:
    print(os.environ.get('FAKE_HEAD', 'pinned'))
if tool == 'git' and args == ['status', '--porcelain']:
    print(os.environ.get('FAKE_DIRTY', ''), end='')
if tool == 'dotnet' and args[0] == 'test':
    # Fail closed if the audit ever enables hardware or external interop implicitly.
    if '--filter' not in args:
        sys.exit(91)
    exclusions = set(args[args.index('--filter') + 1].split('&'))
    if not {'Category!=HardwareLoop', 'Category!=Interop'} <= exclusions:
        sys.exit(92)
    sys.exit(int(os.environ.get('FAKE_TEST_EXIT', '0')))
'''
        for tool in ('git', 'dotnet'):
            path = tools / tool
            path.write_text(f'#!{sys.executable}\n' + fake)
            path.chmod(0o755)
        self.env = {**os.environ, 'PATH': str(tools)}

    def run_audit(self, **env):
        result = subprocess.run([sys.executable, str(self.root / 'scripts/audit-upstream.py'), '--run'],
                                env={**self.env, **env}, capture_output=True, text=True, timeout=10)
        return result.returncode, json.loads((self.out / 'preflight.json').read_text())

    def test_default_never_enables_hardware_or_external_interop(self):
        code, report = self.run_audit()
        self.assertEqual(0, code)
        self.assertTrue(any(c['argv'][:2] == ['dotnet', 'test'] for c in report['commands']))

    def test_dirty_or_wrong_commit_never_builds(self):
        for condition in ({'FAKE_DIRTY': ' M local.cs'}, {'FAKE_HEAD': 'different'}):
            with self.subTest(condition=condition):
                code, report = self.run_audit(**condition)
                self.assertEqual(1, code)
                self.assertFalse(any(c['argv'][:2] == ['dotnet', 'build'] for c in report['commands']))

    def test_test_failure_is_not_reported_as_success(self):
        code, report = self.run_audit(FAKE_TEST_EXIT='1')
        self.assertEqual(1, code)
        self.assertEqual('failed', report['status'])

    def test_modified_audit_sources_stop_before_any_command(self):
        (self.out / 'source').write_bytes(b'changed')
        code, report = self.run_audit()
        self.assertEqual(1, code)
        self.assertEqual('failed-source-integrity', report['status'])
        self.assertEqual([], report['commands'])


if __name__ == '__main__':
    unittest.main()

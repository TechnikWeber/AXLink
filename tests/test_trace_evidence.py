"""Reject incomplete/corrupted evidence rather than accepting an application-only demo."""
import copy
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('two_nodes', ROOT / 'scripts/test-two-nodes.py')
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class TraceEvidenceTests(unittest.TestCase):
    def setUp(self):
        evidence = ROOT / 'artifacts/m1/evidence'
        report = json.loads((evidence / 'result.json').read_text())
        self.exchanges = report['exchanges']
        self.captures = {station: [json.loads(line) for line in
                        (evidence / f'{station}-decoded.jsonl').read_text().splitlines()]
                         for station in ('a', 'b')}

    def test_real_capture_proves_every_request_and_response(self):
        runner.verify_traces(self.exchanges, self.captures)
        self.assertEqual(6, len(self.exchanges))
        self.assertTrue(all(e['trace_verified'] for e in self.exchanges))

    def test_missing_peer_reception_is_rejected(self):
        self.captures['b'] = [f for f in self.captures['b'] if f['direction'] != 'rx']
        with self.assertRaises(RuntimeError):
            runner.verify_traces(self.exchanges, self.captures)

    def test_missing_return_path_is_rejected(self):
        self.captures['a'] = [f for f in self.captures['a'] if f['direction'] != 'rx']
        with self.assertRaises(RuntimeError):
            runner.verify_traces(self.exchanges, self.captures)

    def test_changed_packet_wrong_pid_or_wrong_receiver_is_rejected(self):
        for field, value in (('ipHex', '00'), ('pid', 240), ('destination', 'TESTC-1')):
            with self.subTest(field=field):
                changed = copy.deepcopy(self.captures)
                for frame in changed['b']:
                    if frame['direction'] == 'rx':
                        frame[field] = value
                with self.assertRaises(RuntimeError):
                    runner.verify_traces(self.exchanges, changed)

    def test_old_capture_cannot_prove_a_new_nonce(self):
        self.exchanges[0]['nonce'] = 'not-in-the-capture'
        with self.assertRaises(RuntimeError):
            runner.verify_traces(self.exchanges, self.captures)


if __name__ == '__main__':
    unittest.main()

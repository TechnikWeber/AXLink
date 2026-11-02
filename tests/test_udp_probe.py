"""Ordinary UDP client retries loss within a deadline, keeping one exchange nonce."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import socket
import threading
import time
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('lab_node', ROOT / 'simulator/node.py')
lab = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lab)


class UdpProbeTests(unittest.TestCase):
    def test_dropped_first_request_is_retried_with_same_nonce(self):
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as server:
            server.bind(('127.0.0.1', 0))
            server.settimeout(1)
            received = []

            def echo_second():
                for _ in range(2):
                    data, sender = server.recvfrom(256)
                    received.append(data)
                server.sendto(data, sender)

            worker = threading.Thread(target=echo_second)
            worker.start()
            output = io.StringIO()
            try:
                with patch.object(lab, 'UDP_PORT', server.getsockname()[1]), \
                     patch.object(lab, 'UDP_RETRY_SECONDS', .05), contextlib.redirect_stdout(output):
                    lab.probe('udp', '127.0.0.1', 'loss-test', .8)
            finally:
                worker.join(timeout=2)
            self.assertFalse(worker.is_alive())
            self.assertEqual([b'AXLink-loss-test', b'AXLink-loss-test'], received)
            self.assertEqual(2, json.loads(output.getvalue())['attempts'])

    def test_total_loss_still_times_out(self):
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as server:
            server.bind(('127.0.0.1', 0))
            start = time.monotonic()
            with patch.object(lab, 'UDP_PORT', server.getsockname()[1]), \
                 patch.object(lab, 'UDP_RETRY_SECONDS', .03):
                with self.assertRaises(TimeoutError):
                    lab.probe('udp', '127.0.0.1', 'no-reply', .12)
            self.assertLess(time.monotonic() - start, 1)


if __name__ == '__main__':
    unittest.main()

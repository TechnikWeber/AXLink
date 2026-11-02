"""Isolated lab process supervisor and ordinary UDP/HTTP socket applications."""
import http.client
import json
import os
from pathlib import Path
import signal
import socket
import socketserver
import subprocess
import sys
import threading
import time

UDP_PORT = 7070
UDP_RETRY_SECONDS = 5


def probe(protocol, target, nonce, timeout):
    payload = ('AXLink-' + nonce).encode('ascii')
    started = time.monotonic()
    attempts = 1
    if protocol == 'udp':
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as client:
            client.connect((target, UDP_PORT))
            # UDP does not promise delivery. A bounded request/reply application
            # retries at most once per five seconds, with the SAME exchange nonce.
            deadline = started + timeout
            attempts = 0
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise TimeoutError('UDP budget exhausted')
                attempts += 1
                client.settimeout(min(UDP_RETRY_SECONDS, remaining))
                client.send(payload)
                try:
                    reply = client.recv(256)
                    break
                except TimeoutError:
                    if time.monotonic() >= deadline:
                        raise
    else:
        client = http.client.HTTPConnection(target, 8081, timeout=timeout)
        try:
            client.request('GET', '/' + nonce)
            response = client.getresponse()
            if response.status != 200:
                raise RuntimeError(f'HTTP {response.status}')
            reply = response.read()
        finally:
            client.close()
    if reply != payload:
        raise RuntimeError('Wrong reply payload')
    print(json.dumps({'protocol': protocol, 'nonce': nonce, 'reply': reply.decode(),
                      'attempts': attempts, 'seconds': round(time.monotonic() - started, 3)}))


class UdpEcho(socketserver.BaseRequestHandler):
    def handle(self):
        data, sock = self.request
        if len(data) <= 128:
            sock.sendto(data, self.client_address)


class HttpEcho(socketserver.StreamRequestHandler):
    def handle(self):
        self.connection.settimeout(10)
        request = self.rfile.readline(512).split()
        if len(request) != 3 or request[0] != b'GET':
            return
        # Read the complete small request before responding, so the kernel closes
        # gracefully instead of resetting a socket with unread data.
        for _ in range(20):
            if self.rfile.readline(512) in (b'\r\n', b'\n', b''):
                break
        body = b'AXLink-' + request[1].lstrip(b'/')[:64]
        self.wfile.write(b'HTTP/1.0 200 OK\r\nContent-Length: ' + str(len(body)).encode()
                         + b'\r\nConnection: close\r\n\r\n' + body)


def serve():
    station = os.environ['STATION']
    assert station in ('a', 'b')
    number, peer = (1, 2) if station == 'a' else (2, 1)
    call, peer_call = ('TESTA-1', 'TESTB-1') if number == 1 else ('TESTB-1', 'TESTA-1')
    ip, peer_ip = f'10.73.0.{number}', f'10.73.0.{peer}'
    Path('/tmp/pdn.yaml').write_text(f'''schemaVersion: 2
identity:
  callsign: {call}
ports:
  - id: sim
    transport:
      kind: kiss-tcp
      host: channel
      port: {8099 + number}
    kiss:
      txDelay: 30
      txTail: 5
management:
  telnet:
    enabled: false
  http:
    bind: 127.0.0.1
    port: 8080
  auth:
    enabled: false
rhp:
  enabled: true
  bind: 127.0.0.1
  port: 9000
netRom:
  enabled: false
beacon:
  enabled: false
traffic:
  enabled: true
  path: /tmp/traffic.db
''')
    Path('/tmp/pdn-net.json').write_text(json.dumps({
        'myCallsign': call, 'myIp': ip, 'rhpAddress': '127.0.0.1:9000',
        'tunName': 'radio0', 'mtu': 256,
        'routes': [{'ip': peer_ip + '/32', 'callsign': peer_call}]}))
    children = []
    stop = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stop.set())

    def wait_until(check):
        for _ in range(150):
            if stop.is_set() or any(p.poll() is not None for p in children):
                raise RuntimeError('Child process exited')
            try:
                if check():
                    return
            except OSError:
                pass
            time.sleep(.2)
        raise TimeoutError('Node start takes too long')

    def rhp_ready():
        with socket.create_connection(('127.0.0.1', 9000), timeout=.2):
            return True

    try:
        children.append(subprocess.Popen(['dotnet', '/opt/pdn/packetnet.dll',
                                          '--config', '/tmp/pdn.yaml', '--db', '/tmp/pdn.db']))
        wait_until(rhp_ready)
        children.append(subprocess.Popen(['dotnet', '/opt/pdn-net/pdn-net.dll',
                                          '--config', '/tmp/pdn-net.json']))
        wait_until(lambda: Path('/sys/class/net/radio0').exists())
        for command in (['addr', 'add', ip + '/32', 'dev', 'radio0'],
                        ['link', 'set', 'radio0', 'up', 'mtu', '256'],
                        ['route', 'add', peer_ip + '/32', 'dev', 'radio0']):
            subprocess.run(['ip', *command], check=True)
        udp = socketserver.ThreadingUDPServer((ip, 7070), UdpEcho)
        http = socketserver.ThreadingTCPServer((ip, 8081), HttpEcho)
        for server in (udp, http):
            server.daemon_threads = True
            threading.Thread(target=server.serve_forever, daemon=True).start()
        Path('/tmp/ready').write_text(ip)
        print(f'AXLink lab {station} ready: {ip}', flush=True)
        while not stop.wait(.5):
            if any(p.poll() is not None for p in children):
                raise RuntimeError('pdn/pdn-net exited unexpectedly')
    finally:
        for p in reversed(children):
            if p.poll() is None:
                p.terminate()
        for p in children:
            try:
                p.wait(timeout=5)
            except subprocess.TimeoutExpired:
                p.kill()
                p.wait()


if __name__ == '__main__':
    if sys.argv[1] == 'serve':
        serve()
    else:
        try:
            probe(sys.argv[2], sys.argv[3], sys.argv[4], float(sys.argv[5]))
        except (TimeoutError, ConnectionError, OSError, http.client.HTTPException) as exc:
            print(json.dumps({'error': type(exc).__name__, 'message': str(exc)}))
            sys.exit(2)

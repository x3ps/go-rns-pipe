#!/usr/bin/env python3
"""Tiny HTTP control server for killing/checking rns-udp-iface."""

import subprocess
from http.server import HTTPServer, BaseHTTPRequestHandler

IFACE_PATTERN = "rns-udp-iface"


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path == "/kill-iface":
            rc = subprocess.call(["pkill", "-f", IFACE_PATTERN])
            code = 200 if rc == 0 else 404
            self.send_response(code)
            self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

    def do_GET(self):
        if self.path == "/check-iface":
            rc = subprocess.call(["pgrep", "-f", IFACE_PATTERN])
            code = 200 if rc == 0 else 404
            self.send_response(code)
            self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, fmt, *args):
        pass  # silence logs


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 9000), Handler).serve_forever()

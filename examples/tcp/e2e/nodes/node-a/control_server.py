#!/usr/bin/env python3
"""Tiny HTTP control server for killing/checking rns-tcp-iface."""

import socket
import subprocess
from http.server import HTTPServer, BaseHTTPRequestHandler

IFACE_PATTERN = "rns-tcp-iface"


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
            # Use a TCP connect probe to :4244 instead of pgrep.
            # This is zombie-immune: a zombie process holds no open sockets,
            # so connect() fails immediately when the server is not running.
            try:
                with socket.create_connection(("localhost", 4244), timeout=2):
                    code = 200
            except OSError:
                code = 404
            self.send_response(code)
            self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, fmt, *args):
        pass  # silence logs


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 9000), Handler).serve_forever()

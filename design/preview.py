#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""Локальный предпросмотр прототипов редизайна.

    python3 design/preview.py [порт]

Открывает /design/prototypes/index.html на корне: так относительные пути
к ../tokens.css и ../../static/fonts/ остаются рабочими.
"""

import http.server
import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
START = "/design/prototypes/index.html"
REDIRECT_FROM = {"/", "/index.html", "/design", "/design/", "/prototypes"}


class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=ROOT, **kwargs)

    def do_GET(self):  # noqa: N802 (имя из базового класса)
        if self.path in REDIRECT_FROM:
            self.send_response(302)
            self.send_header("Location", START)
            self.end_headers()
            return
        super().do_GET()

    def end_headers(self):
        # Прототипы правятся часто — кэш только мешает.
        if self.path.endswith((".html", ".css", ".js")):
            self.send_header("Cache-Control", "no-store")
        super().end_headers()

    def log_message(self, fmt, *args):
        sys.stderr.write("  %s\n" % (fmt % args))


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8000
    with http.server.ThreadingHTTPServer(("0.0.0.0", port), Handler) as httpd:
        print(f"Прототипы редизайна: http://localhost:{port}{START}")
        print("Ctrl+C — остановить")
        httpd.serve_forever()


if __name__ == "__main__":
    main()

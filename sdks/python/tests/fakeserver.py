"""A local HTTP server standing in for the LevelUp API. Tests never talk to the real API.

Importing this module also puts ``sdks/python/src`` on ``sys.path`` so the tests run
without installing the package: ``python3 -m unittest discover sdks/python/tests``.
"""

from __future__ import annotations

import json
import os
import sys
import threading
import time
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Callable, Dict, List, Optional
from urllib.parse import parse_qs, urlsplit

_SRC = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "src"))
if _SRC not in sys.path:
    sys.path.insert(0, _SRC)


@dataclass
class Recorded:
    method: str
    path: str
    query: Dict[str, List[str]]
    headers: Dict[str, str]
    raw_body: bytes
    body: Any

    def q(self, name: str) -> Optional[str]:
        values = self.query.get(name)
        return values[0] if values else None


@dataclass
class Reply:
    status: int = 200
    body: Any = None
    headers: Dict[str, str] = field(default_factory=dict)
    drop: bool = False  # close the socket without answering
    delay: float = 0.0


Handler = Callable[[Recorded], Reply]


class FakeServer:
    def __init__(self) -> None:
        self.requests: List[Recorded] = []
        self.handler: Handler = lambda req: Reply(404, {"status": 404, "code": "no_handler"})
        outer = self

        class _Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *args: Any) -> None:  # silence
                pass

            def _handle(self) -> None:
                length = int(self.headers.get("Content-Length") or 0)
                raw = self.rfile.read(length) if length else b""
                parts = urlsplit(self.path)
                rec = Recorded(
                    method=self.command,
                    path=parts.path,
                    query=parse_qs(parts.query, keep_blank_values=True),
                    headers={k.lower(): v for k, v in self.headers.items()},
                    raw_body=raw,
                    body=json.loads(raw) if raw else None,
                )
                outer.requests.append(rec)
                reply = outer.handler(rec)
                if reply.delay:
                    time.sleep(reply.delay)
                if reply.drop:
                    self.close_connection = True
                    self.connection.close()
                    return
                payload = b""
                if reply.body is not None and reply.status != 204:
                    payload = (reply.body if isinstance(reply.body, str) else json.dumps(reply.body)).encode()
                try:
                    self._reply(reply, payload)
                except (BrokenPipeError, ConnectionResetError):
                    pass  # the client gave up (timeout tests)

            def _reply(self, reply: Reply, payload: bytes) -> None:
                self.send_response(reply.status)
                if payload:
                    ctype = "application/problem+json" if reply.status >= 400 else "application/json"
                    self.send_header("Content-Type", ctype)
                for k, v in reply.headers.items():
                    self.send_header(k, v)
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                if payload:
                    self.wfile.write(payload)

            do_GET = do_POST = do_PATCH = do_PUT = do_DELETE = _handle

        self._server = ThreadingHTTPServer(("127.0.0.1", 0), _Handler)
        self._server.daemon_threads = True
        self.url = f"http://127.0.0.1:{self._server.server_address[1]}"
        self._thread = threading.Thread(target=self._server.serve_forever, kwargs={"poll_interval": 0.05}, daemon=True)

    def start(self) -> "FakeServer":
        self._thread.start()
        return self

    def stop(self) -> None:
        self._server.shutdown()
        self._server.server_close()

    def reset(self) -> None:
        self.requests.clear()

    def sequence(self, *replies: Reply) -> None:
        """Answer with the given replies in order; the last one repeats."""
        state = {"i": 0}

        def handler(_: Recorded) -> Reply:
            i = min(state["i"], len(replies) - 1)
            state["i"] += 1
            return replies[i]

        self.handler = handler

    @property
    def last(self) -> Recorded:
        return self.requests[-1]

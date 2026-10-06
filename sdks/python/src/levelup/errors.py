"""Exceptions raised by the LevelUp SDK."""

from __future__ import annotations

import json
import re
from typing import Any, Dict, Mapping, Optional

__all__ = [
    "LevelUpError",
    "LevelUpConnectionError",
    "LevelUpTimeoutError",
]

_SNAKE = re.compile(r"^[a-z][a-z0-9_]*$")


class LevelUpError(Exception):
    """Every failure surfaced by the SDK.

    For HTTP errors the attributes come from the RFC 9457 problem+json body.
    Branch on ``code`` (stable, snake_case), never on ``detail`` (human text).

    Attributes:
        status: HTTP status, or 0 when no response was received.
        code: machine-readable code, e.g. ``"insufficient_balance"``.
        title / detail / type: problem+json members.
        field_errors: field-level validation messages (422), keyed by field name.
        trace_id: server trace id. Include it in support requests.
        body: parsed response body (or raw text when it was not JSON).
        headers: response headers (lower-cased keys).
    """

    def __init__(
        self,
        status: int,
        code: str,
        message: Optional[str] = None,
        *,
        title: Optional[str] = None,
        detail: Optional[str] = None,
        type: Optional[str] = None,
        field_errors: Optional[Dict[str, str]] = None,
        trace_id: Optional[str] = None,
        body: Any = None,
        headers: Optional[Mapping[str, str]] = None,
    ) -> None:
        self.status = status
        self.code = code
        self.title = title
        self.detail = detail
        self.type = type
        self.field_errors: Dict[str, str] = dict(field_errors or {})
        self.trace_id = trace_id
        self.body = body
        self.headers: Dict[str, str] = {k.lower(): v for k, v in (headers or {}).items()}
        super().__init__(message or self._default_message())

    def _default_message(self) -> str:
        text = self.detail or self.title or "request failed"
        if self.status:
            return f"LevelUp API error {self.status} ({self.code}): {text}"
        return f"LevelUp API error ({self.code}): {text}"

    @classmethod
    def from_response(cls, status: int, headers: Mapping[str, str], raw: bytes) -> "LevelUpError":
        text = raw.decode("utf-8", errors="replace")
        body: Any = text
        problem: Dict[str, Any] = {}
        if text:
            try:
                body = json.loads(text)
                if isinstance(body, dict):
                    problem = body
            except ValueError:
                pass
        title = problem.get("title")
        code = problem.get("code") or (title if isinstance(title, str) and _SNAKE.match(title) else None)
        errors = problem.get("errors")
        return cls(
            status,
            code or _fallback_code(status),
            title=title,
            detail=problem.get("detail"),
            type=problem.get("type"),
            field_errors=errors if isinstance(errors, dict) else None,
            trace_id=problem.get("trace_id"),
            body=body,
            headers=headers,
        )


class LevelUpConnectionError(LevelUpError):
    """No HTTP response was received (DNS failure, refused or reset connection, ...). ``status`` is 0."""

    def __init__(self, message: str, code: str = "connection_error") -> None:
        super().__init__(0, code, message)


class LevelUpTimeoutError(LevelUpConnectionError):
    """The request timed out. ``status`` is 0 and ``code`` is ``"timeout"``."""

    def __init__(self, message: str) -> None:
        super().__init__(message, code="timeout")


def _fallback_code(status: int) -> str:
    return {
        400: "bad_request",
        401: "unauthenticated",
        403: "permission_denied",
        404: "not_found",
        409: "conflict",
        422: "invalid",
        429: "rate_limited",
    }.get(status) or ("server_error" if status >= 500 else f"http_{status}")

"""Low-level transport (stdlib ``urllib`` only)."""

from __future__ import annotations

import datetime as _dt
import email.utils
import json
import platform
import random
import socket
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from typing import Any, Callable, Dict, Mapping, Optional

from ._version import VERSION
from .errors import LevelUpConnectionError, LevelUpError, LevelUpTimeoutError
from .pagination import Page
from .types import RequestOptions

DEFAULT_BASE_URL = "https://api.levelupos.ge"
API_PREFIX = "/api/v1"
RETRYABLE_STATUSES = frozenset({429, 502, 503, 504})


class HttpClient:
    def __init__(
        self,
        api_key: str,
        base_url: str = DEFAULT_BASE_URL,
        timeout: float = 30.0,
        retries: int = 2,
        retry_backoff: float = 0.5,
        max_retry_delay: float = 30.0,
        headers: Optional[Mapping[str, str]] = None,
    ) -> None:
        if not isinstance(api_key, str) or not api_key.strip():
            raise ValueError("LevelUp: api_key is required")
        self._api_key = api_key.strip()
        self.base_url = (base_url or DEFAULT_BASE_URL).rstrip("/")
        self.timeout = timeout
        self.retries = max(0, int(retries))
        self.retry_backoff = max(0.0, float(retry_backoff))
        self.max_retry_delay = max(0.0, float(max_retry_delay))
        self._headers = dict(headers or {})
        self._user_agent = f"levelup-python/{VERSION} python/{platform.python_version()}"
        # Indirection so tests can observe delays without sleeping.
        self._sleep: Callable[[float], None] = time.sleep

    # ------------------------------------------------------------------ public

    def request(
        self,
        method: str,
        path: str,
        *,
        query: Optional[Mapping[str, Any]] = None,
        body: Any = None,
        options: Optional[RequestOptions] = None,
    ) -> Any:
        opts: RequestOptions = options or {}
        url = self.build_url(path, query)
        headers: Dict[str, str] = {
            "Accept": "application/json, application/problem+json",
            "User-Agent": self._user_agent,
            **self._headers,
            **(opts.get("headers") or {}),
            "Authorization": f"Bearer {self._api_key}",
        }
        data: Optional[bytes] = None
        if body is not None:
            headers["Content-Type"] = "application/json"
            data = json.dumps(_jsonable(body), separators=(",", ":")).encode("utf-8")
        idempotency_key = opts.get("idempotency_key")
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key

        method = method.upper()
        safe_to_retry = method == "GET" or bool(idempotency_key)
        max_retries = max(0, int(opts.get("retries", self.retries))) if safe_to_retry else 0
        timeout = opts.get("timeout", self.timeout)

        attempt = 0
        while True:
            can_retry = attempt < max_retries
            try:
                status, resp_headers, raw = self._send(method, url, headers, data, timeout)
            except LevelUpConnectionError:
                if can_retry:
                    self._sleep(self._backoff(attempt))
                    attempt += 1
                    continue
                raise

            if 200 <= status < 300:
                return _parse_success(status, resp_headers, raw)
            if can_retry and status in RETRYABLE_STATUSES:
                retry_after = parse_retry_after(resp_headers.get("retry-after"))
                delay = self._backoff(attempt) if retry_after is None else min(retry_after, self.max_retry_delay)
                self._sleep(delay)
                attempt += 1
                continue
            raise LevelUpError.from_response(status, resp_headers, raw)

    def page(
        self,
        path: str,
        query: Optional[Mapping[str, Any]] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[Any]:
        base = {k: v for k, v in (query or {}).items() if k != "cursor"}

        def fetch(cursor: Optional[str]) -> Page[Any]:
            q = dict(base)
            if cursor:
                q["cursor"] = cursor
            body = self.request("GET", path, query=q, options=options)
            return Page(body if isinstance(body, dict) else {}, fetch, cursor)

        return fetch((query or {}).get("cursor") or None)

    def build_url(self, path: str, query: Optional[Mapping[str, Any]] = None) -> str:
        url = f"{self.base_url}{API_PREFIX}{path}"
        if query:
            pairs = [(k, _query_value(v)) for k, v in query.items() if v is not None and v != ""]
            if pairs:
                url += "?" + urllib.parse.urlencode(pairs)
        return url

    # ---------------------------------------------------------------- internal

    def _send(
        self,
        method: str,
        url: str,
        headers: Dict[str, str],
        data: Optional[bytes],
        timeout: Optional[float],
    ) -> "tuple[int, Dict[str, str], bytes]":
        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                raw = resp.read()
                return resp.status, _lower(resp.headers), raw
        except urllib.error.HTTPError as err:
            try:
                raw = err.read()
            except Exception:  # noqa: BLE001 - body may be unreadable on a broken connection
                raw = b""
            return err.code, _lower(err.headers), raw
        except urllib.error.URLError as err:
            if isinstance(err.reason, (socket.timeout, TimeoutError)):
                raise LevelUpTimeoutError(f"request timed out after {timeout}s") from err
            raise LevelUpConnectionError(f"network error: {err.reason}") from err
        except (socket.timeout, TimeoutError) as err:
            raise LevelUpTimeoutError(f"request timed out after {timeout}s") from err
        except (OSError, ValueError) as err:
            # http.client.RemoteDisconnected / ConnectionResetError / IncompleteRead ...
            raise LevelUpConnectionError(f"network error: {err!r}") from err

    def _backoff(self, attempt: int) -> float:
        """Exponential backoff with jitter: base * 2^attempt, scaled into [50%, 100%], capped."""
        exp = min(self.max_retry_delay, self.retry_backoff * (2**attempt))
        return exp * (0.5 + random.random() * 0.5)


def new_idempotency_key(options: Optional[RequestOptions], explicit: Optional[str]) -> RequestOptions:
    """Options carrying an Idempotency-Key: the caller's, or a fresh UUIDv4."""
    merged: RequestOptions = dict(options or {})  # type: ignore[assignment]
    merged["idempotency_key"] = explicit or merged.get("idempotency_key") or str(uuid.uuid4())
    return merged


def with_idempotency_key(options: Optional[RequestOptions], explicit: Optional[str]) -> Optional[RequestOptions]:
    """Options with the caller's Idempotency-Key, if any (no key is generated)."""
    if not explicit:
        return options
    merged: RequestOptions = dict(options or {})  # type: ignore[assignment]
    merged["idempotency_key"] = explicit
    return merged


def seg(value: str) -> str:
    """Encodes one path segment."""
    if not isinstance(value, str) or not value:
        raise ValueError("LevelUp: path parameter must be a non-empty string")
    return urllib.parse.quote(value, safe="")


def parse_retry_after(value: Optional[str]) -> Optional[float]:
    """Retry-After (delta-seconds or HTTP-date) in seconds, or None."""
    if value is None or not value.strip():
        return None
    value = value.strip()
    try:
        return max(0.0, float(value))
    except ValueError:
        pass
    try:
        when = email.utils.parsedate_to_datetime(value)
    except (TypeError, ValueError):
        return None
    if when is None:
        return None
    if when.tzinfo is None:
        when = when.replace(tzinfo=_dt.timezone.utc)
    return max(0.0, (when - _dt.datetime.now(_dt.timezone.utc)).total_seconds())


def compact(**fields: Any) -> Dict[str, Any]:
    """Drops ``None`` values (omitted optional fields)."""
    return {k: v for k, v in fields.items() if v is not None}


def _parse_success(status: int, headers: Dict[str, str], raw: bytes) -> Any:
    if status == 204 or not raw.strip():
        return None
    try:
        return json.loads(raw.decode("utf-8"))
    except ValueError as err:
        raise LevelUpError(
            status,
            "invalid_response",
            "LevelUp API returned a non-JSON success response",
            body=raw.decode("utf-8", errors="replace"),
            headers=headers,
        ) from err


def _lower(headers: Any) -> Dict[str, str]:
    if headers is None:
        return {}
    return {k.lower(): v for k, v in headers.items()}


def _query_value(value: Any) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, _dt.datetime):
        return _iso(value)
    return str(value)


def _iso(value: _dt.datetime) -> str:
    if value.tzinfo is None:
        value = value.replace(tzinfo=_dt.timezone.utc)
    return value.astimezone(_dt.timezone.utc).isoformat().replace("+00:00", "Z")


def _jsonable(value: Any) -> Any:
    if isinstance(value, _dt.datetime):
        return _iso(value)
    if isinstance(value, dict):
        return {k: _jsonable(v) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [_jsonable(v) for v in value]
    return value

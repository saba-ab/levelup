"""Webhook signature verification.

Every delivery carries::

    LevelUp-Signature: t=<unix seconds>,v1=<hex HMAC-SHA256(secret, "<t>.<raw body>")>
    LevelUp-Event:     <event type, e.g. badges.awarded>
    LevelUp-Delivery:  <delivery id; stable across redeliveries, use it to deduplicate>
"""

from __future__ import annotations

import hashlib
import hmac
import json
import re
import time
from typing import Any, Dict, List, Optional, Tuple, TypedDict, Union

__all__ = [
    "SIGNATURE_HEADER",
    "EVENT_HEADER",
    "DELIVERY_HEADER",
    "DEFAULT_TOLERANCE_SECONDS",
    "WebhookEvent",
    "WebhookVerificationError",
    "verify_webhook",
    "sign_webhook",
]

SIGNATURE_HEADER = "LevelUp-Signature"
EVENT_HEADER = "LevelUp-Event"
DELIVERY_HEADER = "LevelUp-Delivery"
DEFAULT_TOLERANCE_SECONDS = 300



class WebhookEvent(TypedDict):
    """The JSON body of every delivery. ``data`` is the event's own payload."""

    event: str  # same as the LevelUp-Event header, e.g. "badges.awarded"
    event_id: str  # id of the underlying fact
    occurred_at: str
    tenant_id: str
    data: Dict[str, Any]


_HEX64 = re.compile(r"^[0-9a-fA-F]{64}$")


class WebhookVerificationError(Exception):
    """Raised when a delivery cannot be trusted.

    ``code`` is one of: missing_signature, malformed_signature, timestamp_out_of_tolerance,
    signature_mismatch, invalid_payload.
    """

    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


def verify_webhook(
    payload: Union[bytes, bytearray, memoryview, str],
    signature_header: Optional[str],
    secret: str,
    tolerance_seconds: int = DEFAULT_TOLERANCE_SECONDS,
    *,
    now: Optional[float] = None,
) -> Any:
    """Verifies a webhook delivery and returns its parsed JSON body.

    Pass the **raw** request body exactly as received (e.g. ``request.get_data()`` in Flask,
    ``await request.body()`` in Starlette/FastAPI, ``request.body`` in Django). A re-serialised
    body will not verify.

    Raises WebhookVerificationError if the header is missing or malformed, if the timestamp is more
    than ``tolerance_seconds`` away from now (replay protection), or if no ``v1`` signature matches.
    Several ``v1`` entries are accepted (secret rotation); comparison is constant-time.
    """
    if not secret:
        raise ValueError("verify_webhook: secret is required")
    if not signature_header:
        raise WebhookVerificationError("missing_signature", f"missing {SIGNATURE_HEADER} header")

    timestamp, signatures = _parse_header(signature_header)
    current = int(time.time()) if now is None else now
    if abs(current - timestamp) > tolerance_seconds:
        raise WebhookVerificationError("timestamp_out_of_tolerance", "webhook timestamp is outside the tolerance window")

    body = _to_bytes(payload)
    expected = _sign(body, secret, timestamp)
    matched = False
    for candidate in signatures:
        if hmac.compare_digest(candidate, expected):
            matched = True
    if not matched:
        raise WebhookVerificationError("signature_mismatch", "no signature matches the payload")

    try:
        return json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, ValueError) as err:
        raise WebhookVerificationError("invalid_payload", "webhook body is not valid JSON") from err


def sign_webhook(payload: Union[bytes, str], secret: str, timestamp: Optional[int] = None) -> str:
    """Builds a ``LevelUp-Signature`` value, handy for testing your own webhook endpoint."""
    t = int(time.time()) if timestamp is None else int(timestamp)
    return f"t={t},v1={_sign(_to_bytes(payload), secret, t)}"


def _sign(body: bytes, secret: str, timestamp: int) -> str:
    return hmac.new(secret.encode("utf-8"), f"{timestamp}.".encode("ascii") + body, hashlib.sha256).hexdigest()


def _to_bytes(payload: Union[bytes, bytearray, memoryview, str]) -> bytes:
    if isinstance(payload, str):
        return payload.encode("utf-8")
    return bytes(payload)


def _parse_header(header: str) -> Tuple[int, List[str]]:
    timestamp: Optional[int] = None
    signatures: List[str] = []
    for part in header.split(","):
        key, sep, value = part.partition("=")
        if not sep:
            continue
        key, value = key.strip(), value.strip()
        if key == "t" and value.isdigit():
            timestamp = int(value)
        elif key == "v1" and _HEX64.match(value):
            signatures.append(value.lower())
    if timestamp is None or not signatures:
        raise WebhookVerificationError("malformed_signature", f"malformed {SIGNATURE_HEADER} header")
    return timestamp, signatures

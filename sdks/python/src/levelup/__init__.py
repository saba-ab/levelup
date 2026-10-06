"""Official server-side Python SDK for the LevelUp API."""

from ._client import Client
from ._http import API_PREFIX, DEFAULT_BASE_URL, RETRYABLE_STATUSES
from ._version import VERSION
from .errors import LevelUpConnectionError, LevelUpError, LevelUpTimeoutError
from .pagination import Page
from .webhooks import (
    DEFAULT_TOLERANCE_SECONDS,
    DELIVERY_HEADER,
    EVENT_HEADER,
    SIGNATURE_HEADER,
    WebhookEvent,
    WebhookVerificationError,
    sign_webhook,
    verify_webhook,
)

__version__ = VERSION

__all__ = [
    "API_PREFIX",
    "Client",
    "DEFAULT_BASE_URL",
    "DEFAULT_TOLERANCE_SECONDS",
    "DELIVERY_HEADER",
    "EVENT_HEADER",
    "LevelUpConnectionError",
    "LevelUpError",
    "LevelUpTimeoutError",
    "Page",
    "RETRYABLE_STATUSES",
    "SIGNATURE_HEADER",
    "VERSION",
    "WebhookEvent",
    "WebhookVerificationError",
    "sign_webhook",
    "verify_webhook",
]

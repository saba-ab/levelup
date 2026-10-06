from __future__ import annotations

from typing import Any, Mapping, Optional

from ._http import DEFAULT_BASE_URL, HttpClient
from .resources.activities import Activities
from .resources.engagement import Badges, Leaderboards, Missions, Progression, Rewards, Rules, Streaks
from .resources.players import Players
from .resources.wallets import Wallets
from .types import RequestOptions


class Client:
    """LevelUp API client for server-side integrations.

    >>> import levelup
    >>> client = levelup.Client(api_key="lvl_live_...")
    >>> client.activities.send(event_id="order-1042", event_type="purchase", player_external_id="user-7")

    Args:
        api_key: ``lvl_live_…`` key, sent as ``Authorization: Bearer <key>``.
        base_url: API origin (default ``https://api.levelupos.ge``); ``/api/v1`` is appended.
        timeout: socket timeout per attempt, in seconds (default 30).
        retries: retries after the first attempt for retry-safe requests (default 2).
        retry_backoff: base delay of the exponential backoff, in seconds (default 0.5).
        max_retry_delay: cap for any single retry delay, including Retry-After (default 30).
        headers: extra headers sent with every request.

    Never ship an API key to a browser or mobile app.
    """

    def __init__(
        self,
        api_key: str,
        base_url: str = DEFAULT_BASE_URL,
        timeout: float = 30.0,
        retries: int = 2,
        *,
        retry_backoff: float = 0.5,
        max_retry_delay: float = 30.0,
        headers: Optional[Mapping[str, str]] = None,
    ) -> None:
        self._http = HttpClient(
            api_key,
            base_url=base_url,
            timeout=timeout,
            retries=retries,
            retry_backoff=retry_backoff,
            max_retry_delay=max_retry_delay,
            headers=headers,
        )
        self.activities = Activities(self._http)
        self.players = Players(self._http)
        self.wallets = Wallets(self._http)
        self.progression = Progression(self._http)
        self.badges = Badges(self._http)
        self.missions = Missions(self._http)
        self.streaks = Streaks(self._http)
        self.rewards = Rewards(self._http)
        self.leaderboards = Leaderboards(self._http)
        self.rules = Rules(self._http)

    @property
    def base_url(self) -> str:
        return self._http.base_url

    def request(
        self,
        method: str,
        path: str,
        *,
        query: Optional[Mapping[str, Any]] = None,
        body: Any = None,
        options: Optional[RequestOptions] = None,
    ) -> Any:
        """Escape hatch for endpoints without a dedicated method. ``path`` is relative to ``/api/v1``."""
        return self._http.request(method, path, query=query, body=body, options=options)

    def __repr__(self) -> str:
        return f"levelup.Client(base_url={self.base_url!r})"

from __future__ import annotations

import datetime as dt
from typing import Any, Dict, Iterator, Optional, Sequence, Union

from .._http import HttpClient, compact, seg, with_idempotency_key
from ..pagination import Page
from ..types import Activity, ActivityInput, RequestOptions, SendActivityBatchResponse, SendActivityResponse

Timestamp = Union[str, dt.datetime]


class Activities:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def send(
        self,
        event_id: str,
        event_type: str,
        player_external_id: str,
        properties: Optional[Dict[str, Any]] = None,
        occurred_at: Optional[Timestamp] = None,
        context: Optional[Dict[str, Any]] = None,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> SendActivityResponse:
        """Reports one activity.

        Answers with ``status`` "pending" (accepted, 202), or ``duplicate=True`` when ``event_id``
        was already received. It is not retried unless an ``idempotency_key`` is given; using the
        ``event_id`` as the key is a good choice.
        """
        body = compact(
            event_id=event_id,
            event_type=event_type,
            player_external_id=player_external_id,
            properties=properties,
            occurred_at=occurred_at,
            context=context,
        )
        return self._http.request(
            "POST", "/activities", body=body, options=with_idempotency_key(options, idempotency_key)
        )

    def send_batch(
        self,
        items: Sequence[ActivityInput],
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> SendActivityBatchResponse:
        """Reports up to 100 activities. Each item succeeds or fails on its own; see ``results``."""
        return self._http.request(
            "POST",
            "/activities/batch",
            body={"items": [dict(item) for item in items]},
            options=with_idempotency_key(options, idempotency_key),
        )

    def get(self, activity_id: str, *, options: Optional[RequestOptions] = None) -> Activity:
        return self._http.request("GET", f"/activities/{seg(activity_id)}", options=options)

    def list(
        self,
        *,
        event_type: Optional[str] = None,
        player_external_id: Optional[str] = None,
        status: Optional[str] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[Activity]:
        query = compact(
            event_type=event_type, player_external_id=player_external_id, status=status, limit=limit, cursor=cursor
        )
        return self._http.page("/activities", query, options)

    def iterate(self, **filters: Any) -> Iterator[Activity]:
        """Every matching activity across all pages. Accepts the same keywords as ``list``."""
        return iter(self.list(**filters))

from __future__ import annotations

from typing import Any, Dict, Iterator, Optional

from .._http import HttpClient, compact, seg, with_idempotency_key
from ..pagination import Page
from ..types import Player, RequestOptions


class Players:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def create(
        self,
        external_id: str,
        display_name: Optional[str] = None,
        email: Optional[str] = None,
        attributes: Optional[Dict[str, Any]] = None,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Player:
        """Creates a player. 409 ``player_external_id_taken`` when ``external_id`` is already used."""
        body = compact(external_id=external_id, display_name=display_name, email=email, attributes=attributes)
        return self._http.request("POST", "/players", body=body, options=with_idempotency_key(options, idempotency_key))

    def get(self, player_id: str, *, options: Optional[RequestOptions] = None) -> Player:
        return self._http.request("GET", f"/players/{seg(player_id)}", options=options)

    def get_by_external_id(self, external_id: str, *, options: Optional[RequestOptions] = None) -> Player:
        """Looks a player up by your own id."""
        return self._http.request("GET", f"/players/by-external-id/{seg(external_id)}", options=options)

    def update(
        self,
        player_id: str,
        *,
        display_name: Optional[str] = None,
        email: Optional[str] = None,
        attributes: Optional[Dict[str, Any]] = None,
        is_active: Optional[bool] = None,
        options: Optional[RequestOptions] = None,
    ) -> Player:
        """Partial update: only the fields you pass (not ``None``) are changed."""
        body = compact(display_name=display_name, email=email, attributes=attributes, is_active=is_active)
        return self._http.request("PATCH", f"/players/{seg(player_id)}", body=body, options=options)

    def list(
        self,
        *,
        is_active: Optional[bool] = None,
        search: Optional[str] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[Player]:
        return self._http.page("/players", compact(is_active=is_active, search=search, limit=limit, cursor=cursor), options)

    def iterate(self, **filters: Any) -> Iterator[Player]:
        """Every matching player across all pages: ``for p in client.players.iterate(is_active=True): ...``"""
        return iter(self.list(**filters))

    def activate(self, player_id: str, *, options: Optional[RequestOptions] = None) -> Player:
        return self._http.request("POST", f"/players/{seg(player_id)}/activate", options=options)

    def deactivate(self, player_id: str, *, options: Optional[RequestOptions] = None) -> Player:
        return self._http.request("POST", f"/players/{seg(player_id)}/deactivate", options=options)

    def delete(self, player_id: str, *, options: Optional[RequestOptions] = None) -> None:
        self._http.request("DELETE", f"/players/{seg(player_id)}", options=options)

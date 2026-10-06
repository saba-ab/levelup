from __future__ import annotations

from typing import Any, Iterator, Optional

from .._http import HttpClient, compact, new_idempotency_key, seg
from ..pagination import Page
from ..types import LedgerEntry, RequestOptions, TransferResponse, Wallet


class Wallets:
    """Points wallets.

    ``credit``, ``debit`` and ``transfer`` ALWAYS send an Idempotency-Key: yours when you pass
    ``idempotency_key``, otherwise a fresh UUIDv4 per call. Pass your own (e.g. your order id)
    so that re-running your job cannot move points twice.
    """

    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def get(self, player_id: str, *, options: Optional[RequestOptions] = None) -> Wallet:
        return self._http.request("GET", f"/players/{seg(player_id)}/wallet", options=options)

    def credit(
        self,
        player_id: str,
        amount: int,
        kind: str = "earn",
        description: Optional[str] = None,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> LedgerEntry:
        """Credits points. ``kind``: earn | bonus | reward | adjustment."""
        return self._http.request(
            "POST",
            f"/players/{seg(player_id)}/wallet/credit",
            body=compact(amount=amount, kind=kind, description=description),
            options=new_idempotency_key(options, idempotency_key),
        )

    def debit(
        self,
        player_id: str,
        amount: int,
        kind: str = "spend",
        description: Optional[str] = None,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> LedgerEntry:
        """Debits points. ``kind``: spend | redeem | penalty | expire. 422 ``insufficient_balance`` if too low."""
        return self._http.request(
            "POST",
            f"/players/{seg(player_id)}/wallet/debit",
            body=compact(amount=amount, kind=kind, description=description),
            options=new_idempotency_key(options, idempotency_key),
        )

    def transfer(
        self,
        from_player_id: str,
        to_player_id: str,
        amount: int,
        description: Optional[str] = None,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> TransferResponse:
        return self._http.request(
            "POST",
            "/wallets/transfer",
            body=compact(
                from_player_id=from_player_id, to_player_id=to_player_id, amount=amount, description=description
            ),
            options=new_idempotency_key(options, idempotency_key),
        )

    def transactions(
        self,
        player_id: str,
        *,
        kind: Optional[str] = None,
        direction: Optional[str] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[LedgerEntry]:
        """The player's ledger, newest first."""
        query = compact(kind=kind, direction=direction, limit=limit, cursor=cursor)
        return self._http.page(f"/players/{seg(player_id)}/wallet/transactions", query, options)

    def iter_transactions(self, player_id: str, **filters: Any) -> Iterator[LedgerEntry]:
        return iter(self.transactions(player_id, **filters))

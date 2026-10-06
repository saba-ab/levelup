"""Cursor pagination (``{"data": [...], "next_cursor": "..."}``)."""

from __future__ import annotations

from typing import Any, Callable, Dict, Generic, Iterator, List, Optional, TypeVar

T = TypeVar("T")

__all__ = ["Page"]


class Page(Generic[T]):
    """One page of a cursor-paginated list.

    * ``page.data``: this page's items. ``page.next_cursor``: the next cursor, or ``None`` on the last page.
    * ``page.next_page()``: fetches the following page (``None`` when there is none).
    * Iterating a page yields **every** item from this page onwards, fetching further
      pages lazily: ``for player in client.players.list(): ...``
    * ``page.iter_pages()`` yields page by page instead.
    * ``page.body`` is the full envelope (some lists carry extra fields, e.g. ``period_start``).
    """

    def __init__(
        self,
        body: Dict[str, Any],
        fetch_page: Callable[[str], "Page[T]"],
        requested_cursor: Optional[str] = None,
    ) -> None:
        self.body = body
        data = body.get("data")
        self.data: List[T] = list(data) if isinstance(data, list) else []
        nxt = body.get("next_cursor") or None
        # Guard against a server echoing the same cursor back, which would loop forever.
        self.next_cursor: Optional[str] = None if nxt is not None and nxt == requested_cursor else nxt
        self._fetch_page = fetch_page

    def has_next_page(self) -> bool:
        return self.next_cursor is not None

    def next_page(self) -> Optional["Page[T]"]:
        if self.next_cursor is None:
            return None
        return self._fetch_page(self.next_cursor)

    def iter_pages(self) -> Iterator["Page[T]"]:
        page: Optional[Page[T]] = self
        while page is not None:
            yield page
            page = page.next_page()

    def __iter__(self) -> Iterator[T]:
        for page in self.iter_pages():
            yield from page.data

    def __repr__(self) -> str:
        return f"Page(items={len(self.data)}, next_cursor={self.next_cursor!r})"

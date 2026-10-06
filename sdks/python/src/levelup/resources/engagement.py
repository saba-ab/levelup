from __future__ import annotations

import datetime as dt
from typing import Any, Dict, Iterator, Optional, Union

from .._http import HttpClient, compact, seg, with_idempotency_key
from ..pagination import Page
from ..types import (
    AwardBadgeResponse,
    Badge,
    GrantXpResponse,
    Leaderboard,
    LeaderboardEntry,
    Mission,
    MissionAttempt,
    MissionProgressResponse,
    PlayerBadge,
    PlayerProgress,
    PlayerStanding,
    PlayerStreak,
    RecordStreakResponse,
    RequestOptions,
    Reward,
    RewardClaim,
    SimulatedPlayer,
    SimulateRulesResponse,
    Streak,
)

Timestamp = Union[str, dt.datetime]


class Progression:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def grant_xp(
        self,
        player_id: str,
        amount: int,
        description: Optional[str] = None,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> GrantXpResponse:
        """Grants XP. Pass ``idempotency_key`` to make it retry-safe (a replay answers ``replayed=True``)."""
        return self._http.request(
            "POST",
            f"/players/{seg(player_id)}/xp",
            body=compact(amount=amount, description=description),
            options=with_idempotency_key(options, idempotency_key),
        )

    def get_progress(self, player_id: str, *, options: Optional[RequestOptions] = None) -> PlayerProgress:
        return self._http.request("GET", f"/players/{seg(player_id)}/progress", options=options)


class Badges:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def list(
        self,
        *,
        tier: Optional[str] = None,
        category: Optional[str] = None,
        active: Optional[bool] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[Badge]:
        query = compact(tier=tier, category=category, active=active, limit=limit, cursor=cursor)
        return self._http.page("/badges", query, options)

    def iterate(self, **filters: Any) -> Iterator[Badge]:
        return iter(self.list(**filters))

    def award(
        self,
        badge_id: str,
        player_id: str,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> AwardBadgeResponse:
        """Awards a badge. 409 ``badge_already_earned`` for a non-stackable badge the player holds."""
        return self._http.request(
            "POST",
            f"/badges/{seg(badge_id)}/award",
            body={"player_id": player_id},
            options=with_idempotency_key(options, idempotency_key),
        )

    def revoke(self, badge_id: str, player_id: str, *, options: Optional[RequestOptions] = None) -> None:
        self._http.request("DELETE", f"/badges/{seg(badge_id)}/players/{seg(player_id)}", options=options)

    def list_for_player(
        self,
        player_id: str,
        *,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[PlayerBadge]:
        """Badges a player holds."""
        return self._http.page(f"/players/{seg(player_id)}/badges", compact(limit=limit, cursor=cursor), options)


class Missions:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def list(
        self,
        *,
        status: Optional[str] = None,
        type: Optional[str] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[Mission]:
        return self._http.page("/missions", compact(status=status, type=type, limit=limit, cursor=cursor), options)

    def iterate(self, **filters: Any) -> Iterator[Mission]:
        return iter(self.list(**filters))

    def progress(
        self,
        mission_id: str,
        player_id: str,
        increment: int = 1,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> MissionProgressResponse:
        """Adds progress (1 to 1 000 000) to the player's current attempt."""
        return self._http.request(
            "POST",
            f"/missions/{seg(mission_id)}/progress",
            body={"player_id": player_id, "increment": increment},
            options=with_idempotency_key(options, idempotency_key),
        )

    def start(self, mission_id: str, player_id: str, *, options: Optional[RequestOptions] = None) -> MissionAttempt:
        return self._http.request(
            "POST", f"/missions/{seg(mission_id)}/start", body={"player_id": player_id}, options=options
        )

    def complete(self, mission_id: str, player_id: str, *, options: Optional[RequestOptions] = None) -> MissionAttempt:
        return self._http.request(
            "POST", f"/missions/{seg(mission_id)}/complete", body={"player_id": player_id}, options=options
        )

    def list_for_player(
        self,
        player_id: str,
        *,
        status: Optional[str] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[MissionAttempt]:
        """A player's mission attempts."""
        query = compact(status=status, limit=limit, cursor=cursor)
        return self._http.page(f"/players/{seg(player_id)}/missions", query, options)


class Streaks:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def list(
        self,
        *,
        active: Optional[bool] = None,
        period: Optional[str] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[Streak]:
        return self._http.page("/streaks", compact(active=active, period=period, limit=limit, cursor=cursor), options)

    def iterate(self, **filters: Any) -> Iterator[Streak]:
        return iter(self.list(**filters))

    def record(
        self,
        streak_id: str,
        player_id: str,
        occurred_at: Optional[Timestamp] = None,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> RecordStreakResponse:
        """Records qualifying activity for the player in the streak's current period."""
        return self._http.request(
            "POST",
            f"/streaks/{seg(streak_id)}/record",
            body=compact(player_id=player_id, occurred_at=occurred_at),
            options=with_idempotency_key(options, idempotency_key),
        )

    def list_for_player(self, player_id: str, *, options: Optional[RequestOptions] = None) -> Page[PlayerStreak]:
        """A player's streaks."""
        return self._http.page(f"/players/{seg(player_id)}/streaks", None, options)


class Rewards:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def list(
        self,
        *,
        status: Optional[str] = None,
        type: Optional[str] = None,
        is_active: Optional[bool] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[Reward]:
        query = compact(status=status, type=type, is_active=is_active, limit=limit, cursor=cursor)
        return self._http.page("/rewards", query, options)

    def iterate(self, **filters: Any) -> Iterator[Reward]:
        return iter(self.list(**filters))

    def claim(
        self,
        reward_id: str,
        player_id: str,
        *,
        idempotency_key: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> RewardClaim:
        """Claims a reward for a player (spends ``points_cost``). Pass ``idempotency_key`` to make it retry-safe."""
        return self._http.request(
            "POST",
            f"/rewards/{seg(reward_id)}/claim",
            body={"player_id": player_id},
            options=with_idempotency_key(options, idempotency_key),
        )

    def get_claim(self, claim_id: str, *, options: Optional[RequestOptions] = None) -> RewardClaim:
        return self._http.request("GET", f"/rewards/claims/{seg(claim_id)}", options=options)

    def redeem(self, claim_id: str, *, options: Optional[RequestOptions] = None) -> RewardClaim:
        """Marks a claim as redeemed (e.g. the voucher was used at checkout)."""
        return self._http.request("POST", f"/rewards/claims/{seg(claim_id)}/redeem", options=options)

    def cancel_claim(self, claim_id: str, *, options: Optional[RequestOptions] = None) -> RewardClaim:
        return self._http.request("POST", f"/rewards/claims/{seg(claim_id)}/cancel", options=options)

    def list_claims(
        self,
        player_id: str,
        *,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[RewardClaim]:
        """A player's reward claims."""
        return self._http.page(f"/players/{seg(player_id)}/reward-claims", compact(limit=limit, cursor=cursor), options)


class Leaderboards:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def list(
        self,
        *,
        type: Optional[str] = None,
        active: Optional[bool] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[Leaderboard]:
        return self._http.page("/leaderboards", compact(type=type, active=active, limit=limit, cursor=cursor), options)

    def iterate(self, **filters: Any) -> Iterator[Leaderboard]:
        return iter(self.list(**filters))

    def entries(
        self,
        leaderboard_id: str,
        *,
        period: Optional[Timestamp] = None,
        limit: Optional[int] = None,
        cursor: Optional[str] = None,
        options: Optional[RequestOptions] = None,
    ) -> Page[LeaderboardEntry]:
        """Ranked entries. ``period``: "current" (default) or a time inside the wanted period.

        ``page.body["period_start"]`` / ``["period_end"]`` describe the period.
        """
        query = compact(period=period, limit=limit, cursor=cursor)
        return self._http.page(f"/leaderboards/{seg(leaderboard_id)}/entries", query, options)

    def player_standing(
        self,
        leaderboard_id: str,
        player_id: str,
        *,
        period: Optional[Timestamp] = None,
        around: Optional[int] = None,
        options: Optional[RequestOptions] = None,
    ) -> PlayerStanding:
        """A player's rank, optionally with ``around`` neighbours on each side."""
        return self._http.request(
            "GET",
            f"/leaderboards/{seg(leaderboard_id)}/players/{seg(player_id)}",
            query=compact(period=period, around=around),
            options=options,
        )


class Rules:
    def __init__(self, http: HttpClient) -> None:
        self._http = http

    def simulate(
        self,
        event_type: str,
        *,
        player_id: Optional[str] = None,
        player_external_id: Optional[str] = None,
        player: Optional[SimulatedPlayer] = None,
        properties: Optional[Dict[str, Any]] = None,
        context: Optional[Dict[str, Any]] = None,
        causation_depth: Optional[int] = None,
        options: Optional[RequestOptions] = None,
    ) -> SimulateRulesResponse:
        """Dry-runs the published rules against an event. Nothing is recorded or awarded.

        Identify an existing player (``player_id`` / ``player_external_id``) or describe a hypothetical ``player``.
        """
        body = compact(
            event_type=event_type,
            player_id=player_id,
            player_external_id=player_external_id,
            player=dict(player) if player is not None else None,
            properties=properties,
            context=context,
            causation_depth=causation_depth,
        )
        return self._http.request("POST", "/rules/simulate", body=body, options=options)

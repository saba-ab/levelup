"""Response bodies of the LevelUp API, as TypedDicts.

Hand-written from the transport DTOs in ``backend/api/docs/swagger.json``. Responses are
plain ``dict`` objects at runtime; these types exist for editors and type checkers.
Ids are UUID strings and timestamps are RFC 3339 UTC strings.
"""

from __future__ import annotations

from typing import Any, Dict, List, Optional, TypedDict

JsonObject = Dict[str, Any]


class Problem(TypedDict, total=False):
    type: str
    title: str
    status: int
    detail: str
    code: str
    errors: Dict[str, str]
    trace_id: str


class RequestOptions(TypedDict, total=False):
    """Per-call overrides accepted by every resource method as ``options=``."""

    timeout: float
    retries: int
    headers: Dict[str, str]
    idempotency_key: str


# --- Activities -------------------------------------------------------------


class Activity(TypedDict, total=False):
    id: str
    event_id: str
    event_type: str
    player_external_id: str
    player_id: str
    status: str  # "pending" | "decided" | "rejected"
    outcome: str
    reason: str
    decision_id: str
    properties: JsonObject
    context: JsonObject
    causation_depth: int
    source_event_id: str
    occurred_at: str
    received_at: str
    decided_at: str
    created_at: str


class ActivityInput(TypedDict, total=False):
    """One item of ``activities.send_batch``. ``event_id``, ``event_type``, ``player_external_id`` are required."""

    event_id: str
    event_type: str
    player_external_id: str
    properties: JsonObject
    occurred_at: str
    context: JsonObject


class SendActivityResponse(TypedDict, total=False):
    activity_id: str
    status: str
    duplicate: bool
    activity: Activity


class BatchItemError(TypedDict, total=False):
    status: int
    code: str
    detail: str
    errors: Dict[str, str]


class BatchItemResult(TypedDict, total=False):
    index: int
    event_id: str
    activity_id: str
    status: str
    duplicate: bool
    error: BatchItemError


class SendActivityBatchResponse(TypedDict, total=False):
    accepted: int
    duplicates: int
    failed: int
    results: List[BatchItemResult]


# --- Players ----------------------------------------------------------------


class Player(TypedDict, total=False):
    id: str
    tenant_id: str
    external_id: str
    display_name: str
    email: str
    attributes: JsonObject
    is_active: bool
    version: int
    created_by: str
    created_at: str
    updated_at: str


# --- Wallets ----------------------------------------------------------------


class Wallet(TypedDict, total=False):
    id: str
    player_id: str
    balance: int
    lifetime_earned: int
    lifetime_spent: int
    is_active: bool
    opened: bool
    version: int
    created_at: str
    updated_at: str


class LedgerEntry(TypedDict, total=False):
    id: str
    wallet_id: str
    player_id: str
    direction: str  # "credit" | "debit"
    kind: str
    amount: int
    balance_before: int
    balance_after: int
    description: str
    activity_id: str
    source_kind: str
    source_id: str
    transfer_id: str
    reversal_of: str
    created_by: str
    occurred_at: str
    created_at: str


# "from" is a Python keyword, hence the functional syntax.
TransferResponse = TypedDict(
    "TransferResponse",
    {"transfer_id": str, "from": LedgerEntry, "to": LedgerEntry},
    total=False,
)


# --- Progression ------------------------------------------------------------


class LevelRef(TypedDict, total=False):
    id: str
    level_number: int
    name: str
    xp_required: int


class XpGrant(TypedDict, total=False):
    id: str
    player_id: str
    amount: int
    description: str
    idempotency_key: str
    activity_id: str
    source_kind: str
    source_id: str
    created_by: str
    occurred_at: str
    created_at: str


class GrantXpResponse(TypedDict, total=False):
    grant: XpGrant
    total_xp: int
    level_number: int
    levels_reached: List[LevelRef]
    replayed: bool


class PlayerProgress(TypedDict, total=False):
    player_id: str
    total_xp: int
    current_level: Optional[LevelRef]
    next_level: Optional[LevelRef]
    xp_to_next: int
    progress_percent: float


# --- Badges -----------------------------------------------------------------


class Badge(TypedDict, total=False):
    id: str
    tenant_id: str
    slug: str
    name: str
    description: str
    icon_url: str
    category: str
    tier: str  # bronze | silver | gold | platinum | diamond
    points_value: int
    requirements: JsonObject
    is_active: bool
    is_secret: bool
    is_stackable: bool
    max_awards: int
    sort_order: int
    version: int
    created_at: str
    updated_at: str
    deleted_at: str


class PlayerBadge(TypedDict, total=False):
    id: str
    player_id: str
    badge_id: str
    badge: Badge
    earned_count: int
    first_awarded_at: str
    last_awarded_at: str
    version: int
    created_at: str


class AwardBadgeResponse(TypedDict, total=False):
    award_id: str
    status: str
    replay: bool
    idempotency_key: str
    player_badge: PlayerBadge


# --- Missions ---------------------------------------------------------------


class Mission(TypedDict, total=False):
    id: str
    slug: str
    name: str
    description: str
    type: str
    status: str
    target: int
    criteria: JsonObject
    points_reward: int
    xp_reward: int
    badge_reward_id: str
    max_completions_per_player: int
    starts_at: str
    ends_at: str
    version: int
    created_at: str
    updated_at: str


class MissionSummary(TypedDict, total=False):
    id: str
    slug: str
    name: str
    type: str
    status: str
    deleted: bool


class MissionAttempt(TypedDict, total=False):
    id: str
    mission_id: str
    mission: MissionSummary
    player_id: str
    period_key: str
    status: str
    progress: int
    target: int
    started_at: str
    completed_at: str


class MissionProgressResponse(TypedDict, total=False):
    attempt: MissionAttempt
    completed: bool
    duplicate: bool


# --- Streaks ----------------------------------------------------------------


class StreakMilestone(TypedDict, total=False):
    count: int
    bonus_points: int


class Streak(TypedDict, total=False):
    id: str
    tenant_id: str
    slug: str
    name: str
    description: str
    activity_key: str
    period: str
    grace_periods: int
    points_per_period: int
    milestones: List[StreakMilestone]
    is_active: bool
    created_at: str
    updated_at: str


class PlayerStreak(TypedDict, total=False):
    id: str
    streak_id: str
    streak: Streak
    player_id: str
    current_count: int
    longest_count: int
    run_started_at: str
    last_period_start: str
    broken_at: str


class RecordStreakResponse(TypedDict, total=False):
    outcome: str
    new_period: bool
    period_start: str
    milestones_reached: List[int]
    player_streak: PlayerStreak


# --- Rewards ----------------------------------------------------------------


class Reward(TypedDict, total=False):
    id: str
    tenant_id: str
    slug: str
    name: str
    description: str
    type: str
    status: str
    value: str
    value_type: str
    points_cost: int
    level_requirement: int
    badge_reward_id: str
    level_reward_id: str
    max_per_player: int
    max_redemptions: int
    stock_used: int
    claim_ttl_days: int
    metadata: JsonObject
    is_active: bool
    start_at: str
    end_at: str
    created_at: str
    updated_at: str


class RewardClaim(TypedDict, total=False):
    id: str
    tenant_id: str
    reward_id: str
    reward_slug: str
    reward_type: str
    player_id: str
    status: str
    points_cost: int
    code: str
    reject_reason: str
    claimed_at: str
    hold_expires_at: str
    expires_at: str
    redeemed_at: str
    cancelled_at: str
    created_at: str
    updated_at: str


# --- Leaderboards -----------------------------------------------------------


class Leaderboard(TypedDict, total=False):
    id: str
    tenant_id: str
    program_id: str
    slug: str
    name: str
    description: str
    type: str
    metric: str
    reset_frequency: str
    max_entries: int
    is_active: bool
    created_at: str
    updated_at: str


class LeaderboardEntry(TypedDict, total=False):
    rank: int
    score: int
    player_id: str
    external_id: str
    display_name: str


class PlayerStanding(TypedDict, total=False):
    entry: LeaderboardEntry
    neighbours: List[LeaderboardEntry]
    period_start: str
    period_end: str


# --- Rules ------------------------------------------------------------------


class SimulatedPlayer(TypedDict, total=False):
    external_id: str
    is_active: bool
    level: int
    xp: int
    points: int
    attributes: JsonObject


class ConditionTrace(TypedDict, total=False):
    path: str
    field: str
    source: str
    operator: str
    expected: Any
    actual: Any
    present: bool
    result: bool


class SimulatedEffect(TypedDict, total=False):
    action_index: int
    type: str
    params: JsonObject


class SimulatedLimits(TypedDict, total=False):
    cooldown_seconds: int
    max_per_player: int
    max_per_player_per_day: int
    max_per_player_per_week: int


class SimulatedRule(TypedDict, total=False):
    rule_id: str
    rule_version_id: str
    name: str
    priority: int
    status: str
    matched: bool
    condition_results: List[ConditionTrace]
    effects: List[SimulatedEffect]
    limits: SimulatedLimits
    error: str


class SimulateRulesResponse(TypedDict, total=False):
    outcome: str
    reason: str
    player_id: str
    ruleset_generation: int
    limits_enforced: bool
    rules: List[SimulatedRule]

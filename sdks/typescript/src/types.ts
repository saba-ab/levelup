/**
 * Request and response bodies of the LevelUp API (`/api/v1`).
 *
 * Hand-written from the transport DTOs in `backend/api/docs/swagger.json`.
 * Field names are the wire names (snake_case). Ids are UUID strings and
 * timestamps are RFC 3339 UTC strings.
 */

/** A free-form JSON object. */
export type JsonObject = Record<string, unknown>;

/** RFC 3339 timestamp string, or a Date (serialised with `toISOString()`). */
export type Timestamp = string | Date;

// ---------------------------------------------------------------------------
// Pagination (ADR-0016)
// ---------------------------------------------------------------------------

/** Every list endpoint answers with this envelope. `next_cursor` is "" on the last page. */
export interface CursorPage<T> {
  data: T[];
  next_cursor: string;
}

export interface ListParams {
  /** Page size, 1–100 (server default 25). */
  limit?: number;
  /** Opaque cursor from a previous page's `next_cursor`. */
  cursor?: string;
}

// ---------------------------------------------------------------------------
// Errors (ADR-0016, RFC 9457)
// ---------------------------------------------------------------------------

export interface Problem {
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
  /** Machine-readable, snake_case. Branch on this, never on `detail`. */
  code?: string;
  /** Field-level validation messages, keyed by field name. */
  errors?: Record<string, string>;
  trace_id?: string;
}

// ---------------------------------------------------------------------------
// Activities
// ---------------------------------------------------------------------------

export type ActivityStatus = "pending" | "decided" | "rejected" | (string & {});

export interface Activity {
  id: string;
  event_id: string;
  event_type: string;
  player_external_id: string;
  player_id?: string;
  status: ActivityStatus;
  outcome?: string;
  reason?: string;
  decision_id?: string;
  properties?: JsonObject;
  context?: JsonObject;
  causation_depth?: number;
  source_event_id?: string;
  occurred_at: string;
  received_at?: string;
  decided_at?: string;
  created_at: string;
}

export interface SendActivityParams {
  /** Your unique id for this event (max 128). Re-sending the same id is deduplicated. */
  event_id: string;
  /** Event type slug from the event catalog (max 100). */
  event_type: string;
  /** Your id for the player (max 255). */
  player_external_id: string;
  properties?: JsonObject;
  occurred_at?: Timestamp;
  context?: JsonObject;
}

export interface SendActivityResponse {
  activity_id: string;
  status: string;
  duplicate: boolean;
  activity?: Activity;
}

export interface BatchItemError {
  status?: number;
  code?: string;
  detail?: string;
  errors?: Record<string, string>;
}

export interface BatchItemResult {
  index: number;
  event_id: string;
  activity_id?: string;
  status: string;
  duplicate?: boolean;
  error?: BatchItemError;
}

export interface SendActivityBatchResponse {
  accepted: number;
  duplicates: number;
  failed: number;
  results: BatchItemResult[];
}

export interface ListActivitiesParams extends ListParams {
  event_type?: string;
  player_external_id?: string;
  status?: string;
}

// ---------------------------------------------------------------------------
// Players
// ---------------------------------------------------------------------------

export interface Player {
  id: string;
  tenant_id: string;
  external_id: string;
  display_name?: string;
  email?: string;
  attributes?: JsonObject;
  is_active: boolean;
  version: number;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface CreatePlayerParams {
  external_id: string;
  display_name?: string;
  email?: string;
  attributes?: JsonObject;
}

/** Partial update: omitted fields are left untouched. */
export interface UpdatePlayerParams {
  display_name?: string;
  email?: string;
  attributes?: JsonObject;
  is_active?: boolean;
}

export interface ListPlayersParams extends ListParams {
  is_active?: boolean;
  search?: string;
}

// ---------------------------------------------------------------------------
// Wallets / points
// ---------------------------------------------------------------------------

export interface Wallet {
  id: string;
  player_id: string;
  balance: number;
  lifetime_earned: number;
  lifetime_spent: number;
  is_active: boolean;
  /** False when the player has no wallet row yet (zero balances are returned). */
  opened: boolean;
  version: number;
  created_at?: string;
  updated_at?: string;
}

export type CreditKind = "earn" | "bonus" | "reward" | "adjustment";
export type DebitKind = "spend" | "redeem" | "penalty" | "expire";

export interface LedgerEntry {
  id: string;
  wallet_id: string;
  player_id: string;
  direction: "credit" | "debit" | (string & {});
  kind: string;
  amount: number;
  balance_before: number;
  balance_after: number;
  description?: string;
  activity_id?: string;
  source_kind?: string;
  source_id?: string;
  transfer_id?: string;
  reversal_of?: string;
  created_by?: string;
  occurred_at: string;
  created_at: string;
}

export interface CreditParams {
  amount: number;
  kind: CreditKind;
  description?: string;
}

export interface DebitParams {
  amount: number;
  kind: DebitKind;
  description?: string;
}

export interface TransferParams {
  from_player_id: string;
  to_player_id: string;
  amount: number;
  description?: string;
}

export interface TransferResponse {
  transfer_id: string;
  from: LedgerEntry;
  to: LedgerEntry;
}

export interface ListTransactionsParams extends ListParams {
  kind?: string;
  direction?: "credit" | "debit";
}

// ---------------------------------------------------------------------------
// Progression
// ---------------------------------------------------------------------------

export interface LevelRef {
  id: string;
  level_number: number;
  name: string;
  xp_required: number;
}

export interface XpGrant {
  id: string;
  player_id: string;
  amount: number;
  description?: string;
  idempotency_key?: string;
  activity_id?: string;
  source_kind?: string;
  source_id?: string;
  created_by?: string;
  occurred_at: string;
  created_at: string;
}

export interface GrantXpParams {
  amount: number;
  description?: string;
}

export interface GrantXpResponse {
  grant: XpGrant;
  total_xp: number;
  level_number: number;
  levels_reached?: LevelRef[];
  replayed: boolean;
}

export interface PlayerProgress {
  player_id: string;
  total_xp: number;
  current_level?: LevelRef | null;
  next_level?: LevelRef | null;
  xp_to_next: number;
  progress_percent: number;
}

// ---------------------------------------------------------------------------
// Badges
// ---------------------------------------------------------------------------

export type BadgeTier = "bronze" | "silver" | "gold" | "platinum" | "diamond";

export interface Badge {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  description?: string;
  icon_url?: string;
  category?: string;
  tier?: BadgeTier;
  points_value: number;
  requirements?: JsonObject;
  is_active: boolean;
  is_secret: boolean;
  is_stackable: boolean;
  max_awards?: number;
  sort_order: number;
  version: number;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
}

export interface PlayerBadge {
  id: string;
  player_id: string;
  badge_id: string;
  badge?: Badge;
  earned_count: number;
  first_awarded_at: string;
  last_awarded_at: string;
  version: number;
  created_at: string;
}

export interface ListBadgesParams extends ListParams {
  tier?: BadgeTier;
  category?: string;
  active?: boolean;
}

export interface AwardBadgeParams {
  player_id: string;
}

export interface AwardBadgeResponse {
  award_id: string;
  status: string;
  replay: boolean;
  idempotency_key?: string;
  player_badge?: PlayerBadge;
}

// ---------------------------------------------------------------------------
// Missions
// ---------------------------------------------------------------------------

export interface Mission {
  id: string;
  slug: string;
  name: string;
  description?: string;
  type: string;
  status: string;
  target: number;
  criteria?: JsonObject;
  points_reward: number;
  xp_reward: number;
  badge_reward_id?: string;
  max_completions_per_player?: number;
  starts_at?: string;
  ends_at?: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface MissionSummary {
  id: string;
  slug: string;
  name: string;
  type: string;
  status: string;
  deleted?: boolean;
}

export interface MissionAttempt {
  id: string;
  mission_id: string;
  mission?: MissionSummary;
  player_id: string;
  period_key?: string;
  status: string;
  progress: number;
  target: number;
  started_at: string;
  completed_at?: string;
}

export interface ListMissionsParams extends ListParams {
  status?: string;
  type?: string;
}

export interface ListPlayerMissionsParams extends ListParams {
  status?: string;
}

export interface MissionProgressParams {
  player_id: string;
  /** 1 – 1 000 000. */
  increment: number;
}

export interface MissionProgressResponse {
  attempt: MissionAttempt;
  completed: boolean;
  duplicate: boolean;
}

export interface MissionPlayerParams {
  player_id: string;
}

// ---------------------------------------------------------------------------
// Streaks
// ---------------------------------------------------------------------------

export interface StreakMilestone {
  count: number;
  bonus_points?: number;
}

export interface Streak {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  description?: string;
  activity_key?: string;
  period: string;
  grace_periods: number;
  points_per_period: number;
  milestones?: StreakMilestone[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface PlayerStreak {
  id: string;
  streak_id: string;
  streak?: Streak;
  player_id: string;
  current_count: number;
  longest_count: number;
  run_started_at?: string;
  last_period_start?: string;
  broken_at?: string;
}

export interface ListStreaksParams extends ListParams {
  active?: boolean;
  period?: string;
}

export interface RecordStreakParams {
  player_id: string;
  occurred_at?: Timestamp;
}

export interface RecordStreakResponse {
  outcome: string;
  new_period: boolean;
  period_start: string;
  milestones_reached?: number[];
  player_streak: PlayerStreak;
}

// ---------------------------------------------------------------------------
// Rewards
// ---------------------------------------------------------------------------

export interface Reward {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  description?: string;
  type: string;
  status: string;
  value?: string;
  value_type?: string;
  points_cost: number;
  level_requirement?: number;
  badge_reward_id?: string;
  level_reward_id?: string;
  max_per_player?: number;
  max_redemptions?: number;
  stock_used: number;
  claim_ttl_days?: number;
  metadata?: JsonObject;
  is_active: boolean;
  start_at?: string;
  end_at?: string;
  created_at: string;
  updated_at: string;
}

export interface RewardClaim {
  id: string;
  tenant_id: string;
  reward_id: string;
  reward_slug?: string;
  reward_type?: string;
  player_id: string;
  status: string;
  points_cost: number;
  /** Redemption code, when the reward issues one. */
  code?: string;
  reject_reason?: string;
  claimed_at?: string;
  hold_expires_at?: string;
  expires_at?: string;
  redeemed_at?: string;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ListRewardsParams extends ListParams {
  status?: string;
  type?: string;
  is_active?: boolean;
}

export interface ClaimRewardParams {
  player_id: string;
}

// ---------------------------------------------------------------------------
// Leaderboards
// ---------------------------------------------------------------------------

export interface Leaderboard {
  id: string;
  tenant_id: string;
  program_id?: string;
  slug: string;
  name: string;
  description?: string;
  type: string;
  metric: string;
  reset_frequency: string;
  max_entries?: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface LeaderboardEntry {
  rank: number;
  score: number;
  player_id: string;
  external_id?: string;
  display_name?: string;
}

export interface LeaderboardEntriesPage extends CursorPage<LeaderboardEntry> {
  period_start?: string;
  period_end?: string;
}

export interface ListLeaderboardsParams extends ListParams {
  type?: string;
  active?: boolean;
}

export interface LeaderboardEntriesParams extends ListParams {
  /** "current" (default) or an RFC 3339 time inside the wanted period. */
  period?: string | Date;
}

export interface PlayerStandingParams {
  /** "current" (default) or an RFC 3339 time inside the wanted period. */
  period?: string | Date;
  /** Number of neighbours to return above and below the player. */
  around?: number;
}

export interface PlayerStanding {
  entry: LeaderboardEntry;
  neighbours?: LeaderboardEntry[];
  period_start?: string;
  period_end?: string;
}

// ---------------------------------------------------------------------------
// Rules
// ---------------------------------------------------------------------------

export interface SimulatedPlayer {
  external_id?: string;
  is_active?: boolean;
  level?: number;
  xp?: number;
  points?: number;
  attributes?: JsonObject;
}

export interface SimulateRulesParams {
  event_type: string;
  /** Simulate against an existing player... */
  player_id?: string;
  player_external_id?: string;
  /** ...or against a hypothetical one. */
  player?: SimulatedPlayer;
  properties?: JsonObject;
  context?: JsonObject;
  causation_depth?: number;
}

export interface ConditionTrace {
  path?: string;
  field?: string;
  source?: string;
  operator?: string;
  expected?: unknown;
  actual?: unknown;
  present?: boolean;
  result: boolean;
}

export interface SimulatedEffect {
  action_index: number;
  type: string;
  params?: JsonObject;
}

export interface SimulatedLimits {
  cooldown_seconds?: number;
  max_per_player?: number;
  max_per_player_per_day?: number;
  max_per_player_per_week?: number;
}

export interface SimulatedRule {
  rule_id: string;
  rule_version_id?: string;
  name: string;
  priority: number;
  status?: string;
  matched: boolean;
  condition_results?: ConditionTrace[];
  effects?: SimulatedEffect[];
  limits?: SimulatedLimits;
  error?: string;
}

export interface SimulateRulesResponse {
  outcome: string;
  reason?: string;
  player_id?: string;
  ruleset_generation: number;
  limits_enforced: boolean;
  rules: SimulatedRule[];
}

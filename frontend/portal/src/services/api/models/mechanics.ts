// mechanics models: badges, levels, missions, streaks, rewards, leaderboards.
// Field-for-field with the Go transport DTOs in
// backend/internal/modules/{badges,progression,missions,streaks,rewards,leaderboards}/internal/transport
// (see backend/api/docs/swagger.json). Ids are UUID strings; nullable fields
// are `| null`; omitempty fields are optional.
import type { CursorPage, CursorParams, ID } from './common';

type JsonObject = Record<string, unknown>;

// ==================== BADGES ====================

export type BadgeTier = 'bronze' | 'silver' | 'gold' | 'platinum' | 'diamond';
export type BadgeCategory = 'achievement' | 'milestone' | 'skill' | 'social' | 'exploration' | 'collection' | 'special' | 'seasonal';

/** BadgeResp */
export interface Badge {
  id: ID;
  tenant_id: ID;
  slug: string;
  name: string;
  description: string;
  icon_url: string;
  tier: BadgeTier;
  category: BadgeCategory;
  points_value: number;
  is_stackable: boolean;
  max_awards: number | null;
  requirements: JsonObject | null;
  is_active: boolean;
  is_secret: boolean;
  sort_order: number;
  version: number;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
}

/** CreateBadgeReq: points_value defaults to the tier's default, slug to a slug of name. */
export interface CreateBadgeData {
  name: string;
  slug?: string;
  description?: string;
  icon_url?: string;
  tier: BadgeTier;
  category: BadgeCategory;
  points_value?: number;
  is_stackable?: boolean;
  max_awards?: number;
  requirements?: JsonObject;
  is_active?: boolean;
  is_secret?: boolean;
  sort_order?: number;
}

/** UpdateBadgeReq (PATCH): omitted fields stay; max_awards/requirements accept null to clear. */
export interface UpdateBadgeData {
  name?: string;
  slug?: string;
  description?: string;
  icon_url?: string;
  tier?: BadgeTier;
  category?: BadgeCategory;
  points_value?: number;
  is_stackable?: boolean;
  max_awards?: number | null;
  requirements?: JsonObject | null;
  is_active?: boolean;
  is_secret?: boolean;
  sort_order?: number;
}

/** GET /badges query. */
export interface BadgeFilters extends CursorParams {
  tier?: BadgeTier;
  category?: BadgeCategory;
  active?: boolean;
}

/** PlayerBadgeResp */
export interface PlayerBadge {
  id: ID;
  player_id: ID;
  badge_id: ID;
  earned_count: number;
  first_awarded_at: string;
  last_awarded_at: string;
  version: number;
  created_at: string;
  badge?: Badge;
}

/** POST /badges/{badge_id}/award with { player_id }. */
export interface AwardBadgeData {
  badge_id: ID;
  player_id: ID;
}

/** AwardResp: the applied award (201) or its replay (200). */
export interface AwardBadgeResult {
  award_id: ID;
  idempotency_key: string;
  status: string;
  replay: boolean;
  player_badge: PlayerBadge;
}

// ==================== LEVELS (progression) ====================

/** LevelResp */
export interface Level {
  id: ID;
  level_number: number;
  name: string;
  description: string;
  xp_required: number;
  points_reward: number;
  badge_reward_id: ID | null;
  perks: JsonObject | null;
  icon_url: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

/** CreateLevelReq. xp_required must strictly increase with level_number. */
export interface CreateLevelData {
  level_number: number;
  name?: string;
  description?: string;
  xp_required: number;
  points_reward?: number;
  badge_reward_id?: ID;
  perks?: JsonObject;
  icon_url?: string;
  is_active?: boolean;
}

/** UpdateLevelReq (PATCH). badge_reward_id: null clears it. */
export interface UpdateLevelData {
  level_number?: number;
  name?: string;
  description?: string;
  xp_required?: number;
  points_reward?: number;
  badge_reward_id?: ID | null;
  perks?: JsonObject;
  icon_url?: string;
  is_active?: boolean;
}

/** GET /levels query (the ladder is returned whole). */
export interface LevelFilters extends CursorParams {
  active?: boolean;
}

/** LevelRefResp */
export interface LevelRef {
  id: ID;
  level_number: number;
  name: string;
  xp_required: number;
}

/** ProgressResp: GET /players/{id}/progress. */
export interface PlayerProgress {
  player_id: ID;
  total_xp: number;
  current_level: LevelRef | null;
  next_level: LevelRef | null;
  xp_to_next: number | null;
  progress_percent: number;
}

/** @deprecated Laravel name; the Go API returns PlayerProgress. */
export type PlayerLevel = PlayerProgress;

/** POST /players/{player_id}/xp with { amount, description }. */
export interface GrantXpData {
  player_id: ID;
  amount: number;
  description?: string;
}

/** GrantResp */
export interface XpGrant {
  id: ID;
  player_id: ID;
  idempotency_key: string;
  amount: number;
  description: string;
  source_kind: string;
  source_id: string;
  activity_id?: string;
  occurred_at: string;
  created_by?: string;
  created_at: string;
}

/** GrantXPResp */
export interface GrantXpResponse {
  grant: XpGrant;
  replayed: boolean;
  total_xp: number;
  level_number: number;
  levels_reached: LevelRef[];
}

// ==================== MISSIONS ====================

export type MissionType = 'one_time' | 'daily' | 'weekly' | 'repeating';
/** Lifecycle: draft -> active <-> paused -> expired -> archived (terminal). */
export type MissionStatus = 'draft' | 'active' | 'paused' | 'expired' | 'archived';
export type MissionAttemptStatus = 'in_progress' | 'completed' | 'expired' | 'abandoned';
/** @deprecated use MissionAttemptStatus */
export type PlayerMissionStatus = MissionAttemptStatus;

/** MissionResp */
export interface Mission {
  id: ID;
  slug: string;
  name: string;
  description: string;
  type: MissionType;
  status: MissionStatus;
  target: number;
  /** Opaque to missions; documents which activities count (evaluated by rules). */
  criteria: JsonObject;
  points_reward: number;
  xp_reward: number;
  badge_reward_id: ID | null;
  max_completions_per_player: number | null;
  starts_at: string | null;
  ends_at: string | null;
  version: number;
  created_at: string;
  updated_at: string;
}

/** CreateMissionReq: a mission is created as draft or active. */
export interface CreateMissionData {
  slug?: string;
  name: string;
  description?: string;
  type: MissionType;
  status?: 'draft' | 'active';
  target: number;
  criteria?: JsonObject;
  points_reward?: number;
  xp_reward?: number;
  badge_reward_id?: ID;
  max_completions_per_player?: number;
  starts_at?: string;
  ends_at?: string;
}

/** UpdateMissionReq (PATCH). type can only change while draft. */
export interface UpdateMissionData {
  slug?: string;
  name?: string;
  description?: string;
  type?: MissionType;
  status?: MissionStatus;
  target?: number;
  criteria?: JsonObject;
  points_reward?: number;
  xp_reward?: number;
  badge_reward_id?: ID;
  max_completions_per_player?: number;
  starts_at?: string;
  ends_at?: string;
}

/** GET /missions query. */
export interface MissionFilters extends CursorParams {
  status?: MissionStatus;
  type?: MissionType;
}

/** MissionSummaryResp */
export interface MissionSummary {
  id: ID;
  slug: string;
  name: string;
  type: MissionType;
  status: MissionStatus;
  deleted: boolean;
}

/** AttemptResp: one player's run at a mission for one period. */
export interface MissionAttempt {
  id: ID;
  mission_id: ID;
  player_id: ID;
  status: MissionAttemptStatus;
  progress: number;
  target: number;
  period_key: string;
  started_at: string;
  completed_at: string | null;
  mission?: MissionSummary;
}

/** A player's mission attempt (GET /players/{id}/missions). */
export type PlayerMission = MissionAttempt;

/** GET /missions/{id}/attempts query. */
export interface MissionAttemptFilters extends CursorParams {
  player_id?: ID;
  status?: MissionAttemptStatus;
}

/** POST /missions/{mission_id}/start with { player_id }. */
export interface StartMissionData {
  mission_id: ID;
  player_id: ID;
}

/** POST /missions/{mission_id}/progress with { player_id, increment }. */
export interface UpdateMissionProgressData {
  mission_id: ID;
  player_id: ID;
  increment: number;
}

/** POST /missions/{mission_id}/complete with { player_id }. */
export interface CompleteMissionData {
  mission_id: ID;
  player_id: ID;
}

/** ProgressResp (missions). duplicate: the Idempotency-Key was already applied. */
export interface MissionProgressResult {
  attempt: MissionAttempt | null;
  completed: boolean;
  duplicate: boolean;
}

// ==================== STREAKS ====================

export type StreakPeriod = 'daily' | 'weekly' | 'monthly';
/** @deprecated use StreakPeriod */
export type StreakType = StreakPeriod;

/** MilestoneDTO */
export interface StreakMilestone {
  count: number;
  bonus_points: number;
}

/** StreakResp */
export interface Streak {
  id: ID;
  tenant_id: ID;
  slug: string;
  name: string;
  description: string;
  activity_key: string;
  period: StreakPeriod;
  grace_periods: number;
  points_per_period: number;
  milestones: StreakMilestone[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

/** CreateReq (streaks). period is immutable after create. */
export interface CreateStreakData {
  slug?: string;
  name: string;
  description?: string;
  activity_key: string;
  period: StreakPeriod;
  grace_periods?: number;
  points_per_period?: number;
  milestones?: StreakMilestone[];
  is_active?: boolean;
}

/** UpdateReq (streaks, PATCH): no period. */
export interface UpdateStreakData {
  slug?: string;
  name?: string;
  description?: string;
  activity_key?: string;
  grace_periods?: number;
  points_per_period?: number;
  milestones?: StreakMilestone[];
  is_active?: boolean;
}

/** GET /streaks query. */
export interface StreakFilters extends CursorParams {
  period?: StreakPeriod;
  active?: boolean;
}

/** PlayerStreakResp */
export interface PlayerStreak {
  id: ID;
  player_id: ID;
  streak_id: ID;
  current_count: number;
  longest_count: number;
  last_period_start: string | null;
  run_started_at: string | null;
  broken_at: string | null;
  streak?: Streak;
}

/** POST /streaks/{streak_id}/record with { player_id, occurred_at? }. */
export interface RecordStreakActivityData {
  streak_id: ID;
  player_id: ID;
  /** RFC 3339; defaults to now and decides the period bucket. */
  occurred_at?: string;
}

/** recorded (new period), noop (period already recorded) or duplicate (key replay). */
export type RecordStreakOutcome = 'recorded' | 'noop' | 'duplicate';

/** RecordResp */
export interface RecordStreakActivityResponse {
  outcome: RecordStreakOutcome;
  new_period: boolean;
  period_start: string;
  milestones_reached: number[];
  player_streak: PlayerStreak;
}

/** POST /streaks/{streak_id}/players/{player_id}/reset */
export interface ResetStreakData {
  streak_id: ID;
  player_id: ID;
}

// ==================== LEADERBOARDS ====================

export type LeaderboardType = 'points' | 'badges' | 'missions' | 'xp';
/** points: earned|net|balance; xp: earned|balance; badges/missions: count. */
export type LeaderboardMetric = 'earned' | 'net' | 'balance' | 'count';
export type ResetFrequency = 'never' | 'daily' | 'weekly' | 'monthly';

/** LeaderboardResp */
export interface Leaderboard {
  id: ID;
  tenant_id: ID;
  slug: string;
  name: string;
  description: string;
  type: LeaderboardType;
  metric: LeaderboardMetric;
  reset_frequency: ResetFrequency;
  program_id: ID | null;
  max_entries: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

/** CreateReq (leaderboards). balance requires reset_frequency never. */
export interface CreateLeaderboardData {
  name: string;
  slug?: string;
  description?: string;
  type: LeaderboardType;
  metric?: LeaderboardMetric;
  reset_frequency?: ResetFrequency;
  program_id?: ID;
  max_entries?: number;
  is_active?: boolean;
}

/** UpdateReq (leaderboards, PATCH): type/metric/reset_frequency are immutable. */
export interface UpdateLeaderboardData {
  name?: string;
  slug?: string;
  description?: string;
  max_entries?: number;
  is_active?: boolean;
}

/** GET /leaderboards query. */
export interface LeaderboardFilters extends CursorParams {
  type?: LeaderboardType;
  active?: boolean;
}

/** EntryResp */
export interface LeaderboardEntry {
  rank: number;
  player_id: ID;
  external_id: string;
  display_name: string;
  score: number;
}

/** GET /leaderboards/{id}/entries query. period: "current" (default) or an RFC 3339 time. */
export interface LeaderboardEntriesParams extends CursorParams {
  period?: string;
}

/** EntriesResp */
export interface LeaderboardEntriesPage extends CursorPage<LeaderboardEntry> {
  period_start: string;
  period_end: string | null;
}

/** PlayerRankResp */
export interface PlayerRank {
  entry: LeaderboardEntry;
  neighbours: LeaderboardEntry[];
  period_start: string;
  period_end: string | null;
}

/** RebuildResp */
export interface RebuildLeaderboardResult {
  periods: number;
  entries: number;
}

// ==================== REWARDS ====================

export type RewardType = 'points' | 'discount' | 'item' | 'badge' | 'level' | 'custom';
export type RewardStatus = 'draft' | 'active' | 'paused' | 'expired' | 'depleted';
export type RewardValueType = 'percentage' | 'fixed';

/** RewardResp. value is a decimal string such as "10.00". */
export interface Reward {
  id: ID;
  tenant_id: ID;
  name: string;
  slug: string;
  description: string;
  type: RewardType;
  status: RewardStatus;
  points_cost: number;
  value: string | null;
  value_type: RewardValueType | null;
  badge_reward_id: ID | null;
  level_reward_id: ID | null;
  max_redemptions: number | null;
  max_per_player: number | null;
  stock_used: number;
  claim_ttl_days: number | null;
  start_at: string | null;
  end_at: string | null;
  level_requirement: number | null;
  is_active: boolean;
  metadata: JsonObject | null;
  created_at: string;
  updated_at: string;
}

/** CreateRewardReq */
export interface CreateRewardData {
  name: string;
  slug?: string;
  description?: string;
  type?: RewardType;
  status?: RewardStatus;
  points_cost: number;
  /** Decimal string, at most 2 decimals, e.g. "10" or "12.50". */
  value?: string;
  value_type?: RewardValueType;
  badge_reward_id?: ID;
  level_reward_id?: ID;
  max_redemptions?: number;
  max_per_player?: number;
  claim_ttl_days?: number;
  start_at?: string;
  end_at?: string;
  level_requirement?: number;
  is_active?: boolean;
  metadata?: JsonObject;
}

/** UpdateRewardReq (PATCH). */
export type UpdateRewardData = Partial<CreateRewardData>;

/** GET /rewards query. */
export interface RewardFilters extends CursorParams {
  status?: RewardStatus;
  type?: RewardType;
  is_active?: boolean;
}

export type RewardClaimStatus =
  | 'pending_payment'
  | 'claimed'
  | 'rejected'
  | 'redeemed'
  | 'expired'
  | 'cancelled'
  | 'refund_pending'
  | 'refunded';

/** ClaimResp */
export interface RewardClaim {
  id: ID;
  tenant_id: ID;
  player_id: ID;
  reward_id: ID;
  reward_slug: string;
  reward_type: RewardType;
  status: RewardClaimStatus;
  points_cost: number;
  reject_reason?: string;
  code: string | null;
  hold_expires_at: string | null;
  claimed_at: string | null;
  redeemed_at: string | null;
  expires_at: string | null;
  cancelled_at: string | null;
  created_at: string;
  updated_at: string;
}

/** A player's reward claim (GET /players/{id}/reward-claims). */
export type PlayerReward = RewardClaim;
/** @deprecated use RewardClaimStatus */
export type PlayerRewardStatus = RewardClaimStatus;

/** POST /rewards/{reward_id}/claim with { player_id }. */
export interface ClaimRewardData {
  reward_id: ID;
  player_id: ID;
}

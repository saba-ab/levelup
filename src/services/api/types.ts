// Common API types matching Laravel backend

// Pagination (Laravel format)
export interface PaginatedResponse<T> {
  data: T[];
  meta: {
    current_page: number;
    from: number;
    last_page: number;
    per_page: number;
    to: number;
    total: number;
  };
  links: {
    first: string;
    last: string;
    prev: string | null;
    next: string | null;
  };
}

export interface PaginationParams {
  page?: number;
  per_page?: number;
}

// Timestamps
export interface Timestamps {
  created_at: string;
  updated_at: string;
}

// ==================== AUTH ====================

export interface AuthUser {
  id: number;
  name: string;
  email: string;
  email_verified_at?: string;
  tenant_id?: number;
  created_at?: string;
  updated_at?: string;
}

export interface AuthResponse {
  access_token: string;
  token_type: 'Bearer';
  expires_in: number;
  user: AuthUser;
}

export interface RegisterData {
  tenant_name: string;
  email: string;
  password: string;
  password_confirmation: string;
}

export interface LoginData {
  email: string;
  password: string;
}

// ==================== USERS ====================

export interface User extends Timestamps {
  id: number;
  name: string;
  email: string;
  email_verified_at?: string;
  role?: string;
}

export interface CreateUserData {
  name: string;
  email: string;
  password: string;
  password_confirmation: string;
  role?: string;
}

export interface UpdateUserData {
  name?: string;
  email?: string;
  password?: string;
  password_confirmation?: string;
  role?: string;
}

// ==================== PLAYERS ====================

export interface Player extends Timestamps {
  id: number;
  tenant_id: number;
  external_id: string;
  email: string;
  first_name: string;
  last_name: string;
  display_name: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
  is_active: boolean;
  created_by: number;
}

export interface CreatePlayerData {
  external_id: string;
  email?: string;
  first_name?: string;
  last_name?: string;
  display_name?: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdatePlayerData {
  email?: string;
  first_name?: string;
  last_name?: string;
  display_name?: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
  is_active?: boolean;
}

export interface PlayerFilters extends PaginationParams {
  search?: string;
  is_active?: boolean;
}

// ==================== BADGES ====================

export type BadgeTier = 'bronze' | 'silver' | 'gold' | 'platinum' | 'diamond';
export type BadgeCategory = 'achievement' | 'milestone' | 'skill' | 'social' | 'exploration' | 'collection' | 'special' | 'seasonal';

export interface Badge extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  slug: string;
  description?: string;
  image_url?: string;
  tier: BadgeTier;
  category: BadgeCategory;
  points_value: number;
  is_stackable: boolean;
  max_awards?: number;
  is_active: boolean;
  is_secret: boolean;
  requirements?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface CreateBadgeData {
  name: string;
  description?: string;
  image_url?: string;
  tier?: BadgeTier;
  category?: BadgeCategory;
  points_value?: number;
  is_stackable?: boolean;
  max_awards?: number;
  is_active?: boolean;
  is_secret?: boolean;
  requirements?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface UpdateBadgeData {
  name?: string;
  description?: string;
  image_url?: string;
  tier?: BadgeTier;
  category?: BadgeCategory;
  points_value?: number;
  is_stackable?: boolean;
  max_awards?: number;
  is_active?: boolean;
  is_secret?: boolean;
  requirements?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface PlayerBadge extends Timestamps {
  id: number;
  player_id: number;
  badge_id: number;
  awarded_at: string;
  awarded_by?: number;
  earned_count: number;
  badge?: Badge;
  metadata?: Record<string, unknown>;
}

export interface AwardBadgeData {
  player_id: number;
  badge_id: number;
  metadata?: Record<string, unknown>;
}

// ==================== LEVELS ====================

export interface Level extends Timestamps {
  id: number;
  tenant_id: number;
  number: number;
  name?: string;
  description?: string;
  xp_required: number;
  points_reward: number;
  badge_reward_id?: number;
  badge_reward?: Badge;
  icon_url?: string;
  color?: string;
  metadata?: Record<string, unknown>;
}

export interface CreateLevelData {
  number: number;
  name?: string;
  description?: string;
  xp_required: number;
  points_reward?: number;
  badge_reward_id?: number;
  icon_url?: string;
  color?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateLevelData {
  number?: number;
  name?: string;
  description?: string;
  xp_required?: number;
  points_reward?: number;
  badge_reward_id?: number;
  icon_url?: string;
  color?: string;
  metadata?: Record<string, unknown>;
}

export interface PlayerLevel extends Timestamps {
  id: number;
  player_id: number;
  level_id: number;
  level?: Level;
  current_xp: number;
  total_xp: number;
  level_reached_at: string;
  xp_to_next_level?: number;
  progress_percentage?: number;
}

export interface GrantXpData {
  player_id: number;
  amount: number;
  source?: string;
  source_id?: number;
}

export interface GrantXpResponse {
  player_level: PlayerLevel;
  leveled_up: boolean;
  levels_gained: number;
}

// ==================== MISSIONS ====================

export type MissionType = 'one_time' | 'daily' | 'weekly' | 'monthly' | 'recurring' | 'event';
export type MissionStatus = 'draft' | 'active' | 'paused' | 'completed' | 'expired' | 'cancelled';
export type PlayerMissionStatus = 'not_started' | 'in_progress' | 'completed' | 'failed' | 'expired';

export interface MissionObjective {
  target: number;
  label: string;
}

export interface Mission extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  type: MissionType;
  status: MissionStatus;
  objectives?: Record<string, MissionObjective>;
  points_reward: number;
  xp_reward: number;
  badge_reward_id?: number;
  badge_reward?: Badge;
  start_date?: string;
  end_date?: string;
  max_completions?: number;
  cooldown_hours?: number;
  is_active: boolean;
  is_secret: boolean;
}

export interface CreateMissionData {
  name: string;
  description?: string;
  type: MissionType;
  status: MissionStatus;
  objectives?: Record<string, MissionObjective>;
  points_reward?: number;
  xp_reward?: number;
  badge_reward_id?: number;
  start_date?: string;
  end_date?: string;
  max_completions?: number;
  cooldown_hours?: number;
  is_active?: boolean;
  is_secret?: boolean;
}

export interface PlayerMission extends Timestamps {
  id: number;
  player_id: number;
  mission_id: number;
  mission?: Mission;
  objectives?: Record<string, number>;
  status: PlayerMissionStatus;
  started_at?: string;
  completed_at?: string;
}

export interface StartMissionData {
  player_id: number;
  mission_id: number;
}

export interface UpdateMissionProgressData {
  objectives: Record<string, number>;
}

// ==================== STREAKS ====================

export type StreakType = 'daily' | 'weekly' | 'monthly';

export interface Streak extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  type: StreakType;
  activity_key: string;
  points_per_day: number;
  bonus_points: number;
  bonus_milestones?: number[];
  max_streak_days?: number;
  is_active: boolean;
}

export interface CreateStreakData {
  name: string;
  description?: string;
  type: StreakType;
  activity_key: string;
  points_per_day?: number;
  bonus_points?: number;
  bonus_milestones?: number[];
  max_streak_days?: number;
  is_active?: boolean;
}

export interface PlayerStreak extends Timestamps {
  id: number;
  player_id: number;
  streak_id: number;
  streak?: Streak;
  current_count: number;
  longest_count: number;
  last_activity_at: string;
}

export interface RecordStreakActivityData {
  player_id: number;
  activity_key: string;
}

export interface RecordStreakActivityResponse {
  player_streak: PlayerStreak;
  was_continued: boolean;
  was_reset: boolean;
  milestone_reached?: number;
}

// ==================== LEADERBOARDS ====================

export type LeaderboardType = 'points' | 'badges' | 'missions' | 'custom';
export type LeaderboardScope = 'global' | 'segment' | 'program';
export type ResetFrequency = 'daily' | 'weekly' | 'monthly' | 'never';

export interface Leaderboard extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  type: LeaderboardType;
  scope: LeaderboardScope;
  scope_id?: number;
  reset_frequency?: ResetFrequency;
  is_active: boolean;
}

export interface LeaderboardEntry {
  rank: number;
  player_id: number;
  player: Pick<Player, 'id' | 'display_name' | 'avatar_url'>;
  score: number;
  change: number;
}

export interface CreateLeaderboardData {
  name: string;
  description?: string;
  type: LeaderboardType;
  scope: LeaderboardScope;
  scope_id?: number;
  reset_frequency?: ResetFrequency;
  is_active?: boolean;
}

// ==================== REWARDS ====================

export type RewardType = 'points' | 'discount' | 'item' | 'badge' | 'level' | 'custom';
export type RewardStatus = 'draft' | 'active' | 'paused' | 'expired' | 'depleted';
export type PlayerRewardStatus = 'claimed' | 'redeemed' | 'expired';

export interface Reward extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  type: RewardType;
  status: RewardStatus;
  points_cost: number;
  value?: number;
  value_type?: string;
  badge_reward_id?: number;
  level_reward_id?: number;
  max_redemptions?: number;
  max_redemptions_per_player?: number;
  start_date?: string;
  end_date?: string;
  level_requirement?: number;
  is_active: boolean;
}

export interface CreateRewardData {
  name: string;
  description?: string;
  type: RewardType;
  status: RewardStatus;
  points_cost?: number;
  value?: number;
  value_type?: string;
  badge_reward_id?: number;
  level_reward_id?: number;
  max_redemptions?: number;
  max_redemptions_per_player?: number;
  start_date?: string;
  end_date?: string;
  level_requirement?: number;
  is_active?: boolean;
}

export interface PlayerReward extends Timestamps {
  id: number;
  player_id: number;
  reward_id: number;
  reward?: Reward;
  status: PlayerRewardStatus;
  claimed_at?: string;
  redeemed_at?: string;
}

export interface ClaimRewardData {
  player_id: number;
  reward_id: number;
}

// ==================== WALLETS ====================

export interface Wallet extends Timestamps {
  id: number;
  player_id: number;
  balance: number;
  lifetime_earned: number;
  lifetime_spent: number;
}

export type WalletTransactionType = 'credit' | 'debit' | 'transfer_in' | 'transfer_out' | 'mission_reward' | 'level_bonus' | 'refund' | 'reward_purchase' | 'penalty';

export interface WalletTransaction extends Timestamps {
  id: number;
  wallet_id: number;
  player_id: number;
  amount: number;
  type: WalletTransactionType;
  description?: string;
  reference_type?: string;
  reference_id?: number;
  balance_before: number;
  balance_after: number;
  metadata?: Record<string, unknown>;
}

export interface CreditWalletData {
  player_id: number;
  amount: number;
  description?: string;
  type?: 'credit' | 'transfer_in' | 'mission_reward' | 'level_bonus' | 'refund';
  reference_type?: string;
  reference_id?: number;
  metadata?: Record<string, unknown>;
}

export interface DebitWalletData {
  player_id: number;
  amount: number;
  description?: string;
  type?: 'debit' | 'transfer_out' | 'reward_purchase' | 'penalty';
  reference_type?: string;
  reference_id?: number;
  metadata?: Record<string, unknown>;
}

export interface TransferPointsData {
  from_player_id: number;
  to_player_id: number;
  amount: number;
  description?: string;
}

export interface WalletTransactionFilters extends PaginationParams {
  type?: WalletTransactionType;
  date_from?: string;
  date_to?: string;
}

// ==================== TRIGGER EVENTS ====================

export interface TriggerEventProperty {
  name: string;
  type: 'string' | 'number' | 'boolean' | 'array' | 'object';
  required?: boolean;
  description?: string;
}

export interface TriggerEvent extends Timestamps {
  id: number;
  name: string;
  slug: string;
  description?: string;
  is_predefined: boolean;
  is_active: boolean;
  metadata?: {
    icon?: string;
    category?: string;
    properties?: TriggerEventProperty[];
    [key: string]: unknown;
  };
}

export interface CreateTriggerEventData {
  name: string;
  slug?: string;
  description?: string;
  is_predefined?: boolean;
  is_active?: boolean;
  metadata?: {
    icon?: string;
    category?: string;
    properties?: TriggerEventProperty[];
    [key: string]: unknown;
  };
}

export interface UpdateTriggerEventData {
  name?: string;
  slug?: string;
  description?: string;
  is_active?: boolean;
  metadata?: {
    icon?: string;
    category?: string;
    properties?: TriggerEventProperty[];
    [key: string]: unknown;
  };
}

export interface TriggerEventFilters extends PaginationParams {
  search?: string;
  is_predefined?: boolean;
  is_active?: boolean;
}

// ==================== RULES ====================

export interface RuleConditions {
  [key: string]: { min?: number; max?: number; eq?: unknown };
}

export interface RuleActions {
  grant_points?: number;
  grant_xp?: number;
  award_badge_id?: number;
  start_mission_id?: number;
}

export interface Rule extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  trigger_event: string;
  is_active: boolean;
  priority: number;
  conditions?: RuleConditions;
  actions?: RuleActions;
}

export interface CreateRuleData {
  name: string;
  description?: string;
  trigger_event: string;
  is_active?: boolean;
  priority?: number;
  conditions?: RuleConditions;
  actions?: RuleActions;
}

export interface RuleVersion extends Timestamps {
  id: number;
  rule_id: number;
  version: number;
  name: string;
  description?: string;
  trigger_event: string;
  conditions?: RuleConditions;
  actions?: RuleActions;
}

export interface ExecuteRulesData {
  player_id: number;
  trigger_event: string;
  trigger_data?: Record<string, unknown>;
}

export interface RuleExecution extends Timestamps {
  id: number;
  rule_id: number;
  player_id: number;
  trigger_event: string;
  trigger_data?: Record<string, unknown>;
  status: 'success' | 'failed' | 'skipped';
  result?: Record<string, unknown>;
}

export interface RuleExecutionFilters extends PaginationParams {
  rule_id?: number;
  player_id?: number;
  status?: 'success' | 'failed' | 'skipped';
  date_from?: string;
  date_to?: string;
}

// ==================== PROGRAMS ====================

export type ProgramStatus = 'draft' | 'active' | 'paused' | 'ended';

export interface ProgramSettings {
  allow_public_signup: boolean;
  require_email_verification: boolean;
  default_level_id?: number;
  welcome_points?: number;
}

export interface ProgramMechanics {
  points_enabled: boolean;
  badges_enabled: boolean;
  levels_enabled: boolean;
  missions_enabled: boolean;
  streaks_enabled: boolean;
  leaderboards_enabled: boolean;
  rewards_enabled: boolean;
}

export interface Program extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  status: ProgramStatus;
  start_date?: string;
  end_date?: string;
  settings?: ProgramSettings;
  mechanics?: ProgramMechanics;
  metadata?: Record<string, unknown>;
  player_count: number;
}

export interface CreateProgramData {
  name: string;
  description?: string;
  start_date?: string;
  end_date?: string;
  settings?: Partial<ProgramSettings>;
  mechanics?: Partial<ProgramMechanics>;
  metadata?: Record<string, unknown>;
}

export interface UpdateProgramData {
  name?: string;
  description?: string;
  status?: ProgramStatus;
  start_date?: string;
  end_date?: string;
  settings?: Partial<ProgramSettings>;
  mechanics?: Partial<ProgramMechanics>;
  metadata?: Record<string, unknown>;
}

export interface ProgramFilters extends PaginationParams {
  search?: string;
  status?: ProgramStatus;
}

// ==================== SEGMENTS ====================

export type SegmentType = 'static' | 'dynamic';

export interface SegmentRule {
  field: string;
  operator: 'eq' | 'neq' | 'gt' | 'gte' | 'lt' | 'lte' | 'contains' | 'in';
  value: unknown;
}

export interface Segment extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  type: SegmentType;
  rules?: SegmentRule[];
  player_count: number;
}

export interface CreateSegmentData {
  name: string;
  description?: string;
  type: SegmentType;
  rules?: SegmentRule[];
}

// ==================== FILTER TYPES ====================

export interface MechanicsFilters extends PaginationParams {
  search?: string;
  is_active?: boolean;
  category?: string;
}

// ==================== STATS ====================

export interface PlayerStats {
  total_points: number;
  badges_earned: number;
  missions_completed: number;
  current_streak: number;
  rank: number;
}

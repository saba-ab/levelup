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
  description?: string;
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
  tier: BadgeTier;
  category: BadgeCategory;
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
  badge?: Badge;
  earned_count: number;
  metadata?: Record<string, unknown>;
}

export interface AwardBadgeData {
  player_id: number;
  badge_id: number;
  metadata?: Record<string, unknown>;
}

export interface RevokeBadgeData {
  player_id: number;
  badge_id: number;
}

// ==================== LEVELS ====================

export interface Level extends Timestamps {
  id: number;
  tenant_id: number;
  number: number;
  name: string;
  description?: string;
  xp_required: number;
  points_reward: number;
  badge_reward_id?: number;
  badge_reward?: Badge;
  icon_url?: string;
  color?: string;
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
}

export interface PlayerLevel extends Timestamps {
  id: number;
  player_id: number;
  level_id: number;
  level?: Level;
  current_xp: number;
  total_xp: number;
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

export type MissionType = 'one_time' | 'recurring' | 'challenge';

export interface MissionRequirement {
  type: string;
  target: number;
  description: string;
}

export interface Mission extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  type: MissionType;
  points_reward: number;
  badge_reward_id?: number;
  badge_reward?: Badge;
  requirements: MissionRequirement[];
  start_date?: string;
  end_date?: string;
  is_active: boolean;
}

export interface CreateMissionData {
  name: string;
  description?: string;
  type: MissionType;
  points_reward: number;
  badge_reward_id?: number;
  requirements?: MissionRequirement[];
  start_date?: string;
  end_date?: string;
  is_active?: boolean;
}

export interface PlayerMission extends Timestamps {
  id: number;
  player_id: number;
  mission_id: number;
  mission?: Mission;
  progress: number;
  status: 'in_progress' | 'completed' | 'failed';
  completed_at?: string;
}

// ==================== STREAKS ====================

export type StreakFrequency = 'daily' | 'weekly';

export interface StreakMilestone {
  days: number;
  reward_points: number;
  badge_id?: number;
}

export interface Streak extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  activity_type: string;
  target_frequency: StreakFrequency;
  reward_per_streak: number;
  milestone_rewards?: StreakMilestone[];
  is_active: boolean;
}

export interface CreateStreakData {
  name: string;
  description?: string;
  activity_type: string;
  target_frequency: StreakFrequency;
  reward_per_streak: number;
  milestone_rewards?: StreakMilestone[];
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

export type RewardType = 'digital' | 'physical' | 'discount' | 'experience';

export interface Reward extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  image_url?: string;
  points_cost: number;
  stock?: number;
  type: RewardType;
  redemption_instructions?: string;
  is_active: boolean;
}

export interface CreateRewardData {
  name: string;
  description?: string;
  image_url?: string;
  points_cost: number;
  stock?: number;
  type: RewardType;
  redemption_instructions?: string;
  is_active?: boolean;
}

export interface RewardRedemption extends Timestamps {
  id: number;
  player_id: number;
  reward_id: number;
  reward?: Reward;
  status: 'pending' | 'fulfilled' | 'cancelled';
  fulfilled_at?: string;
}

// ==================== POINT WALLETS ====================

export interface PointWallet extends Timestamps {
  id: number;
  tenant_id: number;
  player_id: number;
  name: string;
  description?: string;
  balance: number;
  lifetime_earned: number;
  currency?: string;
}

export interface PointTransaction extends Timestamps {
  id: number;
  wallet_id: number;
  amount: number;
  type: 'credit' | 'debit';
  reason: string;
  reference_type?: string;
  reference_id?: number;
}

export interface AwardPointsData {
  player_id: number;
  wallet_id?: number;
  amount: number;
  reason: string;
  reference_type?: string;
  reference_id?: number;
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
  player_count: number;
}

export interface CreateProgramData {
  name: string;
  description?: string;
  start_date?: string;
  end_date?: string;
  settings?: Partial<ProgramSettings>;
  mechanics?: Partial<ProgramMechanics>;
}

export interface UpdateProgramData {
  name?: string;
  description?: string;
  status?: ProgramStatus;
  start_date?: string;
  end_date?: string;
  settings?: Partial<ProgramSettings>;
  mechanics?: Partial<ProgramMechanics>;
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

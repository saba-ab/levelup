// Common API types and interfaces

// Pagination
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
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
}

// Timestamps
export interface Timestamps {
  created_at: string;
  updated_at: string;
}

// Player types
export interface Player extends Timestamps {
  id: string;
  external_id: string;
  email: string;
  name: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
  total_points: number;
  level_id: string;
  level?: Level;
  badges?: Badge[];
  segments?: Segment[];
}

export interface PlayerStats {
  total_points: number;
  badges_earned: number;
  missions_completed: number;
  current_streak: number;
  rank: number;
}

export interface CreatePlayerData {
  external_id: string;
  email: string;
  name: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdatePlayerData {
  email?: string;
  name?: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
}

// Points types
export interface PointWallet extends Timestamps {
  id: string;
  name: string;
  description?: string;
  balance: number;
  lifetime_earned: number;
  currency?: string;
}

export interface PointTransaction extends Timestamps {
  id: string;
  wallet_id: string;
  amount: number;
  type: 'credit' | 'debit';
  reason: string;
  reference_type?: string;
  reference_id?: string;
}

export interface AwardPointsData {
  player_id: string;
  wallet_id: string;
  amount: number;
  reason: string;
  reference_type?: string;
  reference_id?: string;
}

// Badge types
export interface Badge extends Timestamps {
  id: string;
  name: string;
  description?: string;
  image_url?: string;
  criteria?: string;
  points_value: number;
  is_active: boolean;
  category?: string;
}

export interface CreateBadgeData {
  name: string;
  description?: string;
  image_url?: string;
  criteria?: string;
  points_value?: number;
  category?: string;
}

export interface PlayerBadge extends Timestamps {
  id: string;
  player_id: string;
  badge_id: string;
  badge: Badge;
  awarded_at: string;
  awarded_reason?: string;
}

// Level types
export interface Level extends Timestamps {
  id: string;
  name: string;
  description?: string;
  min_points: number;
  max_points?: number;
  icon_url?: string;
  benefits?: string[];
  order: number;
}

export interface CreateLevelData {
  name: string;
  description?: string;
  min_points: number;
  max_points?: number;
  icon_url?: string;
  benefits?: string[];
}

// Mission types
export interface Mission extends Timestamps {
  id: string;
  name: string;
  description?: string;
  type: 'one_time' | 'recurring' | 'challenge';
  points_reward: number;
  badge_reward_id?: string;
  badge_reward?: Badge;
  requirements: MissionRequirement[];
  start_date?: string;
  end_date?: string;
  is_active: boolean;
}

export interface MissionRequirement {
  type: string;
  target: number;
  description: string;
}

export interface CreateMissionData {
  name: string;
  description?: string;
  type: 'one_time' | 'recurring' | 'challenge';
  points_reward: number;
  badge_reward_id?: string;
  requirements: MissionRequirement[];
  start_date?: string;
  end_date?: string;
}

export interface PlayerMission extends Timestamps {
  id: string;
  player_id: string;
  mission_id: string;
  mission: Mission;
  progress: number;
  status: 'in_progress' | 'completed' | 'failed';
  completed_at?: string;
}

// Streak types
export interface Streak extends Timestamps {
  id: string;
  name: string;
  description?: string;
  activity_type: string;
  target_frequency: 'daily' | 'weekly';
  reward_per_streak: number;
  milestone_rewards?: StreakMilestone[];
  is_active: boolean;
}

export interface StreakMilestone {
  days: number;
  reward_points: number;
  badge_id?: string;
}

export interface CreateStreakData {
  name: string;
  description?: string;
  activity_type: string;
  target_frequency: 'daily' | 'weekly';
  reward_per_streak: number;
  milestone_rewards?: StreakMilestone[];
}

export interface PlayerStreak extends Timestamps {
  id: string;
  player_id: string;
  streak_id: string;
  streak: Streak;
  current_count: number;
  longest_count: number;
  last_activity_at: string;
}

// Leaderboard types
export interface Leaderboard extends Timestamps {
  id: string;
  name: string;
  description?: string;
  type: 'points' | 'badges' | 'missions' | 'custom';
  scope: 'global' | 'segment' | 'program';
  scope_id?: string;
  reset_frequency?: 'daily' | 'weekly' | 'monthly' | 'never';
  is_active: boolean;
}

export interface LeaderboardEntry {
  rank: number;
  player_id: string;
  player: Pick<Player, 'id' | 'name' | 'avatar_url'>;
  score: number;
  change: number; // Rank change since last period
}

export interface CreateLeaderboardData {
  name: string;
  description?: string;
  type: 'points' | 'badges' | 'missions' | 'custom';
  scope: 'global' | 'segment' | 'program';
  scope_id?: string;
  reset_frequency?: 'daily' | 'weekly' | 'monthly' | 'never';
}

// Reward types
export interface Reward extends Timestamps {
  id: string;
  name: string;
  description?: string;
  image_url?: string;
  points_cost: number;
  stock?: number;
  type: 'digital' | 'physical' | 'discount' | 'experience';
  redemption_instructions?: string;
  is_active: boolean;
}

export interface CreateRewardData {
  name: string;
  description?: string;
  image_url?: string;
  points_cost: number;
  stock?: number;
  type: 'digital' | 'physical' | 'discount' | 'experience';
  redemption_instructions?: string;
}

export interface RewardRedemption extends Timestamps {
  id: string;
  player_id: string;
  reward_id: string;
  reward: Reward;
  status: 'pending' | 'fulfilled' | 'cancelled';
  fulfilled_at?: string;
}

// Program types
export interface Program extends Timestamps {
  id: string;
  name: string;
  description?: string;
  status: 'draft' | 'active' | 'paused' | 'ended';
  start_date?: string;
  end_date?: string;
  settings: ProgramSettings;
  mechanics: ProgramMechanics;
  player_count: number;
}

export interface ProgramSettings {
  allow_public_signup: boolean;
  require_email_verification: boolean;
  default_level_id?: string;
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
  status?: 'draft' | 'active' | 'paused' | 'ended';
  start_date?: string;
  end_date?: string;
  settings?: Partial<ProgramSettings>;
  mechanics?: Partial<ProgramMechanics>;
}

// Segment types
export interface Segment extends Timestamps {
  id: string;
  name: string;
  description?: string;
  type: 'static' | 'dynamic';
  rules?: SegmentRule[];
  player_count: number;
}

export interface SegmentRule {
  field: string;
  operator: 'eq' | 'neq' | 'gt' | 'gte' | 'lt' | 'lte' | 'contains' | 'in';
  value: unknown;
}

export interface CreateSegmentData {
  name: string;
  description?: string;
  type: 'static' | 'dynamic';
  rules?: SegmentRule[];
}

// Filter params for list endpoints
export interface PlayerFilters extends PaginationParams {
  search?: string;
  segment_id?: string;
  level_id?: string;
  has_badge?: string;
  min_points?: number;
  max_points?: number;
}

export interface MechanicsFilters extends PaginationParams {
  search?: string;
  is_active?: boolean;
  category?: string;
}

export interface ProgramFilters extends PaginationParams {
  search?: string;
  status?: 'draft' | 'active' | 'paused' | 'ended';
}
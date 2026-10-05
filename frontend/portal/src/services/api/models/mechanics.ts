// mechanics models (Go API contract, see docs/rewrite and backend/api/docs/swagger.json)
import type { Timestamps } from './common';
import type { Player } from './players';

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

import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  Badge,
  CreateBadgeData,
  PlayerBadge,
  AwardBadgeData,
  RevokeBadgeData,
  Level,
  CreateLevelData,
  PlayerLevel,
  GrantXpData,
  GrantXpResponse,
  Mission,
  CreateMissionData,
  Streak,
  CreateStreakData,
  Leaderboard,
  LeaderboardEntry,
  CreateLeaderboardData,
  Reward,
  CreateRewardData,
  RewardRedemption,
  PointWallet,
  PaginatedResponse,
  MechanicsFilters,
} from './types';

export function useMechanicsService() {
  const api = useApi();

  // ==================== BADGES ====================
  
  const listBadges = useCallback(async (filters?: MechanicsFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<Badge>>(`/badges${query ? `?${query}` : ''}`);
  }, [api]);

  const getBadge = useCallback(async (badgeId: number) => {
    return api.get<Badge>(`/badges/${badgeId}`);
  }, [api]);

  const createBadge = useCallback(async (data: CreateBadgeData) => {
    return api.post<Badge>('/badges', data);
  }, [api]);

  const updateBadge = useCallback(async (badgeId: number, data: Partial<CreateBadgeData>) => {
    return api.put<Badge>(`/badges/${badgeId}`, data);
  }, [api]);

  const deleteBadge = useCallback(async (badgeId: number) => {
    return api.delete(`/badges/${badgeId}`);
  }, [api]);

  // Award badge to player
  const awardBadge = useCallback(async (data: AwardBadgeData) => {
    return api.post<PlayerBadge>('/badges/award', data);
  }, [api]);

  // Revoke badge from player
  const revokeBadge = useCallback(async (data: RevokeBadgeData) => {
    return api.post<{ message: string }>('/badges/revoke', data);
  }, [api]);

  // Get player's badges
  const getPlayerBadges = useCallback(async (playerId: number) => {
    return api.get<PlayerBadge[]>(`/badges/players/${playerId}/badges`);
  }, [api]);

  // ==================== LEVELS ====================

  const listLevels = useCallback(async (filters?: MechanicsFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<Level>>(`/levels${query ? `?${query}` : ''}`);
  }, [api]);

  const getLevel = useCallback(async (levelId: number) => {
    return api.get<Level>(`/levels/${levelId}`);
  }, [api]);

  const createLevel = useCallback(async (data: CreateLevelData) => {
    return api.post<Level>('/levels', data);
  }, [api]);

  const updateLevel = useCallback(async (levelId: number, data: Partial<CreateLevelData>) => {
    return api.put<Level>(`/levels/${levelId}`, data);
  }, [api]);

  const deleteLevel = useCallback(async (levelId: number) => {
    return api.delete(`/levels/${levelId}`);
  }, [api]);

  // Grant XP to player
  const grantXp = useCallback(async (data: GrantXpData) => {
    return api.post<GrantXpResponse>('/levels/xp', data);
  }, [api]);

  // Get player's current level
  const getPlayerLevel = useCallback(async (playerId: number) => {
    return api.get<PlayerLevel>(`/levels/players/${playerId}/level`);
  }, [api]);

  // ==================== MISSIONS ====================

  const listMissions = useCallback(async (filters?: MechanicsFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<Mission>>(`/missions${query ? `?${query}` : ''}`);
  }, [api]);

  const getMission = useCallback(async (missionId: number) => {
    return api.get<Mission>(`/missions/${missionId}`);
  }, [api]);

  const createMission = useCallback(async (data: CreateMissionData) => {
    return api.post<Mission>('/missions', data);
  }, [api]);

  const updateMission = useCallback(async (missionId: number, data: Partial<CreateMissionData>) => {
    return api.put<Mission>(`/missions/${missionId}`, data);
  }, [api]);

  const deleteMission = useCallback(async (missionId: number) => {
    return api.delete(`/missions/${missionId}`);
  }, [api]);

  // ==================== STREAKS ====================

  const listStreaks = useCallback(async (filters?: MechanicsFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<Streak>>(`/streaks${query ? `?${query}` : ''}`);
  }, [api]);

  const getStreak = useCallback(async (streakId: number) => {
    return api.get<Streak>(`/streaks/${streakId}`);
  }, [api]);

  const createStreak = useCallback(async (data: CreateStreakData) => {
    return api.post<Streak>('/streaks', data);
  }, [api]);

  const updateStreak = useCallback(async (streakId: number, data: Partial<CreateStreakData>) => {
    return api.put<Streak>(`/streaks/${streakId}`, data);
  }, [api]);

  const deleteStreak = useCallback(async (streakId: number) => {
    return api.delete(`/streaks/${streakId}`);
  }, [api]);

  // ==================== LEADERBOARDS ====================

  const listLeaderboards = useCallback(async () => {
    return api.get<Leaderboard[]>('/leaderboards');
  }, [api]);

  const getLeaderboard = useCallback(async (leaderboardId: number) => {
    return api.get<Leaderboard>(`/leaderboards/${leaderboardId}`);
  }, [api]);

  const createLeaderboard = useCallback(async (data: CreateLeaderboardData) => {
    return api.post<Leaderboard>('/leaderboards', data);
  }, [api]);

  const updateLeaderboard = useCallback(async (leaderboardId: number, data: Partial<CreateLeaderboardData>) => {
    return api.put<Leaderboard>(`/leaderboards/${leaderboardId}`, data);
  }, [api]);

  const deleteLeaderboard = useCallback(async (leaderboardId: number) => {
    return api.delete(`/leaderboards/${leaderboardId}`);
  }, [api]);

  const getLeaderboardEntries = useCallback(async (leaderboardId: number, limit = 100, offset = 0) => {
    return api.get<LeaderboardEntry[]>(`/leaderboards/${leaderboardId}/entries?limit=${limit}&offset=${offset}`);
  }, [api]);

  const getPlayerRank = useCallback(async (leaderboardId: number, playerId: number) => {
    return api.get<LeaderboardEntry>(`/leaderboards/${leaderboardId}/players/${playerId}`);
  }, [api]);

  // ==================== REWARDS ====================

  const listRewards = useCallback(async (filters?: MechanicsFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<Reward>>(`/rewards${query ? `?${query}` : ''}`);
  }, [api]);

  const getReward = useCallback(async (rewardId: number) => {
    return api.get<Reward>(`/rewards/${rewardId}`);
  }, [api]);

  const createReward = useCallback(async (data: CreateRewardData) => {
    return api.post<Reward>('/rewards', data);
  }, [api]);

  const updateReward = useCallback(async (rewardId: number, data: Partial<CreateRewardData>) => {
    return api.put<Reward>(`/rewards/${rewardId}`, data);
  }, [api]);

  const deleteReward = useCallback(async (rewardId: number) => {
    return api.delete(`/rewards/${rewardId}`);
  }, [api]);

  const redeemReward = useCallback(async (playerId: number, rewardId: number) => {
    return api.post<RewardRedemption>('/rewards/redeem', { player_id: playerId, reward_id: rewardId });
  }, [api]);

  const listRedemptions = useCallback(async (status?: string) => {
    const query = status ? `?status=${status}` : '';
    return api.get<PaginatedResponse<RewardRedemption>>(`/rewards/redemptions${query}`);
  }, [api]);

  const fulfillRedemption = useCallback(async (redemptionId: number) => {
    return api.post<RewardRedemption>(`/rewards/redemptions/${redemptionId}/fulfill`);
  }, [api]);

  const cancelRedemption = useCallback(async (redemptionId: number, reason?: string) => {
    return api.post<RewardRedemption>(`/rewards/redemptions/${redemptionId}/cancel`, { reason });
  }, [api]);

  // ==================== POINT WALLETS ====================

  const listPointWallets = useCallback(async () => {
    return api.get<PointWallet[]>('/point-wallets');
  }, [api]);

  const getPointWallet = useCallback(async (walletId: number) => {
    return api.get<PointWallet>(`/point-wallets/${walletId}`);
  }, [api]);

  const createPointWallet = useCallback(async (data: { name: string; description?: string; currency?: string }) => {
    return api.post<PointWallet>('/point-wallets', data);
  }, [api]);

  const updatePointWallet = useCallback(async (walletId: number, data: { name?: string; description?: string }) => {
    return api.put<PointWallet>(`/point-wallets/${walletId}`, data);
  }, [api]);

  const deletePointWallet = useCallback(async (walletId: number) => {
    return api.delete(`/point-wallets/${walletId}`);
  }, [api]);

  return {
    // Badges
    listBadges,
    getBadge,
    createBadge,
    updateBadge,
    deleteBadge,
    awardBadge,
    revokeBadge,
    getPlayerBadges,
    // Levels
    listLevels,
    getLevel,
    createLevel,
    updateLevel,
    deleteLevel,
    grantXp,
    getPlayerLevel,
    // Missions
    listMissions,
    getMission,
    createMission,
    updateMission,
    deleteMission,
    // Streaks
    listStreaks,
    getStreak,
    createStreak,
    updateStreak,
    deleteStreak,
    // Leaderboards
    listLeaderboards,
    getLeaderboard,
    createLeaderboard,
    updateLeaderboard,
    deleteLeaderboard,
    getLeaderboardEntries,
    getPlayerRank,
    // Rewards
    listRewards,
    getReward,
    createReward,
    updateReward,
    deleteReward,
    redeemReward,
    listRedemptions,
    fulfillRedemption,
    cancelRedemption,
    // Point Wallets
    listPointWallets,
    getPointWallet,
    createPointWallet,
    updatePointWallet,
    deletePointWallet,
  };
}

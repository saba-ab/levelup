import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  Badge,
  CreateBadgeData,
  Level,
  CreateLevelData,
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
    return api.get<PaginatedResponse<Badge>>(`/api/badges${query ? `?${query}` : ''}`);
  }, [api]);

  const getBadge = useCallback(async (badgeId: string) => {
    return api.get<Badge>(`/api/badges/${badgeId}`);
  }, [api]);

  const createBadge = useCallback(async (data: CreateBadgeData) => {
    return api.post<Badge>('/api/badges', data);
  }, [api]);

  const updateBadge = useCallback(async (badgeId: string, data: Partial<CreateBadgeData>) => {
    return api.patch<Badge>(`/api/badges/${badgeId}`, data);
  }, [api]);

  const deleteBadge = useCallback(async (badgeId: string) => {
    return api.delete(`/api/badges/${badgeId}`);
  }, [api]);

  // ==================== LEVELS ====================

  const listLevels = useCallback(async () => {
    return api.get<Level[]>('/api/levels');
  }, [api]);

  const getLevel = useCallback(async (levelId: string) => {
    return api.get<Level>(`/api/levels/${levelId}`);
  }, [api]);

  const createLevel = useCallback(async (data: CreateLevelData) => {
    return api.post<Level>('/api/levels', data);
  }, [api]);

  const updateLevel = useCallback(async (levelId: string, data: Partial<CreateLevelData>) => {
    return api.patch<Level>(`/api/levels/${levelId}`, data);
  }, [api]);

  const deleteLevel = useCallback(async (levelId: string) => {
    return api.delete(`/api/levels/${levelId}`);
  }, [api]);

  const reorderLevels = useCallback(async (levelIds: string[]) => {
    return api.post<Level[]>('/api/levels/reorder', { level_ids: levelIds });
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
    return api.get<PaginatedResponse<Mission>>(`/api/missions${query ? `?${query}` : ''}`);
  }, [api]);

  const getMission = useCallback(async (missionId: string) => {
    return api.get<Mission>(`/api/missions/${missionId}`);
  }, [api]);

  const createMission = useCallback(async (data: CreateMissionData) => {
    return api.post<Mission>('/api/missions', data);
  }, [api]);

  const updateMission = useCallback(async (missionId: string, data: Partial<CreateMissionData>) => {
    return api.patch<Mission>(`/api/missions/${missionId}`, data);
  }, [api]);

  const deleteMission = useCallback(async (missionId: string) => {
    return api.delete(`/api/missions/${missionId}`);
  }, [api]);

  const activateMission = useCallback(async (missionId: string) => {
    return api.post<Mission>(`/api/missions/${missionId}/activate`);
  }, [api]);

  const deactivateMission = useCallback(async (missionId: string) => {
    return api.post<Mission>(`/api/missions/${missionId}/deactivate`);
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
    return api.get<PaginatedResponse<Streak>>(`/api/streaks${query ? `?${query}` : ''}`);
  }, [api]);

  const getStreak = useCallback(async (streakId: string) => {
    return api.get<Streak>(`/api/streaks/${streakId}`);
  }, [api]);

  const createStreak = useCallback(async (data: CreateStreakData) => {
    return api.post<Streak>('/api/streaks', data);
  }, [api]);

  const updateStreak = useCallback(async (streakId: string, data: Partial<CreateStreakData>) => {
    return api.patch<Streak>(`/api/streaks/${streakId}`, data);
  }, [api]);

  const deleteStreak = useCallback(async (streakId: string) => {
    return api.delete(`/api/streaks/${streakId}`);
  }, [api]);

  // ==================== LEADERBOARDS ====================

  const listLeaderboards = useCallback(async () => {
    return api.get<Leaderboard[]>('/api/leaderboards');
  }, [api]);

  const getLeaderboard = useCallback(async (leaderboardId: string) => {
    return api.get<Leaderboard>(`/api/leaderboards/${leaderboardId}`);
  }, [api]);

  const createLeaderboard = useCallback(async (data: CreateLeaderboardData) => {
    return api.post<Leaderboard>('/api/leaderboards', data);
  }, [api]);

  const updateLeaderboard = useCallback(async (leaderboardId: string, data: Partial<CreateLeaderboardData>) => {
    return api.patch<Leaderboard>(`/api/leaderboards/${leaderboardId}`, data);
  }, [api]);

  const deleteLeaderboard = useCallback(async (leaderboardId: string) => {
    return api.delete(`/api/leaderboards/${leaderboardId}`);
  }, [api]);

  const getLeaderboardEntries = useCallback(async (leaderboardId: string, limit = 100, offset = 0) => {
    return api.get<LeaderboardEntry[]>(`/api/leaderboards/${leaderboardId}/entries?limit=${limit}&offset=${offset}`);
  }, [api]);

  const getPlayerRank = useCallback(async (leaderboardId: string, playerId: string) => {
    return api.get<LeaderboardEntry>(`/api/leaderboards/${leaderboardId}/players/${playerId}`);
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
    return api.get<PaginatedResponse<Reward>>(`/api/rewards${query ? `?${query}` : ''}`);
  }, [api]);

  const getReward = useCallback(async (rewardId: string) => {
    return api.get<Reward>(`/api/rewards/${rewardId}`);
  }, [api]);

  const createReward = useCallback(async (data: CreateRewardData) => {
    return api.post<Reward>('/api/rewards', data);
  }, [api]);

  const updateReward = useCallback(async (rewardId: string, data: Partial<CreateRewardData>) => {
    return api.patch<Reward>(`/api/rewards/${rewardId}`, data);
  }, [api]);

  const deleteReward = useCallback(async (rewardId: string) => {
    return api.delete(`/api/rewards/${rewardId}`);
  }, [api]);

  const redeemReward = useCallback(async (playerId: string, rewardId: string) => {
    return api.post<RewardRedemption>('/api/rewards/redeem', { player_id: playerId, reward_id: rewardId });
  }, [api]);

  const listRedemptions = useCallback(async (status?: string) => {
    const query = status ? `?status=${status}` : '';
    return api.get<PaginatedResponse<RewardRedemption>>(`/api/rewards/redemptions${query}`);
  }, [api]);

  const fulfillRedemption = useCallback(async (redemptionId: string) => {
    return api.post<RewardRedemption>(`/api/rewards/redemptions/${redemptionId}/fulfill`);
  }, [api]);

  const cancelRedemption = useCallback(async (redemptionId: string, reason?: string) => {
    return api.post<RewardRedemption>(`/api/rewards/redemptions/${redemptionId}/cancel`, { reason });
  }, [api]);

  // ==================== POINT WALLETS ====================

  const listPointWallets = useCallback(async () => {
    return api.get<PointWallet[]>('/api/point-wallets');
  }, [api]);

  const getPointWallet = useCallback(async (walletId: string) => {
    return api.get<PointWallet>(`/api/point-wallets/${walletId}`);
  }, [api]);

  const createPointWallet = useCallback(async (data: { name: string; description?: string; currency?: string }) => {
    return api.post<PointWallet>('/api/point-wallets', data);
  }, [api]);

  const updatePointWallet = useCallback(async (walletId: string, data: { name?: string; description?: string }) => {
    return api.patch<PointWallet>(`/api/point-wallets/${walletId}`, data);
  }, [api]);

  const deletePointWallet = useCallback(async (walletId: string) => {
    return api.delete(`/api/point-wallets/${walletId}`);
  }, [api]);

  return {
    // Badges
    listBadges,
    getBadge,
    createBadge,
    updateBadge,
    deleteBadge,
    // Levels
    listLevels,
    getLevel,
    createLevel,
    updateLevel,
    deleteLevel,
    reorderLevels,
    // Missions
    listMissions,
    getMission,
    createMission,
    updateMission,
    deleteMission,
    activateMission,
    deactivateMission,
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
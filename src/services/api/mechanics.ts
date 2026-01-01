import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  Badge,
  CreateBadgeData,
  UpdateBadgeData,
  PlayerBadge,
  AwardBadgeData,
  Level,
  CreateLevelData,
  UpdateLevelData,
  PlayerLevel,
  GrantXpData,
  GrantXpResponse,
  Mission,
  CreateMissionData,
  PlayerMission,
  StartMissionData,
  UpdateMissionProgressData,
  Streak,
  CreateStreakData,
  PlayerStreak,
  RecordStreakActivityData,
  RecordStreakActivityResponse,
  Leaderboard,
  LeaderboardEntry,
  CreateLeaderboardData,
  Reward,
  CreateRewardData,
  PlayerReward,
  ClaimRewardData,
  Wallet,
  WalletTransaction,
  CreditWalletData,
  DebitWalletData,
  TransferPointsData,
  WalletTransactionFilters,
  Rule,
  CreateRuleData,
  RuleVersion,
  ExecuteRulesData,
  RuleExecution,
  RuleExecutionFilters,
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

  const updateBadge = useCallback(async (badgeId: number, data: UpdateBadgeData) => {
    return api.put<Badge>(`/badges/${badgeId}`, data);
  }, [api]);

  const deleteBadge = useCallback(async (badgeId: number) => {
    return api.delete(`/badges/${badgeId}`);
  }, [api]);

  const awardBadge = useCallback(async (data: AwardBadgeData) => {
    return api.post<PlayerBadge>('/badges/award', data);
  }, [api]);

  const revokeBadge = useCallback(async (playerId: number, badgeId: number) => {
    return api.delete(`/badges/players/${playerId}/badges/${badgeId}`);
  }, [api]);

  const getPlayerBadges = useCallback(async (playerId: number) => {
    return api.get<PlayerBadge[]>(`/badges/players/${playerId}`);
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

  const updateLevel = useCallback(async (levelId: number, data: UpdateLevelData) => {
    return api.put<Level>(`/levels/${levelId}`, data);
  }, [api]);

  const deleteLevel = useCallback(async (levelId: number) => {
    return api.delete(`/levels/${levelId}`);
  }, [api]);

  const grantXp = useCallback(async (data: GrantXpData) => {
    return api.post<GrantXpResponse>('/levels/xp', data);
  }, [api]);

  const getPlayerLevel = useCallback(async (playerId: number) => {
    return api.get<PlayerLevel>(`/levels/players/${playerId}`);
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

  const startMission = useCallback(async (data: StartMissionData) => {
    return api.post<PlayerMission>('/missions/start', data);
  }, [api]);

  const updateMissionProgress = useCallback(async (playerId: number, missionId: number, data: UpdateMissionProgressData) => {
    return api.put<PlayerMission>(`/missions/players/${playerId}/missions/${missionId}/progress`, data);
  }, [api]);

  const completeMission = useCallback(async (playerId: number, missionId: number) => {
    return api.post<PlayerMission>(`/missions/players/${playerId}/missions/${missionId}/complete`);
  }, [api]);

  const getPlayerMissions = useCallback(async (playerId: number) => {
    return api.get<PlayerMission[]>(`/missions/players/${playerId}/missions`);
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

  const recordStreakActivity = useCallback(async (data: RecordStreakActivityData) => {
    return api.post<RecordStreakActivityResponse>('/streaks/activity', data);
  }, [api]);

  const getPlayerStreak = useCallback(async (playerId: number, streakId: number) => {
    return api.get<PlayerStreak>(`/streaks/players/${playerId}/streaks/${streakId}`);
  }, [api]);

  const getPlayerStreaks = useCallback(async (playerId: number) => {
    return api.get<PlayerStreak[]>(`/streaks/players/${playerId}/streaks`);
  }, [api]);

  const resetStreak = useCallback(async (playerId: number, streakId: number) => {
    return api.post<PlayerStreak>(`/streaks/players/${playerId}/streaks/${streakId}/reset`);
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

  const claimReward = useCallback(async (data: ClaimRewardData) => {
    return api.post<PlayerReward>('/rewards/claim', data);
  }, [api]);

  const redeemReward = useCallback(async (playerId: number, rewardId: number) => {
    return api.post<PlayerReward>(`/rewards/players/${playerId}/rewards/${rewardId}/redeem`);
  }, [api]);

  const getPlayerRewards = useCallback(async (playerId: number) => {
    return api.get<PlayerReward[]>(`/rewards/players/${playerId}/rewards`);
  }, [api]);

  // ==================== WALLETS ====================

  const getPlayerWallet = useCallback(async (playerId: number) => {
    return api.get<Wallet>(`/wallets/players/${playerId}/wallet`);
  }, [api]);

  const creditWallet = useCallback(async (data: CreditWalletData) => {
    return api.post<WalletTransaction>('/wallets/credit', data);
  }, [api]);

  const debitWallet = useCallback(async (data: DebitWalletData) => {
    return api.post<WalletTransaction>('/wallets/debit', data);
  }, [api]);

  const transferPoints = useCallback(async (data: TransferPointsData) => {
    return api.post<WalletTransaction[]>('/wallets/transfer', data);
  }, [api]);

  const getWalletTransactions = useCallback(async (playerId: number, filters?: WalletTransactionFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<WalletTransaction>>(`/wallets/players/${playerId}/transactions${query ? `?${query}` : ''}`);
  }, [api]);

  // ==================== RULES ====================

  const listRules = useCallback(async (filters?: MechanicsFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<Rule>>(`/rules${query ? `?${query}` : ''}`);
  }, [api]);

  const getRule = useCallback(async (ruleId: number) => {
    return api.get<Rule>(`/rules/${ruleId}`);
  }, [api]);

  const createRule = useCallback(async (data: CreateRuleData) => {
    return api.post<Rule>('/rules', data);
  }, [api]);

  const updateRule = useCallback(async (ruleId: number, data: Partial<CreateRuleData>) => {
    return api.put<Rule>(`/rules/${ruleId}`, data);
  }, [api]);

  const deleteRule = useCallback(async (ruleId: number) => {
    return api.delete(`/rules/${ruleId}`);
  }, [api]);

  const createRuleVersion = useCallback(async (ruleId: number, data: CreateRuleData) => {
    return api.post<RuleVersion>(`/rules/${ruleId}/versions`, data);
  }, [api]);

  const executeRules = useCallback(async (data: ExecuteRulesData) => {
    return api.post<RuleExecution[]>('/rules/execute', data);
  }, [api]);

  const getRuleExecutions = useCallback(async (filters?: RuleExecutionFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<RuleExecution>>(`/rules/executions${query ? `?${query}` : ''}`);
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
    startMission,
    updateMissionProgress,
    completeMission,
    getPlayerMissions,
    // Streaks
    listStreaks,
    getStreak,
    createStreak,
    updateStreak,
    deleteStreak,
    recordStreakActivity,
    getPlayerStreak,
    getPlayerStreaks,
    resetStreak,
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
    claimReward,
    redeemReward,
    getPlayerRewards,
    // Wallets
    getPlayerWallet,
    creditWallet,
    debitWallet,
    transferPoints,
    getWalletTransactions,
    // Rules
    listRules,
    getRule,
    createRule,
    updateRule,
    deleteRule,
    createRuleVersion,
    executeRules,
    getRuleExecutions,
  };
}

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
import {
  BADGE_ENDPOINTS,
  LEVEL_ENDPOINTS,
  MISSION_ENDPOINTS,
  STREAK_ENDPOINTS,
  LEADERBOARD_ENDPOINTS,
  REWARD_ENDPOINTS,
  WALLET_ENDPOINTS,
  PLAYER_ENDPOINTS,
  RULE_ENDPOINTS,
} from '@/lib/api-routes';

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
    return api.get<PaginatedResponse<Badge>>(`${BADGE_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getBadge = useCallback(async (badgeId: number) => {
    return api.get<Badge>(BADGE_ENDPOINTS.SHOW(badgeId));
  }, [api]);

  const createBadge = useCallback(async (data: CreateBadgeData) => {
    return api.post<Badge>(BADGE_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateBadge = useCallback(async (badgeId: number, data: UpdateBadgeData) => {
    return api.put<Badge>(BADGE_ENDPOINTS.UPDATE(badgeId), data);
  }, [api]);

  const deleteBadge = useCallback(async (badgeId: number) => {
    return api.delete(BADGE_ENDPOINTS.DELETE(badgeId));
  }, [api]);

  const awardBadge = useCallback(async (data: AwardBadgeData) => {
    return api.post<PlayerBadge>(BADGE_ENDPOINTS.AWARD, data);
  }, [api]);

  const revokeBadge = useCallback(async (playerId: number, badgeId: number) => {
    return api.delete(`${BADGE_ENDPOINTS.LIST}/players/${playerId}/badges/${badgeId}`);
  }, [api]);

  const getPlayerBadges = useCallback(async (playerId: number) => {
    return api.get<PlayerBadge[]>(`${BADGE_ENDPOINTS.LIST}/players/${playerId}`);
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
    return api.get<PaginatedResponse<Level>>(`${LEVEL_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getLevel = useCallback(async (levelId: number) => {
    return api.get<Level>(LEVEL_ENDPOINTS.SHOW(levelId));
  }, [api]);

  const createLevel = useCallback(async (data: CreateLevelData) => {
    return api.post<Level>(LEVEL_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateLevel = useCallback(async (levelId: number, data: UpdateLevelData) => {
    return api.put<Level>(LEVEL_ENDPOINTS.UPDATE(levelId), data);
  }, [api]);

  const deleteLevel = useCallback(async (levelId: number) => {
    return api.delete(LEVEL_ENDPOINTS.DELETE(levelId));
  }, [api]);

  const grantXp = useCallback(async (data: GrantXpData) => {
    return api.post<GrantXpResponse>(LEVEL_ENDPOINTS.GRANT_XP, data);
  }, [api]);

  const getPlayerLevel = useCallback(async (playerId: number) => {
    return api.get<PlayerLevel>(`${LEVEL_ENDPOINTS.LIST}/players/${playerId}`);
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
    return api.get<PaginatedResponse<Mission>>(`${MISSION_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getMission = useCallback(async (missionId: number) => {
    return api.get<Mission>(MISSION_ENDPOINTS.SHOW(missionId));
  }, [api]);

  const createMission = useCallback(async (data: CreateMissionData) => {
    return api.post<Mission>(MISSION_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateMission = useCallback(async (missionId: number, data: Partial<CreateMissionData>) => {
    return api.put<Mission>(MISSION_ENDPOINTS.UPDATE(missionId), data);
  }, [api]);

  const deleteMission = useCallback(async (missionId: number) => {
    return api.delete(MISSION_ENDPOINTS.DELETE(missionId));
  }, [api]);

  const startMission = useCallback(async (data: StartMissionData) => {
    return api.post<PlayerMission>(MISSION_ENDPOINTS.START, data);
  }, [api]);

  const updateMissionProgress = useCallback(async (playerId: number, missionId: number, data: UpdateMissionProgressData) => {
    return api.put<PlayerMission>(`${MISSION_ENDPOINTS.LIST}/players/${playerId}/missions/${missionId}/progress`, data);
  }, [api]);

  const completeMission = useCallback(async (playerId: number, missionId: number) => {
    return api.post<PlayerMission>(`${MISSION_ENDPOINTS.LIST}/players/${playerId}/missions/${missionId}/complete`);
  }, [api]);

  const getPlayerMissions = useCallback(async (playerId: number) => {
    return api.get<PlayerMission[]>(`${MISSION_ENDPOINTS.LIST}/players/${playerId}/missions`);
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
    return api.get<PaginatedResponse<Streak>>(`${STREAK_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getStreak = useCallback(async (streakId: number) => {
    return api.get<Streak>(STREAK_ENDPOINTS.SHOW(streakId));
  }, [api]);

  const createStreak = useCallback(async (data: CreateStreakData) => {
    return api.post<Streak>(STREAK_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateStreak = useCallback(async (streakId: number, data: Partial<CreateStreakData>) => {
    return api.put<Streak>(STREAK_ENDPOINTS.UPDATE(streakId), data);
  }, [api]);

  const deleteStreak = useCallback(async (streakId: number) => {
    return api.delete(STREAK_ENDPOINTS.DELETE(streakId));
  }, [api]);

  const recordStreakActivity = useCallback(async (data: RecordStreakActivityData) => {
    return api.post<RecordStreakActivityResponse>(STREAK_ENDPOINTS.RECORD_ACTIVITY, data);
  }, [api]);

  const getPlayerStreak = useCallback(async (playerId: number, streakId: number) => {
    return api.get<PlayerStreak>(`${STREAK_ENDPOINTS.LIST}/players/${playerId}/streaks/${streakId}`);
  }, [api]);

  const getPlayerStreaks = useCallback(async (playerId: number) => {
    return api.get<PlayerStreak[]>(`${STREAK_ENDPOINTS.LIST}/players/${playerId}/streaks`);
  }, [api]);

  const resetStreak = useCallback(async (playerId: number, streakId: number) => {
    return api.post<PlayerStreak>(`${STREAK_ENDPOINTS.LIST}/players/${playerId}/streaks/${streakId}/reset`);
  }, [api]);

  // ==================== LEADERBOARDS ====================

  const listLeaderboards = useCallback(async () => {
    return api.get<Leaderboard[]>(LEADERBOARD_ENDPOINTS.LIST);
  }, [api]);

  const getLeaderboard = useCallback(async (leaderboardId: number) => {
    return api.get<Leaderboard>(LEADERBOARD_ENDPOINTS.SHOW(leaderboardId));
  }, [api]);

  const createLeaderboard = useCallback(async (data: CreateLeaderboardData) => {
    return api.post<Leaderboard>(LEADERBOARD_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateLeaderboard = useCallback(async (leaderboardId: number, data: Partial<CreateLeaderboardData>) => {
    return api.put<Leaderboard>(LEADERBOARD_ENDPOINTS.UPDATE(leaderboardId), data);
  }, [api]);

  const deleteLeaderboard = useCallback(async (leaderboardId: number) => {
    return api.delete(LEADERBOARD_ENDPOINTS.DELETE(leaderboardId));
  }, [api]);

  const getLeaderboardEntries = useCallback(async (leaderboardId: number, limit = 100, offset = 0) => {
    return api.get<LeaderboardEntry[]>(`${LEADERBOARD_ENDPOINTS.ENTRIES(leaderboardId)}?limit=${limit}&offset=${offset}`);
  }, [api]);

  const getPlayerRank = useCallback(async (leaderboardId: number, playerId: number) => {
    return api.get<LeaderboardEntry>(LEADERBOARD_ENDPOINTS.PLAYER_RANK(leaderboardId, playerId));
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
    return api.get<PaginatedResponse<Reward>>(`${REWARD_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getReward = useCallback(async (rewardId: number) => {
    return api.get<Reward>(REWARD_ENDPOINTS.SHOW(rewardId));
  }, [api]);

  const createReward = useCallback(async (data: CreateRewardData) => {
    return api.post<Reward>(REWARD_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateReward = useCallback(async (rewardId: number, data: Partial<CreateRewardData>) => {
    return api.put<Reward>(REWARD_ENDPOINTS.UPDATE(rewardId), data);
  }, [api]);

  const deleteReward = useCallback(async (rewardId: number) => {
    return api.delete(REWARD_ENDPOINTS.DELETE(rewardId));
  }, [api]);

  const claimReward = useCallback(async (data: ClaimRewardData) => {
    return api.post<PlayerReward>(REWARD_ENDPOINTS.CLAIM, data);
  }, [api]);

  const redeemReward = useCallback(async (playerId: number, rewardId: number) => {
    return api.post<PlayerReward>(`${REWARD_ENDPOINTS.LIST}/players/${playerId}/rewards/${rewardId}/redeem`);
  }, [api]);

  const getPlayerRewards = useCallback(async (playerId: number) => {
    return api.get<PlayerReward[]>(`${REWARD_ENDPOINTS.LIST}/players/${playerId}/rewards`);
  }, [api]);

  // ==================== WALLETS ====================

  const getPlayerWallet = useCallback(async (playerId: number) => {
    return api.get<Wallet>(PLAYER_ENDPOINTS.WALLET(playerId));
  }, [api]);

  const creditWallet = useCallback(async (data: CreditWalletData) => {
    return api.post<WalletTransaction>(WALLET_ENDPOINTS.CREDIT, data);
  }, [api]);

  const debitWallet = useCallback(async (data: DebitWalletData) => {
    return api.post<WalletTransaction>(WALLET_ENDPOINTS.DEBIT, data);
  }, [api]);

  const transferPoints = useCallback(async (data: TransferPointsData) => {
    return api.post<{ message: string; source_transaction: WalletTransaction; destination_transaction: WalletTransaction }>(WALLET_ENDPOINTS.TRANSFER, data);
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
    return api.get<PaginatedResponse<WalletTransaction>>(`${PLAYER_ENDPOINTS.WALLET_TRANSACTIONS(playerId)}${query ? `?${query}` : ''}`);
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
    return api.get<PaginatedResponse<Rule>>(`${RULE_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getRule = useCallback(async (ruleId: number) => {
    return api.get<Rule>(RULE_ENDPOINTS.SHOW(ruleId));
  }, [api]);

  const createRule = useCallback(async (data: CreateRuleData) => {
    return api.post<Rule>(RULE_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateRule = useCallback(async (ruleId: number, data: Partial<CreateRuleData>) => {
    return api.put<Rule>(RULE_ENDPOINTS.UPDATE(ruleId), data);
  }, [api]);

  const deleteRule = useCallback(async (ruleId: number) => {
    return api.delete(RULE_ENDPOINTS.DELETE(ruleId));
  }, [api]);

  const createRuleVersion = useCallback(async (ruleId: number, data: CreateRuleData) => {
    return api.post<RuleVersion>(`${RULE_ENDPOINTS.SHOW(ruleId)}/versions`, data);
  }, [api]);

  const executeRules = useCallback(async (data: ExecuteRulesData) => {
    return api.post<RuleExecution[]>(RULE_ENDPOINTS.EXECUTE, data);
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
    return api.get<PaginatedResponse<RuleExecution>>(`${RULE_ENDPOINTS.LIST}/executions${query ? `?${query}` : ''}`);
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

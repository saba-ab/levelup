import { useCallback, useMemo } from 'react';
import { useApi } from '@/hooks/useApi';
import {
  BADGE_ENDPOINTS,
  LEVEL_ENDPOINTS,
  MISSION_ENDPOINTS,
  STREAK_ENDPOINTS,
  LEADERBOARD_ENDPOINTS,
  REWARD_ENDPOINTS,
  PLAYER_ENDPOINTS,
  toQuery,
} from '@/lib/api-routes';
import type {
  ID,
  CursorPage,
  CursorParams,
  Badge,
  BadgeFilters,
  CreateBadgeData,
  UpdateBadgeData,
  PlayerBadge,
  AwardBadgeData,
  AwardBadgeResult,
  Level,
  LevelFilters,
  CreateLevelData,
  UpdateLevelData,
  Mission,
  MissionFilters,
  CreateMissionData,
  UpdateMissionData,
  MissionAttempt,
  MissionAttemptFilters,
  StartMissionData,
  UpdateMissionProgressData,
  CompleteMissionData,
  MissionProgressResult,
  Streak,
  StreakFilters,
  CreateStreakData,
  UpdateStreakData,
  PlayerStreak,
  RecordStreakActivityData,
  RecordStreakActivityResponse,
  Leaderboard,
  LeaderboardFilters,
  CreateLeaderboardData,
  UpdateLeaderboardData,
  LeaderboardEntriesParams,
  LeaderboardEntriesPage,
  PlayerRank,
  RebuildLeaderboardResult,
  Reward,
  RewardFilters,
  CreateRewardData,
  UpdateRewardData,
  RewardClaim,
  ClaimRewardData,
} from './types';

export function useMechanicsService() {
  const api = useApi();

  // ==================== BADGES ====================

  const listBadges = useCallback(
    (filters?: BadgeFilters) => api.get<CursorPage<Badge>>(`${BADGE_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );
  const getBadge = useCallback((badgeId: ID) => api.get<Badge>(BADGE_ENDPOINTS.SHOW(badgeId)), [api]);
  const createBadge = useCallback((data: CreateBadgeData) => api.post<Badge>(BADGE_ENDPOINTS.CREATE, data), [api]);
  const updateBadge = useCallback(
    (badgeId: ID, data: UpdateBadgeData) => api.patch<Badge>(BADGE_ENDPOINTS.UPDATE(badgeId), data),
    [api],
  );
  const deleteBadge = useCallback((badgeId: ID) => api.delete<void>(BADGE_ENDPOINTS.DELETE(badgeId)), [api]);
  /** 409 codes: badge_already_earned, badge_max_awards_reached, badge_inactive, player_inactive. */
  const awardBadge = useCallback(
    ({ badge_id, player_id }: AwardBadgeData) =>
      api.post<AwardBadgeResult>(BADGE_ENDPOINTS.AWARD(badge_id), { player_id }, { idempotencyKey: true }),
    [api],
  );
  const revokeBadge = useCallback(
    (playerId: ID, badgeId: ID) => api.delete<void>(BADGE_ENDPOINTS.REVOKE(badgeId, playerId)),
    [api],
  );
  const getPlayerBadges = useCallback(
    (playerId: ID, params?: CursorParams) =>
      api.get<CursorPage<PlayerBadge>>(`${PLAYER_ENDPOINTS.BADGES(playerId)}${toQuery(params)}`),
    [api],
  );

  // ==================== LEVELS ====================

  const listLevels = useCallback(
    (filters?: LevelFilters) => api.get<CursorPage<Level>>(`${LEVEL_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );
  const getLevel = useCallback((levelId: ID) => api.get<Level>(LEVEL_ENDPOINTS.SHOW(levelId)), [api]);
  /** 409 level_number_taken; 422 xp_required_not_increasing. */
  const createLevel = useCallback((data: CreateLevelData) => api.post<Level>(LEVEL_ENDPOINTS.CREATE, data), [api]);
  const updateLevel = useCallback(
    (levelId: ID, data: UpdateLevelData) => api.patch<Level>(LEVEL_ENDPOINTS.UPDATE(levelId), data),
    [api],
  );
  const deleteLevel = useCallback((levelId: ID) => api.delete<void>(LEVEL_ENDPOINTS.DELETE(levelId)), [api]);

  // ==================== MISSIONS ====================

  const listMissions = useCallback(
    (filters?: MissionFilters) => api.get<CursorPage<Mission>>(`${MISSION_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );
  const getMission = useCallback((missionId: ID) => api.get<Mission>(MISSION_ENDPOINTS.SHOW(missionId)), [api]);
  const createMission = useCallback(
    (data: CreateMissionData) => api.post<Mission>(MISSION_ENDPOINTS.CREATE, data),
    [api],
  );
  const updateMission = useCallback(
    (missionId: ID, data: UpdateMissionData) => api.patch<Mission>(MISSION_ENDPOINTS.UPDATE(missionId), data),
    [api],
  );
  const deleteMission = useCallback((missionId: ID) => api.delete<void>(MISSION_ENDPOINTS.DELETE(missionId)), [api]);
  const listMissionAttempts = useCallback(
    (missionId: ID, filters?: MissionAttemptFilters) =>
      api.get<CursorPage<MissionAttempt>>(`${MISSION_ENDPOINTS.ATTEMPTS(missionId)}${toQuery(filters)}`),
    [api],
  );
  const startMission = useCallback(
    ({ mission_id, player_id }: StartMissionData) =>
      api.post<MissionAttempt>(MISSION_ENDPOINTS.START(mission_id), { player_id }, { idempotencyKey: true }),
    [api],
  );
  const updateMissionProgress = useCallback(
    ({ mission_id, player_id, increment }: UpdateMissionProgressData) =>
      api.post<MissionProgressResult>(
        MISSION_ENDPOINTS.PROGRESS(mission_id),
        { player_id, increment },
        { idempotencyKey: true },
      ),
    [api],
  );
  const completeMission = useCallback(
    ({ mission_id, player_id }: CompleteMissionData) =>
      api.post<MissionAttempt>(MISSION_ENDPOINTS.COMPLETE(mission_id), { player_id }, { idempotencyKey: true }),
    [api],
  );
  const getPlayerMissions = useCallback(
    (playerId: ID, params?: CursorParams) =>
      api.get<CursorPage<MissionAttempt>>(`${PLAYER_ENDPOINTS.MISSIONS(playerId)}${toQuery(params)}`),
    [api],
  );

  // ==================== STREAKS ====================

  const listStreaks = useCallback(
    (filters?: StreakFilters) => api.get<CursorPage<Streak>>(`${STREAK_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );
  const getStreak = useCallback((streakId: ID) => api.get<Streak>(STREAK_ENDPOINTS.SHOW(streakId)), [api]);
  const createStreak = useCallback((data: CreateStreakData) => api.post<Streak>(STREAK_ENDPOINTS.CREATE, data), [api]);
  const updateStreak = useCallback(
    (streakId: ID, data: UpdateStreakData) => api.patch<Streak>(STREAK_ENDPOINTS.UPDATE(streakId), data),
    [api],
  );
  const deleteStreak = useCallback((streakId: ID) => api.delete<void>(STREAK_ENDPOINTS.DELETE(streakId)), [api]);
  const recordStreakActivity = useCallback(
    ({ streak_id, ...body }: RecordStreakActivityData) =>
      api.post<RecordStreakActivityResponse>(STREAK_ENDPOINTS.RECORD(streak_id), body, { idempotencyKey: true }),
    [api],
  );
  const getPlayerStreaks = useCallback(
    (playerId: ID, params?: CursorParams) =>
      api.get<CursorPage<PlayerStreak>>(`${PLAYER_ENDPOINTS.STREAKS(playerId)}${toQuery(params)}`),
    [api],
  );
  /** No single player-streak endpoint exists: reads the player's streaks and picks one. */
  const getPlayerStreak = useCallback(
    async (playerId: ID, streakId: ID) => {
      const res = await getPlayerStreaks(playerId, { limit: 100 });
      const found = res.data?.data.find((ps) => ps.streak_id === streakId) ?? null;
      return { ...res, data: res.success ? found : null };
    },
    [getPlayerStreaks],
  );
  const resetStreak = useCallback(
    (playerId: ID, streakId: ID) => api.post<PlayerStreak>(STREAK_ENDPOINTS.RESET(streakId, playerId)),
    [api],
  );

  // ==================== LEADERBOARDS ====================

  const listLeaderboards = useCallback(
    (filters?: LeaderboardFilters) =>
      api.get<CursorPage<Leaderboard>>(`${LEADERBOARD_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );
  const getLeaderboard = useCallback(
    (leaderboardId: ID) => api.get<Leaderboard>(LEADERBOARD_ENDPOINTS.SHOW(leaderboardId)),
    [api],
  );
  const createLeaderboard = useCallback(
    (data: CreateLeaderboardData) => api.post<Leaderboard>(LEADERBOARD_ENDPOINTS.CREATE, data),
    [api],
  );
  const updateLeaderboard = useCallback(
    (leaderboardId: ID, data: UpdateLeaderboardData) =>
      api.patch<Leaderboard>(LEADERBOARD_ENDPOINTS.UPDATE(leaderboardId), data),
    [api],
  );
  const deleteLeaderboard = useCallback(
    (leaderboardId: ID) => api.delete<void>(LEADERBOARD_ENDPOINTS.DELETE(leaderboardId)),
    [api],
  );
  const getLeaderboardEntries = useCallback(
    (leaderboardId: ID, params?: LeaderboardEntriesParams) =>
      api.get<LeaderboardEntriesPage>(`${LEADERBOARD_ENDPOINTS.ENTRIES(leaderboardId)}${toQuery(params)}`),
    [api],
  );
  /** 404 player_not_ranked when the player has no entry in the period. */
  const getPlayerRank = useCallback(
    (leaderboardId: ID, playerId: ID, params?: { period?: string; around?: number }) =>
      api.get<PlayerRank>(`${LEADERBOARD_ENDPOINTS.PLAYER(leaderboardId, playerId)}${toQuery(params)}`),
    [api],
  );
  const rebuildLeaderboard = useCallback(
    (leaderboardId: ID) => api.post<RebuildLeaderboardResult>(LEADERBOARD_ENDPOINTS.REBUILD(leaderboardId)),
    [api],
  );

  // ==================== REWARDS ====================

  const listRewards = useCallback(
    (filters?: RewardFilters) => api.get<CursorPage<Reward>>(`${REWARD_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );
  const getReward = useCallback((rewardId: ID) => api.get<Reward>(REWARD_ENDPOINTS.SHOW(rewardId)), [api]);
  const createReward = useCallback((data: CreateRewardData) => api.post<Reward>(REWARD_ENDPOINTS.CREATE, data), [api]);
  const updateReward = useCallback(
    (rewardId: ID, data: UpdateRewardData) => api.patch<Reward>(REWARD_ENDPOINTS.UPDATE(rewardId), data),
    [api],
  );
  const deleteReward = useCallback((rewardId: ID) => api.delete<void>(REWARD_ENDPOINTS.DELETE(rewardId)), [api]);
  /**
   * 201 claimed (free reward) or 202 pending_payment (paid: points are
   * debited asynchronously; poll getRewardClaim until it settles).
   */
  const claimReward = useCallback(
    ({ reward_id, player_id }: ClaimRewardData) =>
      api.post<RewardClaim>(REWARD_ENDPOINTS.CLAIM(reward_id), { player_id }, { idempotencyKey: true }),
    [api],
  );
  const getRewardClaim = useCallback(
    (claimId: ID) => api.get<RewardClaim>(REWARD_ENDPOINTS.CLAIM_SHOW(claimId)),
    [api],
  );
  const redeemRewardClaim = useCallback(
    (claimId: ID) => api.post<RewardClaim>(REWARD_ENDPOINTS.CLAIM_REDEEM(claimId)),
    [api],
  );
  const cancelRewardClaim = useCallback(
    (claimId: ID) => api.post<RewardClaim>(REWARD_ENDPOINTS.CLAIM_CANCEL(claimId)),
    [api],
  );
  const getPlayerRewards = useCallback(
    (playerId: ID, params?: CursorParams) =>
      api.get<CursorPage<RewardClaim>>(`${PLAYER_ENDPOINTS.REWARD_CLAIMS(playerId)}${toQuery(params)}`),
    [api],
  );

  return useMemo(
    () => ({
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
      // Missions
      listMissions,
      getMission,
      createMission,
      updateMission,
      deleteMission,
      listMissionAttempts,
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
      rebuildLeaderboard,
      // Rewards
      listRewards,
      getReward,
      createReward,
      updateReward,
      deleteReward,
      claimReward,
      getRewardClaim,
      redeemRewardClaim,
      cancelRewardClaim,
      /** @deprecated redeem works on a claim id: use redeemRewardClaim. */
      redeemReward: redeemRewardClaim,
      getPlayerRewards,
    }),
    [
      listBadges, getBadge, createBadge, updateBadge, deleteBadge, awardBadge, revokeBadge, getPlayerBadges,
      listLevels, getLevel, createLevel, updateLevel, deleteLevel,
      listMissions, getMission, createMission, updateMission, deleteMission, listMissionAttempts, startMission,
      updateMissionProgress, completeMission, getPlayerMissions,
      listStreaks, getStreak, createStreak, updateStreak, deleteStreak, recordStreakActivity, getPlayerStreak,
      getPlayerStreaks, resetStreak,
      listLeaderboards, getLeaderboard, createLeaderboard, updateLeaderboard, deleteLeaderboard,
      getLeaderboardEntries, getPlayerRank, rebuildLeaderboard,
      listRewards, getReward, createReward, updateReward, deleteReward, claimReward, getRewardClaim,
      redeemRewardClaim, cancelRewardClaim, getPlayerRewards,
    ],
  );
}

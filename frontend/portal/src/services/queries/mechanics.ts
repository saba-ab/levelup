import { useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import type { ApiResponse, ValidationErrors } from '@/hooks/useApi';
import { useMechanicsService } from '../api/mechanics';
import { fetchAllPages } from '../api/pagination';
import { queryKeys } from './keys';
import type {
  ID,
  CursorParams,
  BadgeFilters,
  CreateBadgeData,
  UpdateBadgeData,
  AwardBadgeData,
  LevelFilters,
  CreateLevelData,
  UpdateLevelData,
  MissionFilters,
  CreateMissionData,
  UpdateMissionData,
  MissionAttemptFilters,
  StartMissionData,
  UpdateMissionProgressData,
  CompleteMissionData,
  StreakFilters,
  CreateStreakData,
  UpdateStreakData,
  RecordStreakActivityData,
  LeaderboardFilters,
  CreateLeaderboardData,
  UpdateLeaderboardData,
  LeaderboardEntriesParams,
  RewardFilters,
  CreateRewardData,
  UpdateRewardData,
  RewardClaim,
  ClaimRewardData,
  MissionStatsFilters,
  RewardClaimFilters,
  WalletDailyParams,
} from '../api/types';

// ==================== ERRORS ====================

/**
 * A failed mechanics call. Carries the problem+json `code` so pages can
 * branch on it (badge_already_earned, level_number_taken, ...).
 */
export class MechanicsApiError extends Error {
  readonly code: string | null;
  readonly status: number;
  readonly validationErrors: ValidationErrors | null;

  constructor(message: string, code: string | null, status: number, validationErrors: ValidationErrors | null) {
    super(message);
    this.name = 'MechanicsApiError';
    this.code = code;
    this.status = status;
    this.validationErrors = validationErrors;
  }
}

function unwrap<T>(res: ApiResponse<T>, fallback: string): T {
  if (!res.success) {
    throw new MechanicsApiError(res.error || fallback, res.code, res.status, res.validationErrors);
  }
  return res.data as T;
}

/**
 * Message for a failed mutation: a per-code override when the page has one,
 * else the server's detail plus the first field error.
 */
export function describeMechanicsError(
  err: unknown,
  fallback: string,
  codeMessages: Record<string, string> = {},
): string {
  if (err instanceof MechanicsApiError) {
    if (err.code && codeMessages[err.code]) return codeMessages[err.code];
    const fieldErrors = err.validationErrors
      ? Object.entries(err.validationErrors).map(([field, msgs]) => `${field}: ${msgs.join(', ')}`)
      : [];
    return fieldErrors.length > 0 ? `${err.message} (${fieldErrors.join('; ')})` : err.message;
  }
  return err instanceof Error ? err.message : fallback;
}

/** Query keys this module needs beyond ./keys (which is shared and frozen). */
const localKeys = {
  allBadges: (filters?: BadgeFilters) => [...queryKeys.badges.lists(), 'all', filters] as const,
  missionAttempts: (missionId: ID, filters?: MissionAttemptFilters) =>
    [...queryKeys.missions.detail(missionId), 'attempts', filters] as const,
  rewardClaim: (claimId: ID) => [...queryKeys.rewards.all, 'claim', claimId] as const,
  leaderboardEntries: (id: ID, params?: LeaderboardEntriesParams) =>
    [...queryKeys.leaderboards.entries(id, params?.period, params?.cursor), params?.limit] as const,
  leaderboardLists: () => [...queryKeys.leaderboards.list()] as const,
  badgeStats: () => [...queryKeys.badges.all, 'stats'] as const,
  missionStatsAll: () => [...queryKeys.missions.all, 'stats'] as const,
  missionStatsList: (filters?: MissionStatsFilters) => [...queryKeys.missions.all, 'stats', filters] as const,
  missionStats: (missionId: ID) => [...queryKeys.missions.detail(missionId), 'stats'] as const,
  /** Under rewards.lists() so every claim / reward change refreshes it. */
  rewardClaims: (filters?: RewardClaimFilters) => [...queryKeys.rewards.lists(), 'claims', filters] as const,
  rewardStats: () => [...queryKeys.rewards.lists(), 'stats'] as const,
  walletAnalytics: () => [...queryKeys.wallets.all, 'analytics'] as const,
  walletSummary: () => [...queryKeys.wallets.all, 'analytics', 'summary'] as const,
  walletDistribution: () => [...queryKeys.wallets.all, 'analytics', 'distribution'] as const,
  walletDaily: (params?: WalletDailyParams) => [...queryKeys.wallets.all, 'analytics', 'daily', params] as const,
  playersProgress: (playerIds: ID[]) => [...queryKeys.levels.all, 'progress', playerIds] as const,
};

/** Query key of the wallet analytics (summary, distribution, daily): invalidate after wallet operations. */
export const walletAnalyticsKey = localKeys.walletAnalytics;

// ==================== BADGES ====================

/** One page of badges (?tier&category&active&limit&cursor). */
export function useBadgesQuery(filters?: BadgeFilters) {
  const { listBadges } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.badges.list(filters),
    queryFn: async () => unwrap(await listBadges(filters), 'Failed to fetch badges'),
  });
}

/** Every badge (walks all pages) for selects such as "badge reward". */
export function useAllBadgesQuery(filters?: Omit<BadgeFilters, 'cursor' | 'limit'>) {
  const { listBadges } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.allBadges(filters),
    queryFn: async () =>
      unwrap(
        await fetchAllPages((cursor) => listBadges({ ...filters, limit: 100, cursor })),
        'Failed to fetch badges',
      ).data,
  });
}

export function useBadgeQuery(badgeId: ID) {
  const { getBadge } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.badges.detail(badgeId),
    queryFn: async () => unwrap(await getBadge(badgeId), 'Failed to fetch badge'),
    enabled: !!badgeId,
  });
}

export function usePlayerBadgesQuery(playerId: ID, params?: CursorParams) {
  const { getPlayerBadges } = useMechanicsService();
  return useQuery({
    queryKey: [...queryKeys.badges.playerBadges(playerId), params],
    queryFn: async () => unwrap(await getPlayerBadges(playerId, params), 'Failed to fetch player badges'),
    enabled: !!playerId,
  });
}

export function useCreateBadgeMutation() {
  const queryClient = useQueryClient();
  const { createBadge } = useMechanicsService();
  return useMutation({
    mutationFn: async (data: CreateBadgeData) => unwrap(await createBadge(data), 'Failed to create badge'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.lists() });
      queryClient.invalidateQueries({ queryKey: localKeys.badgeStats() });
    },
  });
}

export function useUpdateBadgeMutation() {
  const queryClient = useQueryClient();
  const { updateBadge } = useMechanicsService();
  return useMutation({
    mutationFn: async ({ badgeId, data }: { badgeId: ID; data: UpdateBadgeData }) =>
      unwrap(await updateBadge(badgeId, data), 'Failed to update badge'),
    onSuccess: (badge, { badgeId }) => {
      queryClient.setQueryData(queryKeys.badges.detail(badgeId), badge);
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.lists() });
      queryClient.invalidateQueries({ queryKey: localKeys.badgeStats() });
    },
  });
}

export function useDeleteBadgeMutation() {
  const queryClient = useQueryClient();
  const { deleteBadge } = useMechanicsService();
  return useMutation({
    mutationFn: async (badgeId: ID) => {
      unwrap(await deleteBadge(badgeId), 'Failed to delete badge');
      return badgeId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.badges.all }),
  });
}

/** Award statistics (GET /badges/stats): per badge, totals, and the last 30 days. */
export function useBadgeStatsQuery() {
  const { getBadgeStats } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.badgeStats(),
    queryFn: async () => unwrap(await getBadgeStats(), 'Failed to fetch badge statistics'),
  });
}

/** 409 codes: badge_already_earned, badge_max_awards_reached, badge_inactive. */
export function useAwardBadgeMutation() {
  const queryClient = useQueryClient();
  const { awardBadge } = useMechanicsService();
  return useMutation({
    mutationFn: async (data: AwardBadgeData) => unwrap(await awardBadge(data), 'Failed to award badge'),
    onSuccess: (_result, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: localKeys.badgeStats() });
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(player_id) });
    },
  });
}

export function useRevokeBadgeMutation() {
  const queryClient = useQueryClient();
  const { revokeBadge } = useMechanicsService();
  return useMutation({
    mutationFn: async ({ playerId, badgeId }: { playerId: ID; badgeId: ID }) => {
      unwrap(await revokeBadge(playerId, badgeId), 'Failed to revoke badge');
    },
    onSuccess: (_result, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(playerId) });
    },
  });
}

// ==================== LEVELS ====================

/** The whole level ladder, sorted by level_number. */
export function useLevelsQuery(filters?: Omit<LevelFilters, 'cursor' | 'limit'>) {
  const { listLevels } = useMechanicsService();
  return useQuery({
    queryKey: [...queryKeys.levels.lists(), filters],
    queryFn: async () => {
      const page = unwrap(
        await fetchAllPages((cursor) => listLevels({ ...filters, limit: 100, cursor })),
        'Failed to fetch levels',
      );
      return [...page.data].sort((a, b) => a.level_number - b.level_number);
    },
  });
}

export function useLevelQuery(levelId: ID) {
  const { getLevel } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.levels.detail(levelId),
    queryFn: async () => unwrap(await getLevel(levelId), 'Failed to fetch level'),
    enabled: !!levelId,
  });
}

/** 409 level_number_taken; 422 xp_required_not_increasing. */
export function useCreateLevelMutation() {
  const queryClient = useQueryClient();
  const { createLevel } = useMechanicsService();
  return useMutation({
    mutationFn: async (data: CreateLevelData) => unwrap(await createLevel(data), 'Failed to create level'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.levels.all }),
  });
}

export function useUpdateLevelMutation() {
  const queryClient = useQueryClient();
  const { updateLevel } = useMechanicsService();
  return useMutation({
    mutationFn: async ({ levelId, data }: { levelId: ID; data: UpdateLevelData }) =>
      unwrap(await updateLevel(levelId, data), 'Failed to update level'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.levels.all }),
  });
}

export function useDeleteLevelMutation() {
  const queryClient = useQueryClient();
  const { deleteLevel } = useMechanicsService();
  return useMutation({
    mutationFn: async (levelId: ID) => {
      unwrap(await deleteLevel(levelId), 'Failed to delete level');
      return levelId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.levels.all }),
  });
}

/**
 * XP and level of up to 100 players at once (GET /progress?player_ids=).
 * Unknown players are omitted; the result is keyed by player id.
 */
export function usePlayersProgressQuery(playerIds: ID[]) {
  const { getPlayersProgress } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.playersProgress(playerIds),
    queryFn: async () => {
      const list = unwrap(await getPlayersProgress(playerIds), 'Failed to fetch player progress');
      return Object.fromEntries(list.data.map((p) => [p.player_id, p]));
    },
    enabled: playerIds.length > 0 && playerIds.length <= 100,
  });
}

// ==================== MISSIONS ====================

export function useMissionsQuery(filters?: MissionFilters) {
  const { listMissions } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.missions.list(filters),
    queryFn: async () => unwrap(await listMissions(filters), 'Failed to fetch missions'),
  });
}

export function useMissionQuery(missionId: ID) {
  const { getMission } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.missions.detail(missionId),
    queryFn: async () => unwrap(await getMission(missionId), 'Failed to fetch mission'),
    enabled: !!missionId,
  });
}

/** Attempts at one mission (?player_id&status&limit&cursor). */
export function useMissionAttemptsQuery(missionId: ID, filters?: MissionAttemptFilters) {
  const { listMissionAttempts } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.missionAttempts(missionId, filters),
    queryFn: async () => unwrap(await listMissionAttempts(missionId, filters), 'Failed to fetch attempts'),
    enabled: !!missionId,
  });
}

/** Completion analytics of every mission, one cursor page (?status&type&limit&cursor). */
export function useMissionStatsListQuery(filters?: MissionStatsFilters) {
  const { listMissionStats } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.missionStatsList(filters),
    queryFn: async () => unwrap(await listMissionStats(filters), 'Failed to fetch mission statistics'),
  });
}

/** Completion analytics of one mission (404 mission_not_found). */
export function useMissionStatsQuery(missionId: ID) {
  const { getMissionStats } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.missionStats(missionId),
    queryFn: async () => unwrap(await getMissionStats(missionId), 'Failed to fetch mission statistics'),
    enabled: !!missionId,
  });
}

export function usePlayerMissionsQuery(playerId: ID, params?: CursorParams) {
  const { getPlayerMissions } = useMechanicsService();
  return useQuery({
    queryKey: [...queryKeys.missions.playerMissions(playerId), params],
    queryFn: async () => unwrap(await getPlayerMissions(playerId, params), 'Failed to fetch player missions'),
    enabled: !!playerId,
  });
}

export function useCreateMissionMutation() {
  const queryClient = useQueryClient();
  const { createMission } = useMechanicsService();
  return useMutation({
    mutationFn: async (data: CreateMissionData) => unwrap(await createMission(data), 'Failed to create mission'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.lists() });
      queryClient.invalidateQueries({ queryKey: localKeys.missionStatsAll() });
    },
  });
}

export function useUpdateMissionMutation() {
  const queryClient = useQueryClient();
  const { updateMission } = useMechanicsService();
  return useMutation({
    mutationFn: async ({ missionId, data }: { missionId: ID; data: UpdateMissionData }) =>
      unwrap(await updateMission(missionId, data), 'Failed to update mission'),
    onSuccess: (mission, { missionId }) => {
      queryClient.setQueryData(queryKeys.missions.detail(missionId), mission);
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.lists() });
      queryClient.invalidateQueries({ queryKey: localKeys.missionStatsAll() });
      queryClient.invalidateQueries({ queryKey: localKeys.missionStats(missionId) });
    },
  });
}

export function useDeleteMissionMutation() {
  const queryClient = useQueryClient();
  const { deleteMission } = useMechanicsService();
  return useMutation({
    mutationFn: async (missionId: ID) => {
      unwrap(await deleteMission(missionId), 'Failed to delete mission');
      return missionId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.missions.all }),
  });
}

function useInvalidateMissionPlayer() {
  const queryClient = useQueryClient();
  return (missionId: ID, playerId: ID) => {
    queryClient.invalidateQueries({ queryKey: queryKeys.missions.detail(missionId) });
    queryClient.invalidateQueries({ queryKey: localKeys.missionStatsAll() });
    queryClient.invalidateQueries({ queryKey: queryKeys.missions.playerMissions(playerId) });
    queryClient.invalidateQueries({ queryKey: queryKeys.players.missions(playerId) });
  };
}

/** 409 mission_not_available / mission_limit_reached / mission_already_started. */
export function useStartMissionMutation() {
  const { startMission } = useMechanicsService();
  const invalidate = useInvalidateMissionPlayer();
  return useMutation({
    mutationFn: async (data: StartMissionData) => unwrap(await startMission(data), 'Failed to start mission'),
    onSuccess: (_attempt, { mission_id, player_id }) => invalidate(mission_id, player_id),
  });
}

export function useUpdateMissionProgressMutation() {
  const { updateMissionProgress } = useMechanicsService();
  const invalidate = useInvalidateMissionPlayer();
  return useMutation({
    mutationFn: async (data: UpdateMissionProgressData) =>
      unwrap(await updateMissionProgress(data), 'Failed to update mission progress'),
    onSuccess: (_result, { mission_id, player_id }) => invalidate(mission_id, player_id),
  });
}

/** 409 mission_not_completed when the target is not reached. */
export function useCompleteMissionMutation() {
  const { completeMission } = useMechanicsService();
  const invalidate = useInvalidateMissionPlayer();
  return useMutation({
    mutationFn: async (data: CompleteMissionData) =>
      unwrap(await completeMission(data), 'Failed to complete mission'),
    onSuccess: (_attempt, { mission_id, player_id }) => invalidate(mission_id, player_id),
  });
}

// ==================== STREAKS ====================

export function useStreaksQuery(filters?: StreakFilters) {
  const { listStreaks } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.streaks.list(filters),
    queryFn: async () => unwrap(await listStreaks(filters), 'Failed to fetch streaks'),
  });
}

export function useStreakQuery(streakId: ID) {
  const { getStreak } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.streaks.detail(streakId),
    queryFn: async () => unwrap(await getStreak(streakId), 'Failed to fetch streak'),
    enabled: !!streakId,
  });
}

/** The player's state in one streak, or null when they never recorded it. */
export function usePlayerStreakQuery(playerId: ID, streakId: ID) {
  const { getPlayerStreak } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.streaks.playerStreak(playerId, streakId),
    queryFn: async () => unwrap(await getPlayerStreak(playerId, streakId), 'Failed to fetch player streak'),
    enabled: !!playerId && !!streakId,
  });
}

export function usePlayerStreaksQuery(playerId: ID, params?: CursorParams) {
  const { getPlayerStreaks } = useMechanicsService();
  return useQuery({
    queryKey: [...queryKeys.streaks.playerStreaks(playerId), params],
    queryFn: async () => unwrap(await getPlayerStreaks(playerId, params), 'Failed to fetch player streaks'),
    enabled: !!playerId,
  });
}

export function useCreateStreakMutation() {
  const queryClient = useQueryClient();
  const { createStreak } = useMechanicsService();
  return useMutation({
    mutationFn: async (data: CreateStreakData) => unwrap(await createStreak(data), 'Failed to create streak'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.streaks.lists() }),
  });
}

export function useUpdateStreakMutation() {
  const queryClient = useQueryClient();
  const { updateStreak } = useMechanicsService();
  return useMutation({
    mutationFn: async ({ streakId, data }: { streakId: ID; data: UpdateStreakData }) =>
      unwrap(await updateStreak(streakId, data), 'Failed to update streak'),
    onSuccess: (streak, { streakId }) => {
      queryClient.setQueryData(queryKeys.streaks.detail(streakId), streak);
      queryClient.invalidateQueries({ queryKey: queryKeys.streaks.lists() });
    },
  });
}

export function useDeleteStreakMutation() {
  const queryClient = useQueryClient();
  const { deleteStreak } = useMechanicsService();
  return useMutation({
    mutationFn: async (streakId: ID) => {
      unwrap(await deleteStreak(streakId), 'Failed to delete streak');
      return streakId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.streaks.all }),
  });
}

export function useRecordStreakActivityMutation() {
  const queryClient = useQueryClient();
  const { recordStreakActivity } = useMechanicsService();
  return useMutation({
    mutationFn: async (data: RecordStreakActivityData) =>
      unwrap(await recordStreakActivity(data), 'Failed to record streak activity'),
    onSuccess: (_result, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: [...queryKeys.streaks.all, 'player', player_id] });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.streaks(player_id) });
    },
  });
}

export function useResetStreakMutation() {
  const queryClient = useQueryClient();
  const { resetStreak } = useMechanicsService();
  return useMutation({
    mutationFn: async ({ playerId, streakId }: { playerId: ID; streakId: ID }) =>
      unwrap(await resetStreak(playerId, streakId), 'Failed to reset streak'),
    onSuccess: (_result, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: [...queryKeys.streaks.all, 'player', playerId] });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.streaks(playerId) });
    },
  });
}

// ==================== LEADERBOARDS ====================

export function useLeaderboardsQuery(filters?: LeaderboardFilters) {
  const { listLeaderboards } = useMechanicsService();
  return useQuery({
    queryKey: [...queryKeys.leaderboards.list(), filters],
    queryFn: async () => unwrap(await listLeaderboards(filters), 'Failed to fetch leaderboards'),
  });
}

export function useLeaderboardQuery(leaderboardId: ID) {
  const { getLeaderboard } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.leaderboards.detail(leaderboardId),
    queryFn: async () => unwrap(await getLeaderboard(leaderboardId), 'Failed to fetch leaderboard'),
    enabled: !!leaderboardId,
  });
}

/** One page of standings (?period=current|<RFC3339>&limit&cursor). */
export function useLeaderboardEntriesQuery(leaderboardId: ID, params?: LeaderboardEntriesParams) {
  const { getLeaderboardEntries } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.leaderboardEntries(leaderboardId, params),
    queryFn: async () =>
      unwrap(await getLeaderboardEntries(leaderboardId, params), 'Failed to fetch leaderboard entries'),
    enabled: !!leaderboardId,
  });
}

/** 404 player_not_ranked when the player has no entry. */
export function usePlayerRankQuery(leaderboardId: ID, playerId: ID, params?: { period?: string; around?: number }) {
  const { getPlayerRank } = useMechanicsService();
  return useQuery({
    queryKey: [...queryKeys.leaderboards.playerRank(leaderboardId, playerId), params],
    queryFn: async () => unwrap(await getPlayerRank(leaderboardId, playerId, params), 'Failed to fetch player rank'),
    enabled: !!leaderboardId && !!playerId,
    retry: false,
  });
}

export function useCreateLeaderboardMutation() {
  const queryClient = useQueryClient();
  const { createLeaderboard } = useMechanicsService();
  return useMutation({
    mutationFn: async (data: CreateLeaderboardData) =>
      unwrap(await createLeaderboard(data), 'Failed to create leaderboard'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: localKeys.leaderboardLists() }),
  });
}

export function useUpdateLeaderboardMutation() {
  const queryClient = useQueryClient();
  const { updateLeaderboard } = useMechanicsService();
  return useMutation({
    mutationFn: async ({ leaderboardId, data }: { leaderboardId: ID; data: UpdateLeaderboardData }) =>
      unwrap(await updateLeaderboard(leaderboardId, data), 'Failed to update leaderboard'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.all }),
  });
}

export function useDeleteLeaderboardMutation() {
  const queryClient = useQueryClient();
  const { deleteLeaderboard } = useMechanicsService();
  return useMutation({
    mutationFn: async (leaderboardId: ID) => {
      unwrap(await deleteLeaderboard(leaderboardId), 'Failed to delete leaderboard');
      return leaderboardId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.all }),
  });
}

/** Recomputes standings from the source ledgers. */
export function useRebuildLeaderboardMutation() {
  const queryClient = useQueryClient();
  const { rebuildLeaderboard } = useMechanicsService();
  return useMutation({
    mutationFn: async (leaderboardId: ID) =>
      unwrap(await rebuildLeaderboard(leaderboardId), 'Failed to rebuild leaderboard'),
    onSuccess: (_result, leaderboardId) =>
      queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.detail(leaderboardId) }),
  });
}

// ==================== REWARDS ====================

export function useRewardsQuery(filters?: RewardFilters) {
  const { listRewards } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.rewards.list(filters),
    queryFn: async () => unwrap(await listRewards(filters), 'Failed to fetch rewards'),
  });
}

export function useRewardQuery(rewardId: ID) {
  const { getReward } = useMechanicsService();
  return useQuery({
    queryKey: queryKeys.rewards.detail(rewardId),
    queryFn: async () => unwrap(await getReward(rewardId), 'Failed to fetch reward'),
    enabled: !!rewardId,
  });
}

/**
 * One claim. While it is pending_payment / refund_pending (paid rewards
 * settle asynchronously) it is polled every `pollMs` (default 2s).
 */
export function useRewardClaimQuery(claimId: ID, options?: { pollMs?: number | false }) {
  const { getRewardClaim } = useMechanicsService();
  const pollMs = options?.pollMs ?? 2000;
  return useQuery({
    queryKey: localKeys.rewardClaim(claimId),
    queryFn: async () => unwrap(await getRewardClaim(claimId), 'Failed to fetch reward claim'),
    enabled: !!claimId,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === 'pending_payment' || status === 'refund_pending' ? pollMs : false;
    },
  });
}

/** A player's reward claims (GET /players/{id}/reward-claims). */
export function usePlayerRewardsQuery(playerId: ID, params?: CursorParams) {
  const { getPlayerRewards } = useMechanicsService();
  return useQuery({
    queryKey: [...queryKeys.rewards.playerRewards(playerId), params],
    queryFn: async () => unwrap(await getPlayerRewards(playerId, params), 'Failed to fetch player rewards'),
    enabled: !!playerId,
  });
}

/** Tenant-wide redemption history, one cursor page (?status&reward_id&player_id&from&to). */
export function useRewardClaimsQuery(filters?: RewardClaimFilters) {
  const { listRewardClaims } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.rewardClaims(filters),
    queryFn: async () => unwrap(await listRewardClaims(filters), 'Failed to fetch reward claims'),
  });
}

/** Claim statistics per reward plus tenant totals (GET /rewards/stats). */
export function useRewardStatsQuery() {
  const { getRewardStats } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.rewardStats(),
    queryFn: async () => unwrap(await getRewardStats(), 'Failed to fetch reward statistics'),
  });
}

export function useCreateRewardMutation() {
  const queryClient = useQueryClient();
  const { createReward } = useMechanicsService();
  return useMutation({
    mutationFn: async (data: CreateRewardData) => unwrap(await createReward(data), 'Failed to create reward'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.rewards.lists() }),
  });
}

export function useUpdateRewardMutation() {
  const queryClient = useQueryClient();
  const { updateReward } = useMechanicsService();
  return useMutation({
    mutationFn: async ({ rewardId, data }: { rewardId: ID; data: UpdateRewardData }) =>
      unwrap(await updateReward(rewardId, data), 'Failed to update reward'),
    onSuccess: (reward, { rewardId }) => {
      queryClient.setQueryData(queryKeys.rewards.detail(rewardId), reward);
      queryClient.invalidateQueries({ queryKey: queryKeys.rewards.lists() });
    },
  });
}

export function useDeleteRewardMutation() {
  const queryClient = useQueryClient();
  const { deleteReward } = useMechanicsService();
  return useMutation({
    mutationFn: async (rewardId: ID) => {
      unwrap(await deleteReward(rewardId), 'Failed to delete reward');
      return rewardId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.rewards.all }),
  });
}

function useInvalidateClaim() {
  const queryClient = useQueryClient();
  return (claim: RewardClaim) => {
    queryClient.setQueryData(localKeys.rewardClaim(claim.id), claim);
    queryClient.invalidateQueries({ queryKey: queryKeys.rewards.playerRewards(claim.player_id) });
    queryClient.invalidateQueries({ queryKey: queryKeys.rewards.lists() });
    queryClient.invalidateQueries({ queryKey: queryKeys.players.rewards(claim.player_id) });
  };
}

/**
 * Claims a reward. The returned claim's status is `claimed` (free reward,
 * 201) or `pending_payment` (paid reward, 202: points are debited
 * asynchronously; poll useRewardClaimQuery). 409 codes include
 * reward_not_available, reward_depleted, player_limit_reached,
 * level_requirement_not_met.
 */
export function useClaimRewardMutation() {
  const { claimReward } = useMechanicsService();
  const invalidate = useInvalidateClaim();
  return useMutation({
    mutationFn: async (data: ClaimRewardData) => unwrap(await claimReward(data), 'Failed to claim reward'),
    onSuccess: (claim) => invalidate(claim),
  });
}

/** Redeems a claimed reward (by claim id). 409 invalid_status_transition / claim_expired. */
export function useRedeemRewardMutation() {
  const { redeemRewardClaim } = useMechanicsService();
  const invalidate = useInvalidateClaim();
  return useMutation({
    mutationFn: async (claimId: ID) => unwrap(await redeemRewardClaim(claimId), 'Failed to redeem reward'),
    onSuccess: (claim) => invalidate(claim),
  });
}

export function useCancelRewardClaimMutation() {
  const { cancelRewardClaim } = useMechanicsService();
  const invalidate = useInvalidateClaim();
  return useMutation({
    mutationFn: async (claimId: ID) => unwrap(await cancelRewardClaim(claimId), 'Failed to cancel claim'),
    onSuccess: (claim) => invalidate(claim),
  });
}

// ==================== WALLET ANALYTICS ====================

/** Tenant-wide totals (GET /wallets/summary). */
export function useWalletSummaryQuery() {
  const { getWalletSummary } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.walletSummary(),
    queryFn: async () => unwrap(await getWalletSummary(), 'Failed to fetch wallet summary'),
  });
}

/** Balance histogram, 10 equal-width buckets (empty when no wallet exists). */
export function useWalletDistributionQuery() {
  const { getWalletDistribution } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.walletDistribution(),
    queryFn: async () => unwrap(await getWalletDistribution(), 'Failed to fetch balance distribution').data,
  });
}

/** Zero-filled daily credited/debited totals, oldest first (UTC days; 422 invalid_range). */
export function useWalletDailyQuery(params?: WalletDailyParams) {
  const { getWalletDaily } = useMechanicsService();
  return useQuery({
    queryKey: localKeys.walletDaily(params),
    queryFn: async () => unwrap(await getWalletDaily(params), 'Failed to fetch daily totals').data,
    retry: false,
  });
}

// ==================== AI DRAFTS ====================

/**
 * onCreated handler for AIGenerateDialog: refreshes the list (and stats) of
 * the mechanic a draft was created as.
 */
export function useInvalidateCreatedDraft() {
  const queryClient = useQueryClient();
  return useCallback(
    (kind: string) => {
      const roots: Record<string, readonly unknown[]> = {
        badge: queryKeys.badges.all,
        level: queryKeys.levels.all,
        mission: queryKeys.missions.all,
        reward: queryKeys.rewards.all,
      };
      const root = roots[kind];
      if (root) queryClient.invalidateQueries({ queryKey: root });
    },
    [queryClient],
  );
}

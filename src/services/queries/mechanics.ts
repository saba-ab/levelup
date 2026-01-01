import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useMechanicsService } from '../api/mechanics';
import { queryKeys } from './keys';
import {
  Badge,
  CreateBadgeData,
  UpdateBadgeData,
  Level,
  CreateLevelData,
  Mission,
  CreateMissionData,
  Streak,
  CreateStreakData,
  Leaderboard,
  CreateLeaderboardData,
  Reward,
  CreateRewardData,
  Rule,
  CreateRuleData,
  MechanicsFilters,
  WalletTransactionFilters,
  RuleExecutionFilters,
  StartMissionData,
  UpdateMissionProgressData,
  RecordStreakActivityData,
  ClaimRewardData,
  CreditWalletData,
  DebitWalletData,
  TransferPointsData,
  ExecuteRulesData,
  AwardBadgeData,
} from '../api/types';

// ==================== BADGES ====================

export function useBadgesQuery(filters?: MechanicsFilters) {
  const { listBadges } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.badges.list(filters),
    queryFn: async () => {
      const response = await listBadges(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch badges');
      return response.data!;
    },
  });
}

export function useBadgeQuery(badgeId: number) {
  const { getBadge } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.badges.detail(badgeId),
    queryFn: async () => {
      const response = await getBadge(badgeId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch badge');
      return response.data!;
    },
    enabled: !!badgeId,
  });
}

export function usePlayerBadgesQuery(playerId: number) {
  const { getPlayerBadges } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.badges.playerBadges(playerId),
    queryFn: async () => {
      const response = await getPlayerBadges(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player badges');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function useCreateBadgeMutation() {
  const queryClient = useQueryClient();
  const { createBadge } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: CreateBadgeData) => {
      const response = await createBadge(data);
      if (!response.success) throw new Error(response.error || 'Failed to create badge');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.lists() });
    },
  });
}

export function useUpdateBadgeMutation() {
  const queryClient = useQueryClient();
  const { updateBadge } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ badgeId, data }: { badgeId: number; data: UpdateBadgeData }) => {
      const response = await updateBadge(badgeId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update badge');
      return response.data!;
    },
    onMutate: async ({ badgeId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.badges.detail(badgeId) });
      const previousBadge = queryClient.getQueryData<Badge>(queryKeys.badges.detail(badgeId));

      if (previousBadge) {
        queryClient.setQueryData<Badge>(queryKeys.badges.detail(badgeId), { ...previousBadge, ...data });
      }

      return { previousBadge };
    },
    onError: (err, { badgeId }, context) => {
      if (context?.previousBadge) {
        queryClient.setQueryData(queryKeys.badges.detail(badgeId), context.previousBadge);
      }
    },
    onSettled: (data, error, { badgeId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.detail(badgeId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.lists() });
    },
  });
}

export function useDeleteBadgeMutation() {
  const queryClient = useQueryClient();
  const { deleteBadge } = useMechanicsService();

  return useMutation({
    mutationFn: async (badgeId: number) => {
      const response = await deleteBadge(badgeId);
      if (!response.success) throw new Error(response.error || 'Failed to delete badge');
      return badgeId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.all });
    },
  });
}

export function useAwardBadgeMutation() {
  const queryClient = useQueryClient();
  const { awardBadge } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: AwardBadgeData) => {
      const response = await awardBadge(data);
      if (!response.success) throw new Error(response.error || 'Failed to award badge');
      return response.data!;
    },
    onSuccess: (data, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(player_id) });
    },
  });
}

export function useRevokeBadgeMutation() {
  const queryClient = useQueryClient();
  const { revokeBadge } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ playerId, badgeId }: { playerId: number; badgeId: number }) => {
      const response = await revokeBadge(playerId, badgeId);
      if (!response.success) throw new Error(response.error || 'Failed to revoke badge');
    },
    onSuccess: (data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(playerId) });
    },
  });
}


// ==================== LEVELS ====================

export function useLevelsQuery(filters?: MechanicsFilters) {
  const { listLevels } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.levels.list(filters),
    queryFn: async () => {
      const response = await listLevels(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch levels');
      return response.data!;
    },
  });
}

export function useLevelQuery(levelId: number) {
  const { getLevel } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.levels.detail(levelId),
    queryFn: async () => {
      const response = await getLevel(levelId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch level');
      return response.data!;
    },
    enabled: !!levelId,
  });
}

export function useCreateLevelMutation() {
  const queryClient = useQueryClient();
  const { createLevel } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: CreateLevelData) => {
      const response = await createLevel(data);
      if (!response.success) throw new Error(response.error || 'Failed to create level');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.levels.lists() });
    },
  });
}

export function useUpdateLevelMutation() {
  const queryClient = useQueryClient();
  const { updateLevel } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ levelId, data }: { levelId: number; data: Partial<CreateLevelData> }) => {
      const response = await updateLevel(levelId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update level');
      return response.data!;
    },
    onMutate: async ({ levelId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.levels.detail(levelId) });
      const previousLevel = queryClient.getQueryData<Level>(queryKeys.levels.detail(levelId));

      if (previousLevel) {
        queryClient.setQueryData<Level>(queryKeys.levels.detail(levelId), { ...previousLevel, ...data });
      }

      return { previousLevel };
    },
    onError: (err, { levelId }, context) => {
      if (context?.previousLevel) {
        queryClient.setQueryData(queryKeys.levels.detail(levelId), context.previousLevel);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.levels.all });
    },
  });
}

export function useDeleteLevelMutation() {
  const queryClient = useQueryClient();
  const { deleteLevel } = useMechanicsService();

  return useMutation({
    mutationFn: async (levelId: number) => {
      const response = await deleteLevel(levelId);
      if (!response.success) throw new Error(response.error || 'Failed to delete level');
      return levelId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.levels.all });
    },
  });
}

// ==================== MISSIONS ====================

export function useMissionsQuery(filters?: MechanicsFilters) {
  const { listMissions } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.missions.list(filters),
    queryFn: async () => {
      const response = await listMissions(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch missions');
      return response.data!;
    },
  });
}

export function useMissionQuery(missionId: number) {
  const { getMission } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.missions.detail(missionId),
    queryFn: async () => {
      const response = await getMission(missionId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch mission');
      return response.data!;
    },
    enabled: !!missionId,
  });
}

export function usePlayerMissionsQuery(playerId: number) {
  const { getPlayerMissions } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.missions.playerMissions(playerId),
    queryFn: async () => {
      const response = await getPlayerMissions(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player missions');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function useCreateMissionMutation() {
  const queryClient = useQueryClient();
  const { createMission } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: CreateMissionData) => {
      const response = await createMission(data);
      if (!response.success) throw new Error(response.error || 'Failed to create mission');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.lists() });
    },
  });
}

export function useUpdateMissionMutation() {
  const queryClient = useQueryClient();
  const { updateMission } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ missionId, data }: { missionId: number; data: Partial<CreateMissionData> }) => {
      const response = await updateMission(missionId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update mission');
      return response.data!;
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.all });
    },
  });
}

export function useDeleteMissionMutation() {
  const queryClient = useQueryClient();
  const { deleteMission } = useMechanicsService();

  return useMutation({
    mutationFn: async (missionId: number) => {
      const response = await deleteMission(missionId);
      if (!response.success) throw new Error(response.error || 'Failed to delete mission');
      return missionId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.all });
    },
  });
}

export function useStartMissionMutation() {
  const queryClient = useQueryClient();
  const { startMission } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: StartMissionData) => {
      const response = await startMission(data);
      if (!response.success) throw new Error(response.error || 'Failed to start mission');
      return response.data!;
    },
    onSuccess: (data, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.playerMissions(player_id) });
    },
  });
}

export function useUpdateMissionProgressMutation() {
  const queryClient = useQueryClient();
  const { updateMissionProgress } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ playerId, missionId, data }: { playerId: number; missionId: number; data: UpdateMissionProgressData }) => {
      const response = await updateMissionProgress(playerId, missionId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update mission progress');
      return response.data!;
    },
    onSuccess: (data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.playerMissions(playerId) });
    },
  });
}

export function useCompleteMissionMutation() {
  const queryClient = useQueryClient();
  const { completeMission } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ playerId, missionId }: { playerId: number; missionId: number }) => {
      const response = await completeMission(playerId, missionId);
      if (!response.success) throw new Error(response.error || 'Failed to complete mission');
      return response.data!;
    },
    onSuccess: (data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.playerMissions(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.level(playerId) });
    },
  });
}

// ==================== STREAKS ====================

export function useStreaksQuery(filters?: MechanicsFilters) {
  const { listStreaks } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.streaks.list(filters),
    queryFn: async () => {
      const response = await listStreaks(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch streaks');
      return response.data!;
    },
  });
}

export function useStreakQuery(streakId: number) {
  const { getStreak } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.streaks.detail(streakId),
    queryFn: async () => {
      const response = await getStreak(streakId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch streak');
      return response.data!;
    },
    enabled: !!streakId,
  });
}

export function usePlayerStreakQuery(playerId: number, streakId: number) {
  const { getPlayerStreak } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.streaks.playerStreak(playerId, streakId),
    queryFn: async () => {
      const response = await getPlayerStreak(playerId, streakId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player streak');
      return response.data!;
    },
    enabled: !!playerId && !!streakId,
  });
}

export function usePlayerStreaksQuery(playerId: number) {
  const { getPlayerStreaks } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.streaks.playerStreaks(playerId),
    queryFn: async () => {
      const response = await getPlayerStreaks(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player streaks');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function useCreateStreakMutation() {
  const queryClient = useQueryClient();
  const { createStreak } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: CreateStreakData) => {
      const response = await createStreak(data);
      if (!response.success) throw new Error(response.error || 'Failed to create streak');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.streaks.lists() });
    },
  });
}

export function useUpdateStreakMutation() {
  const queryClient = useQueryClient();
  const { updateStreak } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ streakId, data }: { streakId: number; data: Partial<CreateStreakData> }) => {
      const response = await updateStreak(streakId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update streak');
      return response.data!;
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.streaks.all });
    },
  });
}

export function useDeleteStreakMutation() {
  const queryClient = useQueryClient();
  const { deleteStreak } = useMechanicsService();

  return useMutation({
    mutationFn: async (streakId: number) => {
      const response = await deleteStreak(streakId);
      if (!response.success) throw new Error(response.error || 'Failed to delete streak');
      return streakId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.streaks.all });
    },
  });
}

export function useRecordStreakActivityMutation() {
  const queryClient = useQueryClient();
  const { recordStreakActivity } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: RecordStreakActivityData) => {
      const response = await recordStreakActivity(data);
      if (!response.success) throw new Error(response.error || 'Failed to record streak activity');
      return response.data!;
    },
    onSuccess: (data, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.streaks.playerStreaks(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(player_id) });
    },
  });
}

export function useResetStreakMutation() {
  const queryClient = useQueryClient();
  const { resetStreak } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ playerId, streakId }: { playerId: number; streakId: number }) => {
      const response = await resetStreak(playerId, streakId);
      if (!response.success) throw new Error(response.error || 'Failed to reset streak');
      return response.data!;
    },
    onSuccess: (data, { playerId, streakId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.streaks.playerStreak(playerId, streakId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.streaks.playerStreaks(playerId) });
    },
  });
}

// ==================== LEADERBOARDS ====================

export function useLeaderboardsQuery() {
  const { listLeaderboards } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.leaderboards.list(),
    queryFn: async () => {
      const response = await listLeaderboards();
      if (!response.success) throw new Error(response.error || 'Failed to fetch leaderboards');
      return response.data!;
    },
  });
}

export function useLeaderboardQuery(leaderboardId: number) {
  const { getLeaderboard } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.leaderboards.detail(leaderboardId),
    queryFn: async () => {
      const response = await getLeaderboard(leaderboardId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch leaderboard');
      return response.data!;
    },
    enabled: !!leaderboardId,
  });
}

export function useLeaderboardEntriesQuery(leaderboardId: number, limit = 100, offset = 0) {
  const { getLeaderboardEntries } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.leaderboards.entries(leaderboardId, limit, offset),
    queryFn: async () => {
      const response = await getLeaderboardEntries(leaderboardId, limit, offset);
      if (!response.success) throw new Error(response.error || 'Failed to fetch leaderboard entries');
      return response.data!;
    },
    enabled: !!leaderboardId,
  });
}

export function usePlayerRankQuery(leaderboardId: number, playerId: number) {
  const { getPlayerRank } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.leaderboards.playerRank(leaderboardId, playerId),
    queryFn: async () => {
      const response = await getPlayerRank(leaderboardId, playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player rank');
      return response.data!;
    },
    enabled: !!leaderboardId && !!playerId,
  });
}

export function useCreateLeaderboardMutation() {
  const queryClient = useQueryClient();
  const { createLeaderboard } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: CreateLeaderboardData) => {
      const response = await createLeaderboard(data);
      if (!response.success) throw new Error(response.error || 'Failed to create leaderboard');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.list() });
    },
  });
}

export function useUpdateLeaderboardMutation() {
  const queryClient = useQueryClient();
  const { updateLeaderboard } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ leaderboardId, data }: { leaderboardId: number; data: Partial<CreateLeaderboardData> }) => {
      const response = await updateLeaderboard(leaderboardId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update leaderboard');
      return response.data!;
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.all });
    },
  });
}

export function useDeleteLeaderboardMutation() {
  const queryClient = useQueryClient();
  const { deleteLeaderboard } = useMechanicsService();

  return useMutation({
    mutationFn: async (leaderboardId: number) => {
      const response = await deleteLeaderboard(leaderboardId);
      if (!response.success) throw new Error(response.error || 'Failed to delete leaderboard');
      return leaderboardId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.all });
    },
  });
}

// ==================== REWARDS ====================

export function useRewardsQuery(filters?: MechanicsFilters) {
  const { listRewards } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.rewards.list(filters),
    queryFn: async () => {
      const response = await listRewards(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch rewards');
      return response.data!;
    },
  });
}

export function useRewardQuery(rewardId: number) {
  const { getReward } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.rewards.detail(rewardId),
    queryFn: async () => {
      const response = await getReward(rewardId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch reward');
      return response.data!;
    },
    enabled: !!rewardId,
  });
}

export function usePlayerRewardsQuery(playerId: number) {
  const { getPlayerRewards } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.rewards.playerRewards(playerId),
    queryFn: async () => {
      const response = await getPlayerRewards(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player rewards');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function useCreateRewardMutation() {
  const queryClient = useQueryClient();
  const { createReward } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: CreateRewardData) => {
      const response = await createReward(data);
      if (!response.success) throw new Error(response.error || 'Failed to create reward');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rewards.lists() });
    },
  });
}

export function useUpdateRewardMutation() {
  const queryClient = useQueryClient();
  const { updateReward } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ rewardId, data }: { rewardId: number; data: Partial<CreateRewardData> }) => {
      const response = await updateReward(rewardId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update reward');
      return response.data!;
    },
    onMutate: async ({ rewardId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.rewards.detail(rewardId) });
      const previousReward = queryClient.getQueryData<Reward>(queryKeys.rewards.detail(rewardId));

      if (previousReward) {
        queryClient.setQueryData<Reward>(queryKeys.rewards.detail(rewardId), { ...previousReward, ...data });
      }

      return { previousReward };
    },
    onError: (err, { rewardId }, context) => {
      if (context?.previousReward) {
        queryClient.setQueryData(queryKeys.rewards.detail(rewardId), context.previousReward);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rewards.all });
    },
  });
}

export function useDeleteRewardMutation() {
  const queryClient = useQueryClient();
  const { deleteReward } = useMechanicsService();

  return useMutation({
    mutationFn: async (rewardId: number) => {
      const response = await deleteReward(rewardId);
      if (!response.success) throw new Error(response.error || 'Failed to delete reward');
      return rewardId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rewards.all });
    },
  });
}

export function useClaimRewardMutation() {
  const queryClient = useQueryClient();
  const { claimReward } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: ClaimRewardData) => {
      const response = await claimReward(data);
      if (!response.success) throw new Error(response.error || 'Failed to claim reward');
      return response.data!;
    },
    onSuccess: (data, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rewards.playerRewards(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(player_id) });
    },
  });
}

export function useRedeemRewardMutation() {
  const queryClient = useQueryClient();
  const { redeemReward } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ playerId, rewardId }: { playerId: number; rewardId: number }) => {
      const response = await redeemReward(playerId, rewardId);
      if (!response.success) throw new Error(response.error || 'Failed to redeem reward');
      return response.data!;
    },
    onSuccess: (data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rewards.playerRewards(playerId) });
    },
  });
}

// ==================== WALLETS ====================

export function usePlayerWalletQuery(playerId: number) {
  const { getPlayerWallet } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.wallets.player(playerId),
    queryFn: async () => {
      const response = await getPlayerWallet(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player wallet');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function useWalletTransactionsQuery(playerId: number, filters?: WalletTransactionFilters) {
  const { getWalletTransactions } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.wallets.transactions(playerId, filters),
    queryFn: async () => {
      const response = await getWalletTransactions(playerId, filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch wallet transactions');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function useCreditWalletMutation() {
  const queryClient = useQueryClient();
  const { creditWallet } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: CreditWalletData) => {
      const response = await creditWallet(data);
      if (!response.success) throw new Error(response.error || 'Failed to credit wallet');
      return response.data!;
    },
    onSuccess: (data, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(player_id) });
    },
  });
}

export function useDebitWalletMutation() {
  const queryClient = useQueryClient();
  const { debitWallet } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: DebitWalletData) => {
      const response = await debitWallet(data);
      if (!response.success) throw new Error(response.error || 'Failed to debit wallet');
      return response.data!;
    },
    onSuccess: (data, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(player_id) });
    },
  });
}

export function useTransferPointsMutation() {
  const queryClient = useQueryClient();
  const { transferPoints } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: TransferPointsData) => {
      const response = await transferPoints(data);
      if (!response.success) throw new Error(response.error || 'Failed to transfer points');
      return response.data!;
    },
    onSuccess: (data, { from_player_id, to_player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(from_player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(to_player_id) });
    },
  });
}

// ==================== RULES ====================

export function useRulesQuery(filters?: MechanicsFilters) {
  const { listRules } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.rules.list(filters),
    queryFn: async () => {
      const response = await listRules(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch rules');
      return response.data!;
    },
  });
}

export function useRuleQuery(ruleId: number) {
  const { getRule } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.rules.detail(ruleId),
    queryFn: async () => {
      const response = await getRule(ruleId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch rule');
      return response.data!;
    },
    enabled: !!ruleId,
  });
}

export function useRuleExecutionsQuery(filters?: RuleExecutionFilters) {
  const { getRuleExecutions } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.rules.executions(filters),
    queryFn: async () => {
      const response = await getRuleExecutions(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch rule executions');
      return response.data!;
    },
  });
}

export function useCreateRuleMutation() {
  const queryClient = useQueryClient();
  const { createRule } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: CreateRuleData) => {
      const response = await createRule(data);
      if (!response.success) throw new Error(response.error || 'Failed to create rule');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rules.lists() });
    },
  });
}

export function useUpdateRuleMutation() {
  const queryClient = useQueryClient();
  const { updateRule } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ ruleId, data }: { ruleId: number; data: Partial<CreateRuleData> }) => {
      const response = await updateRule(ruleId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update rule');
      return response.data!;
    },
    onMutate: async ({ ruleId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.rules.detail(ruleId) });
      const previousRule = queryClient.getQueryData<Rule>(queryKeys.rules.detail(ruleId));

      if (previousRule) {
        queryClient.setQueryData<Rule>(queryKeys.rules.detail(ruleId), { ...previousRule, ...data });
      }

      return { previousRule };
    },
    onError: (err, { ruleId }, context) => {
      if (context?.previousRule) {
        queryClient.setQueryData(queryKeys.rules.detail(ruleId), context.previousRule);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rules.all });
    },
  });
}

export function useDeleteRuleMutation() {
  const queryClient = useQueryClient();
  const { deleteRule } = useMechanicsService();

  return useMutation({
    mutationFn: async (ruleId: number) => {
      const response = await deleteRule(ruleId);
      if (!response.success) throw new Error(response.error || 'Failed to delete rule');
      return ruleId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rules.all });
    },
  });
}

export function useCreateRuleVersionMutation() {
  const queryClient = useQueryClient();
  const { createRuleVersion } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ ruleId, data }: { ruleId: number; data: CreateRuleData }) => {
      const response = await createRuleVersion(ruleId, data);
      if (!response.success) throw new Error(response.error || 'Failed to create rule version');
      return response.data!;
    },
    onSuccess: (data, { ruleId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rules.detail(ruleId) });
    },
  });
}

export function useExecuteRulesMutation() {
  const queryClient = useQueryClient();
  const { executeRules } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: ExecuteRulesData) => {
      const response = await executeRules(data);
      if (!response.success) throw new Error(response.error || 'Failed to execute rules');
      return response.data!;
    },
    onSuccess: (data, { player_id }) => {
      // Invalidate all player-related queries since rules can affect anything
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.playerMissions(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.level(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.rules.executions() });
    },
  });
}

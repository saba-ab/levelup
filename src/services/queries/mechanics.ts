import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useMechanicsService } from '../api/mechanics';
import { queryKeys } from './keys';
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
  CreateLeaderboardData,
  Reward,
  CreateRewardData,
  MechanicsFilters,
  PaginatedResponse,
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

export function useBadgeQuery(badgeId: string) {
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
    mutationFn: async ({ badgeId, data }: { badgeId: string; data: Partial<CreateBadgeData> }) => {
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
    mutationFn: async (badgeId: string) => {
      const response = await deleteBadge(badgeId);
      if (!response.success) throw new Error(response.error || 'Failed to delete badge');
      return badgeId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.all });
    },
  });
}

// ==================== LEVELS ====================

export function useLevelsQuery() {
  const { listLevels } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.levels.list(),
    queryFn: async () => {
      const response = await listLevels();
      if (!response.success) throw new Error(response.error || 'Failed to fetch levels');
      return response.data!;
    },
  });
}

export function useLevelQuery(levelId: string) {
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
      queryClient.invalidateQueries({ queryKey: queryKeys.levels.list() });
    },
  });
}

export function useUpdateLevelMutation() {
  const queryClient = useQueryClient();
  const { updateLevel } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ levelId, data }: { levelId: string; data: Partial<CreateLevelData> }) => {
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
    mutationFn: async (levelId: string) => {
      const response = await deleteLevel(levelId);
      if (!response.success) throw new Error(response.error || 'Failed to delete level');
      return levelId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.levels.all });
    },
  });
}

export function useReorderLevelsMutation() {
  const queryClient = useQueryClient();
  const { reorderLevels } = useMechanicsService();

  return useMutation({
    mutationFn: async (levelIds: string[]) => {
      const response = await reorderLevels(levelIds);
      if (!response.success) throw new Error(response.error || 'Failed to reorder levels');
      return response.data!;
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

export function useMissionQuery(missionId: string) {
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
    mutationFn: async ({ missionId, data }: { missionId: string; data: Partial<CreateMissionData> }) => {
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
    mutationFn: async (missionId: string) => {
      const response = await deleteMission(missionId);
      if (!response.success) throw new Error(response.error || 'Failed to delete mission');
      return missionId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.missions.all });
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
    mutationFn: async ({ streakId, data }: { streakId: string; data: Partial<CreateStreakData> }) => {
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
    mutationFn: async (streakId: string) => {
      const response = await deleteStreak(streakId);
      if (!response.success) throw new Error(response.error || 'Failed to delete streak');
      return streakId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.streaks.all });
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

export function useLeaderboardEntriesQuery(leaderboardId: string, limit = 100, offset = 0) {
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

export function useDeleteLeaderboardMutation() {
  const queryClient = useQueryClient();
  const { deleteLeaderboard } = useMechanicsService();

  return useMutation({
    mutationFn: async (leaderboardId: string) => {
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

export function useRewardQuery(rewardId: string) {
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
    mutationFn: async ({ rewardId, data }: { rewardId: string; data: Partial<CreateRewardData> }) => {
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
    mutationFn: async (rewardId: string) => {
      const response = await deleteReward(rewardId);
      if (!response.success) throw new Error(response.error || 'Failed to delete reward');
      return rewardId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rewards.all });
    },
  });
}

export function useRedeemRewardMutation() {
  const queryClient = useQueryClient();
  const { redeemReward } = useMechanicsService();

  return useMutation({
    mutationFn: async ({ playerId, rewardId }: { playerId: string; rewardId: string }) => {
      const response = await redeemReward(playerId, rewardId);
      if (!response.success) throw new Error(response.error || 'Failed to redeem reward');
      return response.data!;
    },
    onSuccess: (data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.rewards.all });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.stats(playerId) });
    },
  });
}

// ==================== POINT WALLETS ====================

export function usePointWalletsQuery() {
  const { listPointWallets } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.pointWallets.list(),
    queryFn: async () => {
      const response = await listPointWallets();
      if (!response.success) throw new Error(response.error || 'Failed to fetch point wallets');
      return response.data!;
    },
  });
}

export function useCreatePointWalletMutation() {
  const queryClient = useQueryClient();
  const { createPointWallet } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: { name: string; description?: string; currency?: string }) => {
      const response = await createPointWallet(data);
      if (!response.success) throw new Error(response.error || 'Failed to create point wallet');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pointWallets.list() });
    },
  });
}

export function useDeletePointWalletMutation() {
  const queryClient = useQueryClient();
  const { deletePointWallet } = useMechanicsService();

  return useMutation({
    mutationFn: async (walletId: string) => {
      const response = await deletePointWallet(walletId);
      if (!response.success) throw new Error(response.error || 'Failed to delete point wallet');
      return walletId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pointWallets.all });
    },
  });
}
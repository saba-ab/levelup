import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { usePlayersService } from '../api/players';
import { queryKeys } from './keys';
import {
  Player,
  PlayerFilters,
  CreatePlayerData,
  UpdatePlayerData,
  AwardPointsData,
  PaginatedResponse,
} from '../api/types';

// ==================== QUERIES ====================

export function usePlayersQuery(filters?: PlayerFilters) {
  const { listPlayers } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.list(filters),
    queryFn: async () => {
      const response = await listPlayers(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch players');
      return response.data!;
    },
  });
}

export function usePlayerQuery(playerId: string) {
  const { getPlayer } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.detail(playerId),
    queryFn: async () => {
      const response = await getPlayer(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function usePlayerStatsQuery(playerId: string) {
  const { getPlayerStats } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.stats(playerId),
    queryFn: async () => {
      const response = await getPlayerStats(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player stats');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function usePlayerBadgesQuery(playerId: string) {
  const { getPlayerBadges } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.badges(playerId),
    queryFn: async () => {
      const response = await getPlayerBadges(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player badges');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function usePlayerMissionsQuery(playerId: string, status?: string) {
  const { getPlayerMissions } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.missions(playerId, status),
    queryFn: async () => {
      const response = await getPlayerMissions(playerId, status);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player missions');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function usePlayerStreaksQuery(playerId: string) {
  const { getPlayerStreaks } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.streaks(playerId),
    queryFn: async () => {
      const response = await getPlayerStreaks(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player streaks');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

// ==================== MUTATIONS ====================

export function useCreatePlayerMutation() {
  const queryClient = useQueryClient();
  const { createPlayer } = usePlayersService();

  return useMutation({
    mutationFn: async (data: CreatePlayerData) => {
      const response = await createPlayer(data);
      if (!response.success) throw new Error(response.error || 'Failed to create player');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.lists() });
    },
  });
}

export function useUpdatePlayerMutation() {
  const queryClient = useQueryClient();
  const { updatePlayer } = usePlayersService();

  return useMutation({
    mutationFn: async ({ playerId, data }: { playerId: string; data: UpdatePlayerData }) => {
      const response = await updatePlayer(playerId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update player');
      return response.data!;
    },
    onMutate: async ({ playerId, data }) => {
      // Cancel outgoing refetches
      await queryClient.cancelQueries({ queryKey: queryKeys.players.detail(playerId) });

      // Snapshot previous value
      const previousPlayer = queryClient.getQueryData<Player>(queryKeys.players.detail(playerId));

      // Optimistically update
      if (previousPlayer) {
        queryClient.setQueryData<Player>(queryKeys.players.detail(playerId), {
          ...previousPlayer,
          ...data,
        });
      }

      return { previousPlayer };
    },
    onError: (err, { playerId }, context) => {
      // Rollback on error
      if (context?.previousPlayer) {
        queryClient.setQueryData(queryKeys.players.detail(playerId), context.previousPlayer);
      }
    },
    onSettled: (data, error, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.lists() });
    },
  });
}

export function useDeletePlayerMutation() {
  const queryClient = useQueryClient();
  const { deletePlayer } = usePlayersService();

  return useMutation({
    mutationFn: async (playerId: string) => {
      const response = await deletePlayer(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to delete player');
      return playerId;
    },
    onMutate: async (playerId) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.players.lists() });

      // Snapshot and optimistically remove from lists
      const previousLists = queryClient.getQueriesData<PaginatedResponse<Player>>({
        queryKey: queryKeys.players.lists(),
      });

      queryClient.setQueriesData<PaginatedResponse<Player>>(
        { queryKey: queryKeys.players.lists() },
        (old) => {
          if (!old) return old;
          return {
            ...old,
            data: old.data.filter((p) => p.id !== playerId),
            meta: { ...old.meta, total: old.meta.total - 1 },
          };
        }
      );

      return { previousLists };
    },
    onError: (err, playerId, context) => {
      // Restore previous lists
      context?.previousLists.forEach(([queryKey, data]) => {
        queryClient.setQueryData(queryKey, data);
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.all });
    },
  });
}

export function useAwardBadgeMutation() {
  const queryClient = useQueryClient();
  const { awardBadge } = usePlayersService();

  return useMutation({
    mutationFn: async ({ playerId, badgeId, reason }: { playerId: string; badgeId: string; reason?: string }) => {
      const response = await awardBadge(playerId, badgeId, reason);
      if (!response.success) throw new Error(response.error || 'Failed to award badge');
      return response.data!;
    },
    onSuccess: (data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.stats(playerId) });
    },
  });
}

export function useRevokeBadgeMutation() {
  const queryClient = useQueryClient();
  const { revokeBadge } = usePlayersService();

  return useMutation({
    mutationFn: async ({ playerId, badgeId }: { playerId: string; badgeId: string }) => {
      const response = await revokeBadge(playerId, badgeId);
      if (!response.success) throw new Error(response.error || 'Failed to revoke badge');
    },
    onSuccess: (data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.stats(playerId) });
    },
  });
}

export function useAwardPointsMutation() {
  const queryClient = useQueryClient();
  const { awardPoints } = usePlayersService();

  return useMutation({
    mutationFn: async (data: AwardPointsData) => {
      const response = await awardPoints(data);
      if (!response.success) throw new Error(response.error || 'Failed to award points');
      return response.data!;
    },
    onSuccess: (result, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.stats(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.transactions(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.all });
    },
  });
}

export function useDeductPointsMutation() {
  const queryClient = useQueryClient();
  const { deductPoints } = usePlayersService();

  return useMutation({
    mutationFn: async (data: AwardPointsData) => {
      const response = await deductPoints(data);
      if (!response.success) throw new Error(response.error || 'Failed to deduct points');
      return response.data!;
    },
    onSuccess: (result, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.stats(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.transactions(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.all });
    },
  });
}
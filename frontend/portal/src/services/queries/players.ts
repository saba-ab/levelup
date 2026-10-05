import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { usePlayersService } from '../api/players';
import { useMechanicsService } from '../api/mechanics';
import { queryKeys } from './keys';
import {
  Player,
  PlayerFilters,
  CreatePlayerData,
  UpdatePlayerData,
  PaginatedResponse,
  AwardBadgeData,
  GrantXpData,
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

export function usePlayerQuery(playerId: number) {
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

export function usePlayerBadgesQuery(playerId: number) {
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

export function usePlayerMissionsQuery(playerId: number) {
  const { getPlayerMissions } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.missions(playerId),
    queryFn: async () => {
      const response = await getPlayerMissions(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player missions');
      return response.data!;
    },
    enabled: !!playerId,
  });
}

export function usePlayerStreaksQuery(playerId: number) {
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

export function usePlayerLevelQuery(playerId: number) {
  const { getPlayerLevel } = useMechanicsService();

  return useQuery({
    queryKey: queryKeys.players.level(playerId),
    queryFn: async () => {
      const response = await getPlayerLevel(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch player level');
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
    mutationFn: async ({ playerId, data }: { playerId: number; data: UpdatePlayerData }) => {
      const response = await updatePlayer(playerId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update player');
      return response.data!;
    },
    onMutate: async ({ playerId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.players.detail(playerId) });
      const previousPlayer = queryClient.getQueryData<Player>(queryKeys.players.detail(playerId));

      if (previousPlayer) {
        queryClient.setQueryData<Player>(queryKeys.players.detail(playerId), {
          ...previousPlayer,
          ...data,
        });
      }

      return { previousPlayer };
    },
    onError: (err, { playerId }, context) => {
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
    mutationFn: async (playerId: number) => {
      const response = await deletePlayer(playerId);
      if (!response.success) throw new Error(response.error || 'Failed to delete player');
      return playerId;
    },
    onMutate: async (playerId) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.players.lists() });

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
  const { awardBadge } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: AwardBadgeData) => {
      const response = await awardBadge(data);
      if (!response.success) throw new Error(response.error || 'Failed to award badge');
      return response.data!;
    },
    onSuccess: (data, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(player_id) });
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
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(playerId) });
    },
  });
}

export function useGrantXpMutation() {
  const queryClient = useQueryClient();
  const { grantXp } = useMechanicsService();

  return useMutation({
    mutationFn: async (data: GrantXpData) => {
      const response = await grantXp(data);
      if (!response.success) throw new Error(response.error || 'Failed to grant XP');
      return response.data!;
    },
    onSuccess: (result, { player_id }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.level(player_id) });
      queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.all });
    },
  });
}

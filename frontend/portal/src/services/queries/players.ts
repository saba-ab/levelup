import { useQuery, useQueries, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import type { ApiResponse, ValidationErrors } from '@/hooks/useApi';
import { usePlayersService } from '../api/players';
import { fetchAllPages } from '../api/pagination';
import { queryKeys } from './keys';
import type {
  ID,
  CursorPage,
  CursorParams,
  Player,
  PlayerFilters,
  CreatePlayerData,
  UpdatePlayerData,
  PlayerXpGrantData,
  Wallet,
  WalletTransactionFilters,
  CreditWalletData,
  DebitWalletData,
  TransferPointsData,
} from '../api/types';

/** Error thrown by these hooks: keeps the problem+json code and field errors. */
export class PlayerApiError extends Error {
  code: string | null;
  status: number;
  validationErrors: ValidationErrors | null;

  constructor(res: ApiResponse<unknown>, fallback: string) {
    super(res.error || fallback);
    this.name = 'PlayerApiError';
    this.code = res.code;
    this.status = res.status;
    this.validationErrors = res.validationErrors;
  }
}

function unwrap<T>(res: ApiResponse<T>, fallback: string): T {
  if (!res.success || res.data === null) throw new PlayerApiError(res, fallback);
  return res.data;
}

/** Local keys not in the shared factory. */
const localKeys = {
  xpGrants: (id: ID, params?: CursorParams) => [...queryKeys.players.detail(id), 'xp-grants', params] as const,
  rewardClaims: (id: ID) => [...queryKeys.players.detail(id), 'reward-claims'] as const,
};

/** Per-player lists are small; load them whole (capped) so counts are real. */
const PER_PLAYER_MAX = 500;

// ==================== QUERIES ====================

export function usePlayersQuery(filters?: PlayerFilters, options?: { enabled?: boolean }) {
  const { listPlayers } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.list(filters),
    queryFn: async () => unwrap(await listPlayers(filters), 'Failed to fetch players'),
    placeholderData: keepPreviousData,
    enabled: options?.enabled ?? true,
  });
}

export function usePlayerQuery(playerId: ID | undefined) {
  const { getPlayer } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.detail(playerId ?? ''),
    queryFn: async () => unwrap(await getPlayer(playerId!), 'Failed to fetch player'),
    enabled: !!playerId,
  });
}

export function usePlayerBadgesQuery(playerId: ID | undefined) {
  const { getPlayerBadges } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.badges(playerId ?? ''),
    queryFn: async () =>
      unwrap(
        await fetchAllPages((cursor) => getPlayerBadges(playerId!, { limit: 100, cursor }), PER_PLAYER_MAX),
        'Failed to fetch player badges',
      ).data,
    enabled: !!playerId,
  });
}

export function usePlayerMissionsQuery(playerId: ID | undefined) {
  const { getPlayerMissions } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.missions(playerId ?? ''),
    queryFn: async () =>
      unwrap(
        await fetchAllPages((cursor) => getPlayerMissions(playerId!, { limit: 100, cursor }), PER_PLAYER_MAX),
        'Failed to fetch player missions',
      ).data,
    enabled: !!playerId,
  });
}

export function usePlayerStreaksQuery(playerId: ID | undefined) {
  const { getPlayerStreaks } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.streaks(playerId ?? ''),
    queryFn: async () =>
      unwrap(
        await fetchAllPages((cursor) => getPlayerStreaks(playerId!, { limit: 100, cursor }), PER_PLAYER_MAX),
        'Failed to fetch player streaks',
      ).data,
    enabled: !!playerId,
  });
}

/** Reward claims of one player. */
export function usePlayerRewardClaimsQuery(playerId: ID | undefined) {
  const { getPlayerRewards } = usePlayersService();

  return useQuery({
    queryKey: localKeys.rewardClaims(playerId ?? ''),
    queryFn: async () =>
      unwrap(
        await fetchAllPages((cursor) => getPlayerRewards(playerId!, { limit: 100, cursor }), PER_PLAYER_MAX),
        'Failed to fetch reward claims',
      ).data,
    enabled: !!playerId,
  });
}

/** XP + level progress (GET /players/{id}/progress). */
export function usePlayerLevelQuery(playerId: ID | undefined) {
  const { getPlayerProgress } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.players.level(playerId ?? ''),
    queryFn: async () => unwrap(await getPlayerProgress(playerId!), 'Failed to fetch player progress'),
    enabled: !!playerId,
  });
}

export const usePlayerProgressQuery = usePlayerLevelQuery;

export function usePlayerXpGrantsQuery(playerId: ID | undefined, params?: CursorParams) {
  const { getPlayerXpGrants } = usePlayersService();

  return useQuery({
    queryKey: localKeys.xpGrants(playerId ?? '', params),
    queryFn: async () => unwrap(await getPlayerXpGrants(playerId!, params), 'Failed to fetch XP grants'),
    enabled: !!playerId,
    placeholderData: keepPreviousData,
  });
}

export function usePlayerWalletQuery(playerId: ID | undefined) {
  const { getPlayerWallet } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.wallets.player(playerId ?? ''),
    queryFn: async () => unwrap(await getPlayerWallet(playerId!), 'Failed to fetch wallet'),
    enabled: !!playerId,
  });
}

/** Wallets of several players (one request each), keyed by player id. */
export function usePlayerWalletsQuery(playerIds: ID[]) {
  const { getPlayerWallet } = usePlayersService();

  return useQueries({
    queries: playerIds.map((id) => ({
      queryKey: queryKeys.wallets.player(id),
      queryFn: async () => unwrap(await getPlayerWallet(id), 'Failed to fetch wallet'),
    })),
    combine: (results) => {
      const byPlayer: Record<ID, Wallet> = {};
      results.forEach((r, i) => {
        if (r.data) byPlayer[playerIds[i]] = r.data;
      });
      return {
        byPlayer,
        isLoading: results.some((r) => r.isLoading),
        isError: results.some((r) => r.isError),
      };
    },
  });
}

export function useWalletTransactionsQuery(playerId: ID | undefined, filters?: WalletTransactionFilters) {
  const { getWalletTransactions } = usePlayersService();

  return useQuery({
    queryKey: queryKeys.wallets.transactions(playerId ?? '', filters),
    queryFn: async () => unwrap(await getWalletTransactions(playerId!, filters), 'Failed to fetch transactions'),
    enabled: !!playerId,
    placeholderData: keepPreviousData,
  });
}

// ==================== MUTATIONS ====================

export function useCreatePlayerMutation() {
  const queryClient = useQueryClient();
  const { createPlayer } = usePlayersService();

  return useMutation({
    mutationFn: async (data: CreatePlayerData) => unwrap(await createPlayer(data), 'Failed to create player'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.lists() });
    },
  });
}

export function useUpdatePlayerMutation() {
  const queryClient = useQueryClient();
  const { updatePlayer } = usePlayersService();

  return useMutation({
    mutationFn: async ({ playerId, data }: { playerId: ID; data: UpdatePlayerData }) =>
      unwrap(await updatePlayer(playerId, data), 'Failed to update player'),
    onSuccess: (player) => {
      queryClient.setQueryData(queryKeys.players.detail(player.id), player);
    },
    onSettled: (_data, _error, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.lists() });
    },
  });
}

/** Activate or deactivate a player. */
export function useSetPlayerActiveMutation() {
  const queryClient = useQueryClient();
  const { activatePlayer, deactivatePlayer } = usePlayersService();

  return useMutation({
    mutationFn: async ({ playerId, active }: { playerId: ID; active: boolean }) =>
      unwrap(
        await (active ? activatePlayer(playerId) : deactivatePlayer(playerId)),
        active ? 'Failed to activate player' : 'Failed to deactivate player',
      ),
    onSuccess: (player) => {
      queryClient.setQueryData(queryKeys.players.detail(player.id), player);
    },
    onSettled: (_data, _error, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.players.lists() });
    },
  });
}

export function useDeletePlayerMutation() {
  const queryClient = useQueryClient();
  const { deletePlayer } = usePlayersService();

  return useMutation({
    mutationFn: async (playerId: ID) => {
      const res = await deletePlayer(playerId);
      if (!res.success) throw new PlayerApiError(res, 'Failed to delete player');
      return playerId;
    },
    onMutate: async (playerId) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.players.lists() });
      const previousLists = queryClient.getQueriesData<CursorPage<Player>>({ queryKey: queryKeys.players.lists() });
      queryClient.setQueriesData<CursorPage<Player>>({ queryKey: queryKeys.players.lists() }, (old) =>
        old ? { ...old, data: old.data.filter((p) => p.id !== playerId) } : old,
      );
      return { previousLists };
    },
    onError: (_err, _playerId, context) => {
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
  const { awardPlayerBadge } = usePlayersService();

  return useMutation({
    mutationFn: async ({ playerId, badgeId }: { playerId: ID; badgeId: ID }) =>
      unwrap(await awardPlayerBadge(playerId, badgeId), 'Failed to award badge'),
    onSuccess: (_data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(playerId) });
    },
  });
}

export function useRevokeBadgeMutation() {
  const queryClient = useQueryClient();
  const { revokePlayerBadge } = usePlayersService();

  return useMutation({
    mutationFn: async ({ playerId, badgeId }: { playerId: ID; badgeId: ID }) => {
      const res = await revokePlayerBadge(playerId, badgeId);
      if (!res.success) throw new PlayerApiError(res, 'Failed to revoke badge');
    },
    onSuccess: (_data, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.badges(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.badges.playerBadges(playerId) });
    },
  });
}

export function useGrantXpMutation() {
  const queryClient = useQueryClient();
  const { grantPlayerXp } = usePlayersService();

  return useMutation({
    mutationFn: async ({ playerId, ...data }: PlayerXpGrantData & { playerId: ID }) =>
      unwrap(await grantPlayerXp(playerId, data), 'Failed to grant XP'),
    onSuccess: (_result, { playerId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.players.detail(playerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.leaderboards.all });
    },
  });
}

function invalidateWallet(queryClient: ReturnType<typeof useQueryClient>, playerId: ID) {
  queryClient.invalidateQueries({ queryKey: queryKeys.wallets.player(playerId) });
}

/** Credit a wallet. Sends an Idempotency-Key; errors carry `code` (PlayerApiError). */
export function useCreditWalletMutation() {
  const queryClient = useQueryClient();
  const { creditWallet } = usePlayersService();

  return useMutation({
    mutationFn: async (data: CreditWalletData) => unwrap(await creditWallet(data), 'Failed to credit wallet'),
    onSuccess: (_entry, { player_id }) => invalidateWallet(queryClient, player_id),
  });
}

/** Debit a wallet. Fails with code `insufficient_balance` when the balance is too low. */
export function useDebitWalletMutation() {
  const queryClient = useQueryClient();
  const { debitWallet } = usePlayersService();

  return useMutation({
    mutationFn: async (data: DebitWalletData) => unwrap(await debitWallet(data), 'Failed to debit wallet'),
    onSuccess: (_entry, { player_id }) => invalidateWallet(queryClient, player_id),
  });
}

/** Move points between two players. Fails with `insufficient_balance` or `self_transfer`. */
export function useTransferPointsMutation() {
  const queryClient = useQueryClient();
  const { transferPoints } = usePlayersService();

  return useMutation({
    mutationFn: async (data: TransferPointsData) => unwrap(await transferPoints(data), 'Failed to transfer points'),
    onSuccess: (_result, { from_player_id, to_player_id }) => {
      invalidateWallet(queryClient, from_player_id);
      invalidateWallet(queryClient, to_player_id);
    },
  });
}

import { useApi, type ApiOptions } from '@/hooks/useApi';
import { useCallback } from 'react';
import type {
  ID,
  CursorPage,
  CursorParams,
  Player,
  PlayerBadge,
  PlayerMission,
  PlayerStreak,
  PlayerReward,
  CreatePlayerData,
  UpdatePlayerData,
  PlayerFilters,
  PlayerProgressSummary,
  PlayerXpGrantEntry,
  PlayerXpGrantData,
  PlayerXpGrantResult,
  PlayerBadgeAwardResult,
  Wallet,
  WalletTransaction,
  WalletTransactionFilters,
  CreditWalletData,
  DebitWalletData,
  TransferPointsData,
  TransferPointsResult,
} from './types';
import type { WalletSummary } from './models/players';
import { API_VERSION, PLAYER_ENDPOINTS, WALLET_ENDPOINTS, BADGE_ENDPOINTS, toQuery } from '@/lib/api-routes';

/** Batch and tenant-wide reads added after the shared endpoint maps. */
export const PLAYER_BATCH_ENDPOINTS = {
  /** GET ?player_ids=a,b (max 100): ProgressResp per known player (progression.view). */
  PROGRESS: `${API_VERSION}/progress`,
  /** GET ?player_ids=a,b (max 100): WalletResp per known player, zero views included (points.view_wallet). */
  WALLETS: `${API_VERSION}/wallets`,
  /** GET: tenant-wide wallet summary (points.view_wallet). */
  WALLET_SUMMARY: `${API_VERSION}/wallets/summary`,
} as const;

const idsQuery = (playerIds: ID[]) => toQuery({ player_ids: playerIds.join(',') });

/** Money mutations report their own errors (insufficient_balance etc.) in the UI. */
const MONEY_OPTIONS: ApiOptions = { idempotencyKey: true, showErrorToast: false };

export function usePlayersService() {
  const api = useApi();

  // ---------- players ----------

  const listPlayers = useCallback(async (filters?: PlayerFilters) => {
    return api.get<CursorPage<Player>>(`${PLAYER_ENDPOINTS.LIST}${toQuery(filters)}`);
  }, [api]);

  const getPlayer = useCallback(async (playerId: ID) => {
    return api.get<Player>(PLAYER_ENDPOINTS.SHOW(playerId));
  }, [api]);

  const getPlayerByExternalId = useCallback(async (externalId: string) => {
    return api.get<Player>(PLAYER_ENDPOINTS.BY_EXTERNAL_ID(externalId));
  }, [api]);

  const createPlayer = useCallback(async (data: CreatePlayerData) => {
    return api.post<Player>(PLAYER_ENDPOINTS.CREATE, data, { showErrorToast: false });
  }, [api]);

  const updatePlayer = useCallback(async (playerId: ID, data: UpdatePlayerData) => {
    return api.patch<Player>(PLAYER_ENDPOINTS.UPDATE(playerId), data, { showErrorToast: false });
  }, [api]);

  const deletePlayer = useCallback(async (playerId: ID) => {
    return api.delete(PLAYER_ENDPOINTS.DELETE(playerId));
  }, [api]);

  const activatePlayer = useCallback(async (playerId: ID) => {
    return api.post<Player>(PLAYER_ENDPOINTS.ACTIVATE(playerId));
  }, [api]);

  const deactivatePlayer = useCallback(async (playerId: ID) => {
    return api.post<Player>(PLAYER_ENDPOINTS.DEACTIVATE(playerId));
  }, [api]);

  // ---------- per-player views (owned by other modules) ----------

  const getPlayerBadges = useCallback(async (playerId: ID, params?: CursorParams) => {
    return api.get<CursorPage<PlayerBadge>>(`${PLAYER_ENDPOINTS.BADGES(playerId)}${toQuery(params)}`);
  }, [api]);

  const getPlayerMissions = useCallback(async (playerId: ID, params?: CursorParams) => {
    return api.get<CursorPage<PlayerMission>>(`${PLAYER_ENDPOINTS.MISSIONS(playerId)}${toQuery(params)}`);
  }, [api]);

  const getPlayerStreaks = useCallback(async (playerId: ID, params?: CursorParams) => {
    return api.get<CursorPage<PlayerStreak>>(`${PLAYER_ENDPOINTS.STREAKS(playerId)}${toQuery(params)}`);
  }, [api]);

  /** Reward claims (GET /players/{id}/reward-claims). */
  const getPlayerRewards = useCallback(async (playerId: ID, params?: CursorParams) => {
    return api.get<CursorPage<PlayerReward>>(`${PLAYER_ENDPOINTS.REWARD_CLAIMS(playerId)}${toQuery(params)}`);
  }, [api]);

  const getPlayerProgress = useCallback(async (playerId: ID) => {
    return api.get<PlayerProgressSummary>(PLAYER_ENDPOINTS.PROGRESS(playerId));
  }, [api]);

  const getPlayerXpGrants = useCallback(async (playerId: ID, params?: CursorParams) => {
    return api.get<CursorPage<PlayerXpGrantEntry>>(`${PLAYER_ENDPOINTS.XP_GRANTS(playerId)}${toQuery(params)}`);
  }, [api]);

  const grantPlayerXp = useCallback(async (playerId: ID, data: PlayerXpGrantData) => {
    return api.post<PlayerXpGrantResult>(PLAYER_ENDPOINTS.XP(playerId), data, { idempotencyKey: true });
  }, [api]);

  const awardPlayerBadge = useCallback(async (playerId: ID, badgeId: ID) => {
    return api.post<PlayerBadgeAwardResult>(BADGE_ENDPOINTS.AWARD(badgeId), { player_id: playerId }, { idempotencyKey: true });
  }, [api]);

  const revokePlayerBadge = useCallback(async (playerId: ID, badgeId: ID) => {
    return api.delete(BADGE_ENDPOINTS.REVOKE(badgeId, playerId));
  }, [api]);

  // ---------- batch reads (one request per page of players) ----------

  /** Unknown or foreign players are omitted. */
  const getPlayersProgress = useCallback(async (playerIds: ID[]) => {
    return api.get<{ data: PlayerProgressSummary[] }>(`${PLAYER_BATCH_ENDPOINTS.PROGRESS}${idsQuery(playerIds)}`, {
      showErrorToast: false,
    });
  }, [api]);

  /** Never-opened wallets come back as zero views (opened=false). */
  const getPlayersWallets = useCallback(async (playerIds: ID[]) => {
    return api.get<{ data: Wallet[] }>(`${PLAYER_BATCH_ENDPOINTS.WALLETS}${idsQuery(playerIds)}`, {
      showErrorToast: false,
    });
  }, [api]);

  const getWalletSummary = useCallback(async () => {
    return api.get<WalletSummary>(PLAYER_BATCH_ENDPOINTS.WALLET_SUMMARY, { showErrorToast: false });
  }, [api]);

  // ---------- wallet ----------

  /** Returns a zero view with opened=false when the player has no wallet yet. */
  const getPlayerWallet = useCallback(async (playerId: ID) => {
    return api.get<Wallet>(PLAYER_ENDPOINTS.WALLET(playerId));
  }, [api]);

  const getWalletTransactions = useCallback(async (playerId: ID, filters?: WalletTransactionFilters) => {
    return api.get<CursorPage<WalletTransaction>>(`${PLAYER_ENDPOINTS.WALLET_TRANSACTIONS(playerId)}${toQuery(filters)}`);
  }, [api]);

  const creditWallet = useCallback(async ({ player_id, ...body }: CreditWalletData) => {
    return api.post<WalletTransaction>(PLAYER_ENDPOINTS.WALLET_CREDIT(player_id), body, MONEY_OPTIONS);
  }, [api]);

  const debitWallet = useCallback(async ({ player_id, ...body }: DebitWalletData) => {
    return api.post<WalletTransaction>(PLAYER_ENDPOINTS.WALLET_DEBIT(player_id), body, MONEY_OPTIONS);
  }, [api]);

  const transferPoints = useCallback(async (data: TransferPointsData) => {
    return api.post<TransferPointsResult>(WALLET_ENDPOINTS.TRANSFER, data, MONEY_OPTIONS);
  }, [api]);

  return {
    listPlayers,
    getPlayer,
    getPlayerByExternalId,
    createPlayer,
    updatePlayer,
    deletePlayer,
    activatePlayer,
    deactivatePlayer,
    getPlayerBadges,
    getPlayerMissions,
    getPlayerStreaks,
    getPlayerRewards,
    getPlayerProgress,
    getPlayerXpGrants,
    grantPlayerXp,
    awardPlayerBadge,
    revokePlayerBadge,
    getPlayerWallet,
    getWalletTransactions,
    creditWallet,
    debitWallet,
    transferPoints,
    getPlayersProgress,
    getPlayersWallets,
    getWalletSummary,
  };
}

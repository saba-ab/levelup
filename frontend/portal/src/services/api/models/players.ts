// players + points/wallet models (Go API contract: backend/api/docs/swagger.json,
// backend/internal/modules/{player,points,progression}/internal/transport).
import type { CursorParams, ID } from './common';
import type { PlayerBadge } from './mechanics';

// ==================== PLAYERS ====================

/** PlayerResp. There is no first/last name or avatar: derive initials from display_name. */
export interface Player {
  id: ID;
  tenant_id: ID;
  external_id: string;
  display_name: string | null;
  email: string | null;
  attributes: Record<string, unknown>;
  is_active: boolean;
  created_by: ID | null;
  version: number;
  created_at: string;
  updated_at: string;
}

/** CreateReq (POST /players). */
export interface CreatePlayerData {
  external_id: string;
  display_name?: string;
  email?: string;
  attributes?: Record<string, unknown>;
}

/** UpdateReq (PATCH /players/{id}): send only changed fields. */
export interface UpdatePlayerData {
  display_name?: string;
  email?: string;
  attributes?: Record<string, unknown>;
  is_active?: boolean;
}

/** GET /players query. */
export interface PlayerFilters extends CursorParams {
  search?: string;
  is_active?: boolean;
}

// ==================== PROGRESSION (per player) ====================

/** LevelRefResp. */
export interface PlayerLevelRef {
  id: ID;
  level_number: number;
  name: string;
  xp_required: number;
}

/** ProgressResp (GET /players/{id}/progress). */
export interface PlayerProgressSummary {
  player_id: ID;
  total_xp: number;
  current_level: PlayerLevelRef | null;
  next_level: PlayerLevelRef | null;
  xp_to_next: number | null;
  progress_percent: number;
}

/** GrantResp (GET /players/{id}/xp-grants items). */
export interface PlayerXpGrantEntry {
  id: ID;
  player_id: ID;
  idempotency_key: string;
  amount: number;
  description: string;
  source_kind: string;
  source_id: string;
  activity_id?: string;
  occurred_at: string;
  created_by?: ID;
  created_at: string;
}

/** GrantXPReq (POST /players/{id}/xp, needs an Idempotency-Key). */
export interface PlayerXpGrantData {
  amount: number;
  description?: string;
}

/** GrantXPResp. */
export interface PlayerXpGrantResult {
  grant: PlayerXpGrantEntry;
  replayed: boolean;
  total_xp: number;
  level_number: number;
  levels_reached: PlayerLevelRef[];
}

/** AwardResp (POST /badges/{id}/award). */
export interface PlayerBadgeAwardResult {
  award_id: ID;
  idempotency_key: string;
  status: string;
  replay: boolean;
  player_badge: PlayerBadge;
}

// ==================== WALLETS ====================

/**
 * WalletResp. GET returns a zero view with opened=false when the player has
 * no wallet yet (id is then empty and timestamps are omitted).
 */
export interface Wallet {
  id: ID;
  player_id: ID;
  balance: number;
  lifetime_earned: number;
  lifetime_spent: number;
  is_active: boolean;
  opened: boolean;
  version: number;
  created_at?: string;
  updated_at?: string;
}

export type WalletCreditKind = 'earn' | 'bonus' | 'reward' | 'adjustment';
export type WalletDebitKind = 'spend' | 'redeem' | 'penalty' | 'expire';
/** Every ledger kind, including the ones only the system writes. */
export type WalletTransactionType = WalletCreditKind | WalletDebitKind | 'transfer' | 'refund';
export type WalletDirection = 'credit' | 'debit';

export const WALLET_CREDIT_KINDS: WalletCreditKind[] = ['earn', 'bonus', 'reward', 'adjustment'];
export const WALLET_DEBIT_KINDS: WalletDebitKind[] = ['spend', 'redeem', 'penalty', 'expire'];

/** EntryResp: one ledger entry. */
export interface WalletTransaction {
  id: ID;
  wallet_id: ID;
  player_id: ID;
  kind: WalletTransactionType;
  direction: WalletDirection;
  amount: number;
  balance_before: number;
  balance_after: number;
  description?: string;
  source_kind?: string;
  source_id?: string;
  activity_id?: string;
  transfer_id?: ID;
  reversal_of?: ID;
  created_by?: ID;
  occurred_at: string;
  created_at: string;
}

/** CreditReq; player_id goes in the path, not the body. */
export interface CreditWalletData {
  player_id: ID;
  amount: number;
  kind: WalletCreditKind;
  description?: string;
}

/** DebitReq; player_id goes in the path, not the body. */
export interface DebitWalletData {
  player_id: ID;
  amount: number;
  kind: WalletDebitKind;
  description?: string;
}

/** TransferReq (POST /wallets/transfer). */
export interface TransferPointsData {
  from_player_id: ID;
  to_player_id: ID;
  amount: number;
  description?: string;
}

/** TransferResp. */
export interface TransferPointsResult {
  transfer_id: ID;
  from: WalletTransaction;
  to: WalletTransaction;
}

/** GET /players/{id}/wallet/transactions query. */
export interface WalletTransactionFilters extends CursorParams {
  kind?: WalletTransactionType;
  direction?: WalletDirection;
}

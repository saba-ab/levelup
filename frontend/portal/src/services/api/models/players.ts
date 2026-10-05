// players models (Go API contract, see docs/rewrite and backend/api/docs/swagger.json)
import type { PaginationParams, Timestamps } from './common';

// ==================== PLAYERS ====================

export interface Player extends Timestamps {
  id: number;
  tenant_id: number;
  external_id: string;
  email: string;
  first_name: string;
  last_name: string;
  display_name: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
  is_active: boolean;
  created_by: number;
}

export interface CreatePlayerData {
  external_id: string;
  email?: string;
  first_name?: string;
  last_name?: string;
  display_name?: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdatePlayerData {
  email?: string;
  first_name?: string;
  last_name?: string;
  display_name?: string;
  avatar_url?: string;
  metadata?: Record<string, unknown>;
  is_active?: boolean;
}

export interface PlayerFilters extends PaginationParams {
  search?: string;
  is_active?: boolean;
}

// ==================== WALLETS ====================

export interface Wallet extends Timestamps {
  id: number;
  player_id: number;
  balance: number;
  lifetime_earned: number;
  lifetime_spent: number;
}

export type WalletTransactionType = 'credit' | 'debit' | 'transfer_in' | 'transfer_out' | 'mission_reward' | 'level_bonus' | 'refund' | 'reward_purchase' | 'penalty';

export interface WalletTransaction extends Timestamps {
  id: number;
  wallet_id: number;
  player_id: number;
  amount: number;
  type: WalletTransactionType;
  description?: string;
  reference_type?: string;
  reference_id?: number;
  balance_before: number;
  balance_after: number;
  metadata?: Record<string, unknown>;
}

export interface CreditWalletData {
  player_id: number;
  amount: number;
  description?: string;
  type?: 'credit' | 'transfer_in' | 'mission_reward' | 'level_bonus' | 'refund';
  reference_type?: string;
  reference_id?: number;
  metadata?: Record<string, unknown>;
}

export interface DebitWalletData {
  player_id: number;
  amount: number;
  description?: string;
  type?: 'debit' | 'transfer_out' | 'reward_purchase' | 'penalty';
  reference_type?: string;
  reference_id?: number;
  metadata?: Record<string, unknown>;
}

export interface TransferPointsData {
  from_player_id: number;
  to_player_id: number;
  amount: number;
  description?: string;
}

export interface WalletTransactionFilters extends PaginationParams {
  type?: WalletTransactionType;
  date_from?: string;
  date_to?: string;
}

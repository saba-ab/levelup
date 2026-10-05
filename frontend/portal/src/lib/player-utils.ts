// Player-related display helpers. Levels, XP and balances come from the API
// (GET /players/{id}/progress and /wallet); nothing here invents numbers.
import type { Player, WalletTransactionType } from '@/services/api/types';

export type ActivityType = 'points' | 'badge' | 'mission' | 'level' | 'streak' | 'reward' | 'xp';

/** Name to show for a player: display_name, else email, else external_id. */
export function getPlayerName(player: Pick<Player, 'display_name' | 'email' | 'external_id'>): string {
  return player.display_name?.trim() || player.email || player.external_id;
}

/** Up to two initials from a display name ("Ana Diaz" -> "AD", "ana@x.io" -> "A"). */
export function getPlayerInitials(name: string | null | undefined): string {
  const clean = (name ?? '').trim();
  if (!clean) return '?';
  const words = clean.split(/[\s._-]+/).filter(Boolean);
  if (words.length >= 2 && !clean.includes('@')) {
    return (words[0][0] + words[1][0]).toUpperCase();
  }
  return clean[0].toUpperCase();
}

/** @deprecated use getPlayerInitials. */
export const getPlayerInitial = getPlayerInitials;

const LEVEL_COLORS = [
  'border-amber-700 text-amber-700 bg-amber-700/10',
  'border-gray-400 text-gray-500 bg-gray-400/10',
  'border-yellow-500 text-yellow-600 bg-yellow-500/10',
  'border-slate-400 text-slate-500 bg-slate-300/10',
  'border-cyan-400 text-cyan-500 bg-cyan-400/10',
];

/** Badge colour for a level, by its level_number (tenants name levels freely). */
export function getLevelColor(levelNumber: number | null | undefined): string {
  if (!levelNumber || levelNumber < 1) return 'border-border text-muted-foreground';
  return LEVEL_COLORS[Math.min(levelNumber, LEVEL_COLORS.length) - 1];
}

export function getActivityIcon(type: ActivityType | string): string {
  switch (type) {
    case 'points':
      return '⚡';
    case 'xp':
      return '✨';
    case 'badge':
      return '🏅';
    case 'mission':
      return '🎯';
    case 'level':
      return '🎖️';
    case 'streak':
      return '🔥';
    case 'reward':
      return '🎁';
    default:
      return '📌';
  }
}

const KIND_LABELS: Record<WalletTransactionType, string> = {
  earn: 'Earn',
  bonus: 'Bonus',
  reward: 'Reward',
  adjustment: 'Adjustment',
  spend: 'Spend',
  redeem: 'Redeem',
  penalty: 'Penalty',
  expire: 'Expire',
  transfer: 'Transfer',
  refund: 'Refund',
};

export function getTransactionKindLabel(kind: string): string {
  return KIND_LABELS[kind as WalletTransactionType] ?? kind;
}

export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '-';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleString();
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return '-';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleDateString();
}

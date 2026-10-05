/**
 * API route constants for the Go backend (backend/api/docs/swagger.json is
 * the source of truth; every path here exists there).
 *
 * Ids are UUID strings. Lists take ?limit=&cursor= and return
 * { data, next_cursor } (see CursorPage in services/api/models/common.ts).
 */

export const API_VERSION = '/api/v1';

type Id = string;
const enc = (v: string) => encodeURIComponent(v);

export const AUTH_ENDPOINTS = {
  LOGIN: `${API_VERSION}/auth/login`,
  REGISTER: `${API_VERSION}/auth/register`,
  LOGOUT: `${API_VERSION}/auth/logout`,
  ME: `${API_VERSION}/auth/me`,
  REFRESH: `${API_VERSION}/auth/refresh`,
} as const;

export const USER_ENDPOINTS = {
  LIST: `${API_VERSION}/users`,
  CREATE: `${API_VERSION}/users`,
  SHOW: (id: Id) => `${API_VERSION}/users/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/users/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/users/${id}`,
  ROLES: (id: Id) => `${API_VERSION}/users/${id}/roles`,
} as const;

export const TENANT_ENDPOINTS = {
  CURRENT: `${API_VERSION}/tenant`,
} as const;

export const PLAYER_ENDPOINTS = {
  LIST: `${API_VERSION}/players`,
  CREATE: `${API_VERSION}/players`,
  SHOW: (id: Id) => `${API_VERSION}/players/${id}`,
  BY_EXTERNAL_ID: (externalId: string) => `${API_VERSION}/players/by-external-id/${enc(externalId)}`,
  UPDATE: (id: Id) => `${API_VERSION}/players/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/players/${id}`,
  ACTIVATE: (id: Id) => `${API_VERSION}/players/${id}/activate`,
  DEACTIVATE: (id: Id) => `${API_VERSION}/players/${id}/deactivate`,
  // Per-player views owned by other modules.
  BADGES: (id: Id) => `${API_VERSION}/players/${id}/badges`,
  MISSIONS: (id: Id) => `${API_VERSION}/players/${id}/missions`,
  STREAKS: (id: Id) => `${API_VERSION}/players/${id}/streaks`,
  REWARD_CLAIMS: (id: Id) => `${API_VERSION}/players/${id}/reward-claims`,
  PROGRESS: (id: Id) => `${API_VERSION}/players/${id}/progress`,
  XP: (id: Id) => `${API_VERSION}/players/${id}/xp`,
  XP_GRANTS: (id: Id) => `${API_VERSION}/players/${id}/xp-grants`,
  WALLET: (id: Id) => `${API_VERSION}/players/${id}/wallet`,
  WALLET_TRANSACTIONS: (id: Id) => `${API_VERSION}/players/${id}/wallet/transactions`,
  WALLET_CREDIT: (id: Id) => `${API_VERSION}/players/${id}/wallet/credit`,
  WALLET_DEBIT: (id: Id) => `${API_VERSION}/players/${id}/wallet/debit`,
} as const;

export const WALLET_ENDPOINTS = {
  TRANSFER: `${API_VERSION}/wallets/transfer`,
} as const;

export const BADGE_ENDPOINTS = {
  LIST: `${API_VERSION}/badges`,
  CREATE: `${API_VERSION}/badges`,
  SHOW: (id: Id) => `${API_VERSION}/badges/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/badges/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/badges/${id}`,
  AWARD: (id: Id) => `${API_VERSION}/badges/${id}/award`,
  REVOKE: (id: Id, playerId: Id) => `${API_VERSION}/badges/${id}/players/${playerId}`,
} as const;

export const LEVEL_ENDPOINTS = {
  LIST: `${API_VERSION}/levels`,
  CREATE: `${API_VERSION}/levels`,
  SHOW: (id: Id) => `${API_VERSION}/levels/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/levels/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/levels/${id}`,
} as const;

export const MISSION_ENDPOINTS = {
  LIST: `${API_VERSION}/missions`,
  CREATE: `${API_VERSION}/missions`,
  SHOW: (id: Id) => `${API_VERSION}/missions/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/missions/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/missions/${id}`,
  ATTEMPTS: (id: Id) => `${API_VERSION}/missions/${id}/attempts`,
  START: (id: Id) => `${API_VERSION}/missions/${id}/start`,
  PROGRESS: (id: Id) => `${API_VERSION}/missions/${id}/progress`,
  COMPLETE: (id: Id) => `${API_VERSION}/missions/${id}/complete`,
} as const;

export const STREAK_ENDPOINTS = {
  LIST: `${API_VERSION}/streaks`,
  CREATE: `${API_VERSION}/streaks`,
  SHOW: (id: Id) => `${API_VERSION}/streaks/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/streaks/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/streaks/${id}`,
  RECORD: (id: Id) => `${API_VERSION}/streaks/${id}/record`,
  RESET: (id: Id, playerId: Id) => `${API_VERSION}/streaks/${id}/players/${playerId}/reset`,
} as const;

export const REWARD_ENDPOINTS = {
  LIST: `${API_VERSION}/rewards`,
  CREATE: `${API_VERSION}/rewards`,
  SHOW: (id: Id) => `${API_VERSION}/rewards/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/rewards/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/rewards/${id}`,
  CLAIM: (id: Id) => `${API_VERSION}/rewards/${id}/claim`,
  CLAIM_SHOW: (claimId: Id) => `${API_VERSION}/rewards/claims/${claimId}`,
  CLAIM_REDEEM: (claimId: Id) => `${API_VERSION}/rewards/claims/${claimId}/redeem`,
  CLAIM_CANCEL: (claimId: Id) => `${API_VERSION}/rewards/claims/${claimId}/cancel`,
} as const;

export const LEADERBOARD_ENDPOINTS = {
  LIST: `${API_VERSION}/leaderboards`,
  CREATE: `${API_VERSION}/leaderboards`,
  SHOW: (id: Id) => `${API_VERSION}/leaderboards/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/leaderboards/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/leaderboards/${id}`,
  ENTRIES: (id: Id) => `${API_VERSION}/leaderboards/${id}/entries`,
  PLAYER: (id: Id, playerId: Id) => `${API_VERSION}/leaderboards/${id}/players/${playerId}`,
  REBUILD: (id: Id) => `${API_VERSION}/leaderboards/${id}/rebuild`,
} as const;

/** Event types (the trigger catalogue). The path stays /events. */
export const EVENT_ENDPOINTS = {
  LIST: `${API_VERSION}/events`,
  CREATE: `${API_VERSION}/events`,
  SHOW: (id: Id) => `${API_VERSION}/events/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/events/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/events/${id}`,
  CATEGORIES: `${API_VERSION}/event-categories`,
} as const;

/** Activity ingestion: what tenant systems send; rules react asynchronously. */
export const ACTIVITY_ENDPOINTS = {
  LIST: `${API_VERSION}/activities`,
  INGEST: `${API_VERSION}/activities`,
  BATCH: `${API_VERSION}/activities/batch`,
  SHOW: (id: Id) => `${API_VERSION}/activities/${id}`,
} as const;

export const RULE_ENDPOINTS = {
  LIST: `${API_VERSION}/rules`,
  CREATE: `${API_VERSION}/rules`,
  SHOW: (id: Id) => `${API_VERSION}/rules/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/rules/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/rules/${id}`,
  VERSIONS: (id: Id) => `${API_VERSION}/rules/${id}/versions`,
  PUBLISH: (id: Id) => `${API_VERSION}/rules/${id}/publish`,
  SIMULATE: `${API_VERSION}/rules/simulate`,
  DECISIONS: `${API_VERSION}/rules/decisions`,
  DECISION: (id: Id) => `${API_VERSION}/rules/decisions/${id}`,
} as const;

export const PROGRAM_ENDPOINTS = {
  LIST: `${API_VERSION}/programs`,
  CREATE: `${API_VERSION}/programs`,
  SHOW: (id: Id) => `${API_VERSION}/programs/${id}`,
  UPDATE: (id: Id) => `${API_VERSION}/programs/${id}`,
  DELETE: (id: Id) => `${API_VERSION}/programs/${id}`,
  ACTIVATE: (id: Id) => `${API_VERSION}/programs/${id}/activate`,
  PAUSE: (id: Id) => `${API_VERSION}/programs/${id}/pause`,
  END: (id: Id) => `${API_VERSION}/programs/${id}/end`,
  PLAYERS: (id: Id) => `${API_VERSION}/programs/${id}/players`,
  ADD_PLAYER: (id: Id) => `${API_VERSION}/programs/${id}/players`,
  REMOVE_PLAYER: (programId: Id, playerId: Id) => `${API_VERSION}/programs/${programId}/players/${playerId}`,
} as const;

export const HEALTH_ENDPOINTS = {
  CHECK: '/health',
} as const;

/** Builds "?a=1&b=2" from defined, non-empty values ("" when nothing is set). */
export function toQuery(params?: object): string {
  if (!params) return '';
  const q = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') q.append(key, String(value));
  });
  const s = q.toString();
  return s ? `?${s}` : '';
}

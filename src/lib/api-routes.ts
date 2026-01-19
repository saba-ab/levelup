/**
 * API Route Constants
 *
 * Centralized API endpoint paths for the application.
 * All API routes should use these constants for consistency and maintainability.
 */

// API Version Prefix
export const API_VERSION = '/api/v1';

// Auth Endpoints
export const AUTH_ENDPOINTS = {
  LOGIN: `${API_VERSION}/auth/login`,
  REGISTER: `${API_VERSION}/auth/register`,
  LOGOUT: `${API_VERSION}/auth/logout`,
  ME: `${API_VERSION}/auth/me`,
  REFRESH: `${API_VERSION}/auth/refresh`,
} as const;

// Player Endpoints
export const PLAYER_ENDPOINTS = {
  LIST: `${API_VERSION}/players`,
  CREATE: `${API_VERSION}/players`,
  SHOW: (id: number | string) => `${API_VERSION}/players/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/players/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/players/${id}`,
  WALLET: (id: number | string) => `${API_VERSION}/players/${id}/wallet`,
  WALLET_TRANSACTIONS: (id: number | string) => `${API_VERSION}/players/${id}/wallet/transactions`,
} as const;

// Badge Endpoints
export const BADGE_ENDPOINTS = {
  LIST: `${API_VERSION}/badges`,
  CREATE: `${API_VERSION}/badges`,
  SHOW: (id: number | string) => `${API_VERSION}/badges/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/badges/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/badges/${id}`,
  AWARD: `${API_VERSION}/badges/award`,
  REVOKE: `${API_VERSION}/badges/revoke`,
} as const;

// Mission Endpoints
export const MISSION_ENDPOINTS = {
  LIST: `${API_VERSION}/missions`,
  CREATE: `${API_VERSION}/missions`,
  SHOW: (id: number | string) => `${API_VERSION}/missions/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/missions/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/missions/${id}`,
  START: `${API_VERSION}/missions/start`,
  UPDATE_PROGRESS: `${API_VERSION}/missions/update-progress`,
  COMPLETE: `${API_VERSION}/missions/complete`,
} as const;

// Level Endpoints
export const LEVEL_ENDPOINTS = {
  LIST: `${API_VERSION}/levels`,
  CREATE: `${API_VERSION}/levels`,
  SHOW: (id: number | string) => `${API_VERSION}/levels/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/levels/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/levels/${id}`,
  GRANT_XP: `${API_VERSION}/levels/grant-xp`,
} as const;

// Reward Endpoints
export const REWARD_ENDPOINTS = {
  LIST: `${API_VERSION}/rewards`,
  CREATE: `${API_VERSION}/rewards`,
  SHOW: (id: number | string) => `${API_VERSION}/rewards/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/rewards/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/rewards/${id}`,
  CLAIM: `${API_VERSION}/rewards/claim`,
  REDEEM: `${API_VERSION}/rewards/redeem`,
} as const;

// Streak Endpoints
export const STREAK_ENDPOINTS = {
  LIST: `${API_VERSION}/streaks`,
  CREATE: `${API_VERSION}/streaks`,
  SHOW: (id: number | string) => `${API_VERSION}/streaks/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/streaks/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/streaks/${id}`,
  RECORD_ACTIVITY: `${API_VERSION}/streaks/record-activity`,
} as const;

// Leaderboard Endpoints
export const LEADERBOARD_ENDPOINTS = {
  LIST: `${API_VERSION}/leaderboards`,
  CREATE: `${API_VERSION}/leaderboards`,
  SHOW: (id: number | string) => `${API_VERSION}/leaderboards/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/leaderboards/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/leaderboards/${id}`,
  ENTRIES: (id: number | string) => `${API_VERSION}/leaderboards/${id}/entries`,
  PLAYER_RANK: (leaderboardId: number | string, playerId: number | string) =>
    `${API_VERSION}/leaderboards/${leaderboardId}/players/${playerId}/rank`,
} as const;

// Wallet Endpoints
export const WALLET_ENDPOINTS = {
  CREDIT: `${API_VERSION}/wallets/credit`,
  DEBIT: `${API_VERSION}/wallets/debit`,
  TRANSFER: `${API_VERSION}/wallets/transfer`,
} as const;

// Event Endpoints
export const EVENT_ENDPOINTS = {
  LIST: `${API_VERSION}/events`,
  CREATE: `${API_VERSION}/events`,
  SHOW: (id: number | string) => `${API_VERSION}/events/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/events/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/events/${id}`,
  PREDEFINED: `${API_VERSION}/events/predefined`,
  CUSTOM: `${API_VERSION}/events/custom`,
} as const;

// Rule Endpoints
export const RULE_ENDPOINTS = {
  LIST: `${API_VERSION}/rules`,
  CREATE: `${API_VERSION}/rules`,
  SHOW: (id: number | string) => `${API_VERSION}/rules/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/rules/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/rules/${id}`,
  EXECUTE: `${API_VERSION}/rules/execute`,
} as const;

// Program Endpoints
export const PROGRAM_ENDPOINTS = {
  LIST: `${API_VERSION}/programs`,
  CREATE: `${API_VERSION}/programs`,
  SHOW: (id: number | string) => `${API_VERSION}/programs/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/programs/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/programs/${id}`,
  ACTIVATE: (id: number | string) => `${API_VERSION}/programs/${id}/activate`,
  PAUSE: (id: number | string) => `${API_VERSION}/programs/${id}/pause`,
  END: (id: number | string) => `${API_VERSION}/programs/${id}/end`,
  DUPLICATE: (id: number | string) => `${API_VERSION}/programs/${id}/duplicate`,
  STATS: (id: number | string) => `${API_VERSION}/programs/${id}/stats`,
  PLAYERS: (id: number | string) => `${API_VERSION}/programs/${id}/players`,
  ADD_PLAYER: (id: number | string) => `${API_VERSION}/programs/${id}/players`,
  REMOVE_PLAYER: (programId: number | string, playerId: number | string) =>
    `${API_VERSION}/programs/${programId}/players/${playerId}`,
} as const;

// Segment Endpoints
export const SEGMENT_ENDPOINTS = {
  LIST: `${API_VERSION}/segments`,
  CREATE: `${API_VERSION}/segments`,
  SHOW: (id: number | string) => `${API_VERSION}/segments/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/segments/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/segments/${id}`,
  PLAYERS: (id: number | string) => `${API_VERSION}/segments/${id}/players`,
  ADD_PLAYER: (id: number | string) => `${API_VERSION}/segments/${id}/players`,
  REMOVE_PLAYER: (segmentId: number | string, playerId: number | string) =>
    `${API_VERSION}/segments/${segmentId}/players/${playerId}`,
  REFRESH: (id: number | string) => `${API_VERSION}/segments/${id}/refresh`,
  PREVIEW: `${API_VERSION}/segments/preview`,
} as const;

// User Endpoints
export const USER_ENDPOINTS = {
  LIST: `${API_VERSION}/users`,
  CREATE: `${API_VERSION}/users`,
  SHOW: (id: number | string) => `${API_VERSION}/users/${id}`,
  UPDATE: (id: number | string) => `${API_VERSION}/users/${id}`,
  DELETE: (id: number | string) => `${API_VERSION}/users/${id}`,
} as const;

// Health Check
export const HEALTH_ENDPOINTS = {
  CHECK: '/up',
} as const;

// Helper function to get all endpoint values (for debugging)
export function getAllEndpoints(): string[] {
  const endpoints: string[] = [];

  const addEndpoints = (obj: Record<string, unknown>) => {
    Object.values(obj).forEach(value => {
      if (typeof value === 'string') {
        endpoints.push(value);
      }
    });
  };

  addEndpoints(AUTH_ENDPOINTS);
  addEndpoints(HEALTH_ENDPOINTS);
  // Note: Function endpoints like PLAYER_ENDPOINTS.SHOW(id) would need to be called with a sample ID

  return endpoints;
}

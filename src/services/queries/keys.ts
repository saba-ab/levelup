// Query key factory for consistent cache key management
// IDs are numbers to match Laravel backend

import { PlayerFilters, MechanicsFilters, ProgramFilters } from '../api/types';

export const queryKeys = {
  // Players
  players: {
    all: ['players'] as const,
    lists: () => [...queryKeys.players.all, 'list'] as const,
    list: (filters?: PlayerFilters) => [...queryKeys.players.lists(), filters] as const,
    details: () => [...queryKeys.players.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.players.details(), id] as const,
    stats: (id: number) => [...queryKeys.players.detail(id), 'stats'] as const,
    badges: (id: number) => [...queryKeys.players.detail(id), 'badges'] as const,
    missions: (id: number, status?: string) => [...queryKeys.players.detail(id), 'missions', status] as const,
    streaks: (id: number) => [...queryKeys.players.detail(id), 'streaks'] as const,
    transactions: (id: number, walletId?: number) => [...queryKeys.players.detail(id), 'transactions', walletId] as const,
    level: (id: number) => [...queryKeys.players.detail(id), 'level'] as const,
  },

  // Badges
  badges: {
    all: ['badges'] as const,
    lists: () => [...queryKeys.badges.all, 'list'] as const,
    list: (filters?: MechanicsFilters) => [...queryKeys.badges.lists(), filters] as const,
    details: () => [...queryKeys.badges.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.badges.details(), id] as const,
    playerBadges: (playerId: number) => [...queryKeys.badges.all, 'player', playerId] as const,
  },

  // Levels
  levels: {
    all: ['levels'] as const,
    list: () => [...queryKeys.levels.all, 'list'] as const,
    details: () => [...queryKeys.levels.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.levels.details(), id] as const,
  },

  // Missions
  missions: {
    all: ['missions'] as const,
    lists: () => [...queryKeys.missions.all, 'list'] as const,
    list: (filters?: MechanicsFilters) => [...queryKeys.missions.lists(), filters] as const,
    details: () => [...queryKeys.missions.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.missions.details(), id] as const,
  },

  // Streaks
  streaks: {
    all: ['streaks'] as const,
    lists: () => [...queryKeys.streaks.all, 'list'] as const,
    list: (filters?: MechanicsFilters) => [...queryKeys.streaks.lists(), filters] as const,
    details: () => [...queryKeys.streaks.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.streaks.details(), id] as const,
  },

  // Leaderboards
  leaderboards: {
    all: ['leaderboards'] as const,
    list: () => [...queryKeys.leaderboards.all, 'list'] as const,
    details: () => [...queryKeys.leaderboards.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.leaderboards.details(), id] as const,
    entries: (id: number, limit?: number, offset?: number) => [...queryKeys.leaderboards.detail(id), 'entries', limit, offset] as const,
    playerRank: (id: number, playerId: number) => [...queryKeys.leaderboards.detail(id), 'rank', playerId] as const,
  },

  // Rewards
  rewards: {
    all: ['rewards'] as const,
    lists: () => [...queryKeys.rewards.all, 'list'] as const,
    list: (filters?: MechanicsFilters) => [...queryKeys.rewards.lists(), filters] as const,
    details: () => [...queryKeys.rewards.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.rewards.details(), id] as const,
    redemptions: (status?: string) => [...queryKeys.rewards.all, 'redemptions', status] as const,
  },

  // Point Wallets
  pointWallets: {
    all: ['pointWallets'] as const,
    list: () => [...queryKeys.pointWallets.all, 'list'] as const,
    details: () => [...queryKeys.pointWallets.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.pointWallets.details(), id] as const,
  },

  // Programs
  programs: {
    all: ['programs'] as const,
    lists: () => [...queryKeys.programs.all, 'list'] as const,
    list: (filters?: ProgramFilters) => [...queryKeys.programs.lists(), filters] as const,
    details: () => [...queryKeys.programs.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.programs.details(), id] as const,
    stats: (id: number) => [...queryKeys.programs.detail(id), 'stats'] as const,
    players: (id: number, page?: number) => [...queryKeys.programs.detail(id), 'players', page] as const,
  },

  // Segments
  segments: {
    all: ['segments'] as const,
    list: () => [...queryKeys.segments.all, 'list'] as const,
    details: () => [...queryKeys.segments.all, 'detail'] as const,
    detail: (id: number) => [...queryKeys.segments.details(), id] as const,
    players: (id: number, page?: number) => [...queryKeys.segments.detail(id), 'players', page] as const,
  },
};

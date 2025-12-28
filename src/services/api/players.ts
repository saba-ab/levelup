import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  Player,
  PlayerStats,
  PlayerBadge,
  PlayerMission,
  PlayerStreak,
  CreatePlayerData,
  UpdatePlayerData,
  PaginatedResponse,
  PlayerFilters,
  PointTransaction,
  AwardPointsData,
} from './types';

export function usePlayersService() {
  const api = useApi();

  // List players with filters and pagination
  const listPlayers = useCallback(async (filters?: PlayerFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<Player>>(`/api/players${query ? `?${query}` : ''}`);
  }, [api]);

  // Get single player
  const getPlayer = useCallback(async (playerId: string) => {
    return api.get<Player>(`/api/players/${playerId}`);
  }, [api]);

  // Get player by external ID
  const getPlayerByExternalId = useCallback(async (externalId: string) => {
    return api.get<Player>(`/api/players/external/${externalId}`);
  }, [api]);

  // Create player
  const createPlayer = useCallback(async (data: CreatePlayerData) => {
    return api.post<Player>('/api/players', data);
  }, [api]);

  // Update player
  const updatePlayer = useCallback(async (playerId: string, data: UpdatePlayerData) => {
    return api.patch<Player>(`/api/players/${playerId}`, data);
  }, [api]);

  // Delete player
  const deletePlayer = useCallback(async (playerId: string) => {
    return api.delete(`/api/players/${playerId}`);
  }, [api]);

  // Get player stats
  const getPlayerStats = useCallback(async (playerId: string) => {
    return api.get<PlayerStats>(`/api/players/${playerId}/stats`);
  }, [api]);

  // Get player badges
  const getPlayerBadges = useCallback(async (playerId: string) => {
    return api.get<PlayerBadge[]>(`/api/players/${playerId}/badges`);
  }, [api]);

  // Award badge to player
  const awardBadge = useCallback(async (playerId: string, badgeId: string, reason?: string) => {
    return api.post<PlayerBadge>(`/api/players/${playerId}/badges`, { badge_id: badgeId, reason });
  }, [api]);

  // Revoke badge from player
  const revokeBadge = useCallback(async (playerId: string, badgeId: string) => {
    return api.delete(`/api/players/${playerId}/badges/${badgeId}`);
  }, [api]);

  // Get player missions
  const getPlayerMissions = useCallback(async (playerId: string, status?: string) => {
    const query = status ? `?status=${status}` : '';
    return api.get<PlayerMission[]>(`/api/players/${playerId}/missions${query}`);
  }, [api]);

  // Get player streaks
  const getPlayerStreaks = useCallback(async (playerId: string) => {
    return api.get<PlayerStreak[]>(`/api/players/${playerId}/streaks`);
  }, [api]);

  // Get player point transactions
  const getPlayerTransactions = useCallback(async (playerId: string, walletId?: string) => {
    const query = walletId ? `?wallet_id=${walletId}` : '';
    return api.get<PaginatedResponse<PointTransaction>>(`/api/players/${playerId}/transactions${query}`);
  }, [api]);

  // Award points to player
  const awardPoints = useCallback(async (data: AwardPointsData) => {
    return api.post<PointTransaction>('/api/points/award', data);
  }, [api]);

  // Deduct points from player
  const deductPoints = useCallback(async (data: AwardPointsData) => {
    return api.post<PointTransaction>('/api/points/deduct', data);
  }, [api]);

  // Bulk import players
  const bulkImportPlayers = useCallback(async (players: CreatePlayerData[]) => {
    return api.post<{ imported: number; failed: number; errors: string[] }>('/api/players/bulk-import', { players });
  }, [api]);

  // Export players
  const exportPlayers = useCallback(async (filters?: PlayerFilters, format: 'csv' | 'json' = 'csv') => {
    const params = new URLSearchParams();
    params.append('format', format);
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    return api.get<Blob>(`/api/players/export?${params.toString()}`);
  }, [api]);

  return {
    listPlayers,
    getPlayer,
    getPlayerByExternalId,
    createPlayer,
    updatePlayer,
    deletePlayer,
    getPlayerStats,
    getPlayerBadges,
    awardBadge,
    revokeBadge,
    getPlayerMissions,
    getPlayerStreaks,
    getPlayerTransactions,
    awardPoints,
    deductPoints,
    bulkImportPlayers,
    exportPlayers,
  };
}
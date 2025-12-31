import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  Player,
  PlayerBadge,
  PlayerMission,
  PlayerStreak,
  PlayerReward,
  CreatePlayerData,
  UpdatePlayerData,
  PaginatedResponse,
  PlayerFilters,
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
    return api.get<PaginatedResponse<Player>>(`/players${query ? `?${query}` : ''}`);
  }, [api]);

  // Get single player by ID
  const getPlayer = useCallback(async (playerId: number) => {
    return api.get<Player>(`/players/${playerId}`);
  }, [api]);

  // Get player by external ID
  const getPlayerByExternalId = useCallback(async (externalId: string) => {
    return api.get<Player>(`/players/external/${encodeURIComponent(externalId)}`);
  }, [api]);

  // Create player
  const createPlayer = useCallback(async (data: CreatePlayerData) => {
    return api.post<Player>('/players', data);
  }, [api]);

  // Update player
  const updatePlayer = useCallback(async (playerId: number, data: UpdatePlayerData) => {
    return api.put<Player>(`/players/${playerId}`, data);
  }, [api]);

  // Delete player
  const deletePlayer = useCallback(async (playerId: number) => {
    return api.delete(`/players/${playerId}`);
  }, [api]);

  // Get player badges (via badges endpoint)
  const getPlayerBadges = useCallback(async (playerId: number) => {
    return api.get<PlayerBadge[]>(`/badges/players/${playerId}/badges`);
  }, [api]);

  // Get player missions (via missions endpoint per API spec)
  const getPlayerMissions = useCallback(async (playerId: number) => {
    return api.get<PlayerMission[]>(`/missions/players/${playerId}/missions`);
  }, [api]);

  // Get player streaks (via streaks endpoint per API spec)
  const getPlayerStreaks = useCallback(async (playerId: number) => {
    return api.get<PlayerStreak[]>(`/streaks/players/${playerId}/streaks`);
  }, [api]);

  // Get player rewards (via rewards endpoint per API spec)
  const getPlayerRewards = useCallback(async (playerId: number) => {
    return api.get<PlayerReward[]>(`/rewards/players/${playerId}/rewards`);
  }, [api]);

  return {
    listPlayers,
    getPlayer,
    getPlayerByExternalId,
    createPlayer,
    updatePlayer,
    deletePlayer,
    getPlayerBadges,
    getPlayerMissions,
    getPlayerStreaks,
    getPlayerRewards,
  };
}

import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  Program,
  CreateProgramData,
  UpdateProgramData,
  Segment,
  CreateSegmentData,
  PaginatedResponse,
  ProgramFilters,
  Player,
} from './types';

export function useProgramsService() {
  const api = useApi();

  // ==================== PROGRAMS ====================

  const listPrograms = useCallback(async (filters?: ProgramFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<Program>>(`/programs${query ? `?${query}` : ''}`);
  }, [api]);

  const getProgram = useCallback(async (programId: number) => {
    return api.get<Program>(`/programs/${programId}`);
  }, [api]);

  const createProgram = useCallback(async (data: CreateProgramData) => {
    return api.post<Program>('/programs', data);
  }, [api]);

  const updateProgram = useCallback(async (programId: number, data: UpdateProgramData) => {
    return api.put<Program>(`/programs/${programId}`, data);
  }, [api]);

  const deleteProgram = useCallback(async (programId: number) => {
    return api.delete(`/programs/${programId}`);
  }, [api]);

  const activateProgram = useCallback(async (programId: number) => {
    return api.post<Program>(`/programs/${programId}/activate`);
  }, [api]);

  const pauseProgram = useCallback(async (programId: number) => {
    return api.post<Program>(`/programs/${programId}/pause`);
  }, [api]);

  const endProgram = useCallback(async (programId: number) => {
    return api.post<Program>(`/programs/${programId}/end`);
  }, [api]);

  const duplicateProgram = useCallback(async (programId: number, newName: string) => {
    return api.post<Program>(`/programs/${programId}/duplicate`, { name: newName });
  }, [api]);

  const getProgramStats = useCallback(async (programId: number) => {
    return api.get<{
      total_players: number;
      active_players: number;
      total_points_awarded: number;
      total_badges_awarded: number;
      total_missions_completed: number;
      total_rewards_redeemed: number;
    }>(`/programs/${programId}/stats`);
  }, [api]);

  const getProgramPlayers = useCallback(async (programId: number, page = 1, perPage = 20) => {
    return api.get<PaginatedResponse<Player>>(`/programs/${programId}/players?page=${page}&per_page=${perPage}`);
  }, [api]);

  const addPlayerToProgram = useCallback(async (programId: number, playerId: number) => {
    return api.post(`/programs/${programId}/players`, { player_id: playerId });
  }, [api]);

  const addPlayersToProgram = useCallback(async (programId: number, playerIds: number[]) => {
    // Bulk add - send multiple requests in parallel
    const results = await Promise.allSettled(
      playerIds.map(playerId => api.post(`/programs/${programId}/players`, { player_id: playerId }))
    );
    const successful = results.filter(r => r.status === 'fulfilled').length;
    const failed = results.filter(r => r.status === 'rejected').length;
    return { success: true, data: { successful, failed, total: playerIds.length } };
  }, [api]);

  const removePlayerFromProgram = useCallback(async (programId: number, playerId: number) => {
    return api.delete(`/programs/${programId}/players/${playerId}`);
  }, [api]);

  // ==================== SEGMENTS ====================

  const listSegments = useCallback(async () => {
    return api.get<Segment[]>('/segments');
  }, [api]);

  const getSegment = useCallback(async (segmentId: number) => {
    return api.get<Segment>(`/segments/${segmentId}`);
  }, [api]);

  const createSegment = useCallback(async (data: CreateSegmentData) => {
    return api.post<Segment>('/segments', data);
  }, [api]);

  const updateSegment = useCallback(async (segmentId: number, data: Partial<CreateSegmentData>) => {
    return api.put<Segment>(`/segments/${segmentId}`, data);
  }, [api]);

  const deleteSegment = useCallback(async (segmentId: number) => {
    return api.delete(`/segments/${segmentId}`);
  }, [api]);

  const getSegmentPlayers = useCallback(async (segmentId: number, page = 1, perPage = 20) => {
    return api.get<PaginatedResponse<Player>>(`/segments/${segmentId}/players?page=${page}&per_page=${perPage}`);
  }, [api]);

  const addPlayerToSegment = useCallback(async (segmentId: number, playerId: number) => {
    return api.post(`/segments/${segmentId}/players`, { player_id: playerId });
  }, [api]);

  const removePlayerFromSegment = useCallback(async (segmentId: number, playerId: number) => {
    return api.delete(`/segments/${segmentId}/players/${playerId}`);
  }, [api]);

  const refreshDynamicSegment = useCallback(async (segmentId: number) => {
    return api.post<Segment>(`/segments/${segmentId}/refresh`);
  }, [api]);

  const previewSegmentRules = useCallback(async (rules: CreateSegmentData['rules']) => {
    return api.post<{ player_count: number; sample_players: Player[] }>('/segments/preview', { rules });
  }, [api]);

  return {
    // Programs
    listPrograms,
    getProgram,
    createProgram,
    updateProgram,
    deleteProgram,
    activateProgram,
    pauseProgram,
    endProgram,
    duplicateProgram,
    getProgramStats,
    getProgramPlayers,
    addPlayerToProgram,
    addPlayersToProgram,
    removePlayerFromProgram,
    // Segments
    listSegments,
    getSegment,
    createSegment,
    updateSegment,
    deleteSegment,
    getSegmentPlayers,
    addPlayerToSegment,
    removePlayerFromSegment,
    refreshDynamicSegment,
    previewSegmentRules,
  };
}

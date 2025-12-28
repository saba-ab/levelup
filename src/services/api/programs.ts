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
    return api.get<PaginatedResponse<Program>>(`/api/programs${query ? `?${query}` : ''}`);
  }, [api]);

  const getProgram = useCallback(async (programId: string) => {
    return api.get<Program>(`/api/programs/${programId}`);
  }, [api]);

  const createProgram = useCallback(async (data: CreateProgramData) => {
    return api.post<Program>('/api/programs', data);
  }, [api]);

  const updateProgram = useCallback(async (programId: string, data: UpdateProgramData) => {
    return api.patch<Program>(`/api/programs/${programId}`, data);
  }, [api]);

  const deleteProgram = useCallback(async (programId: string) => {
    return api.delete(`/api/programs/${programId}`);
  }, [api]);

  const activateProgram = useCallback(async (programId: string) => {
    return api.post<Program>(`/api/programs/${programId}/activate`);
  }, [api]);

  const pauseProgram = useCallback(async (programId: string) => {
    return api.post<Program>(`/api/programs/${programId}/pause`);
  }, [api]);

  const endProgram = useCallback(async (programId: string) => {
    return api.post<Program>(`/api/programs/${programId}/end`);
  }, [api]);

  const duplicateProgram = useCallback(async (programId: string, newName: string) => {
    return api.post<Program>(`/api/programs/${programId}/duplicate`, { name: newName });
  }, [api]);

  const getProgramStats = useCallback(async (programId: string) => {
    return api.get<{
      total_players: number;
      active_players: number;
      total_points_awarded: number;
      total_badges_awarded: number;
      total_missions_completed: number;
      total_rewards_redeemed: number;
    }>(`/api/programs/${programId}/stats`);
  }, [api]);

  const getProgramPlayers = useCallback(async (programId: string, page = 1, perPage = 20) => {
    return api.get<PaginatedResponse<Player>>(`/api/programs/${programId}/players?page=${page}&per_page=${perPage}`);
  }, [api]);

  const addPlayerToProgram = useCallback(async (programId: string, playerId: string) => {
    return api.post(`/api/programs/${programId}/players`, { player_id: playerId });
  }, [api]);

  const removePlayerFromProgram = useCallback(async (programId: string, playerId: string) => {
    return api.delete(`/api/programs/${programId}/players/${playerId}`);
  }, [api]);

  // ==================== SEGMENTS ====================

  const listSegments = useCallback(async () => {
    return api.get<Segment[]>('/api/segments');
  }, [api]);

  const getSegment = useCallback(async (segmentId: string) => {
    return api.get<Segment>(`/api/segments/${segmentId}`);
  }, [api]);

  const createSegment = useCallback(async (data: CreateSegmentData) => {
    return api.post<Segment>('/api/segments', data);
  }, [api]);

  const updateSegment = useCallback(async (segmentId: string, data: Partial<CreateSegmentData>) => {
    return api.patch<Segment>(`/api/segments/${segmentId}`, data);
  }, [api]);

  const deleteSegment = useCallback(async (segmentId: string) => {
    return api.delete(`/api/segments/${segmentId}`);
  }, [api]);

  const getSegmentPlayers = useCallback(async (segmentId: string, page = 1, perPage = 20) => {
    return api.get<PaginatedResponse<Player>>(`/api/segments/${segmentId}/players?page=${page}&per_page=${perPage}`);
  }, [api]);

  const addPlayerToSegment = useCallback(async (segmentId: string, playerId: string) => {
    return api.post(`/api/segments/${segmentId}/players`, { player_id: playerId });
  }, [api]);

  const removePlayerFromSegment = useCallback(async (segmentId: string, playerId: string) => {
    return api.delete(`/api/segments/${segmentId}/players/${playerId}`);
  }, [api]);

  const refreshDynamicSegment = useCallback(async (segmentId: string) => {
    return api.post<Segment>(`/api/segments/${segmentId}/refresh`);
  }, [api]);

  const previewSegmentRules = useCallback(async (rules: CreateSegmentData['rules']) => {
    return api.post<{ player_count: number; sample_players: Player[] }>('/api/segments/preview', { rules });
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
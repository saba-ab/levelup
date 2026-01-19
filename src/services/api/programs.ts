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
import { PROGRAM_ENDPOINTS, SEGMENT_ENDPOINTS } from '@/lib/api-routes';

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
    return api.get<PaginatedResponse<Program>>(`${PROGRAM_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getProgram = useCallback(async (programId: number) => {
    return api.get<Program>(PROGRAM_ENDPOINTS.SHOW(programId));
  }, [api]);

  const createProgram = useCallback(async (data: CreateProgramData) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateProgram = useCallback(async (programId: number, data: UpdateProgramData) => {
    return api.put<Program>(PROGRAM_ENDPOINTS.UPDATE(programId), data);
  }, [api]);

  const deleteProgram = useCallback(async (programId: number) => {
    return api.delete(PROGRAM_ENDPOINTS.DELETE(programId));
  }, [api]);

  const activateProgram = useCallback(async (programId: number) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.ACTIVATE(programId));
  }, [api]);

  const pauseProgram = useCallback(async (programId: number) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.PAUSE(programId));
  }, [api]);

  const endProgram = useCallback(async (programId: number) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.END(programId));
  }, [api]);

  const duplicateProgram = useCallback(async (programId: number, newName: string) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.DUPLICATE(programId), { name: newName });
  }, [api]);

  const getProgramStats = useCallback(async (programId: number) => {
    return api.get<{
      total_players: number;
      active_players: number;
      total_points_awarded: number;
      total_badges_awarded: number;
      total_missions_completed: number;
      total_rewards_redeemed: number;
    }>(PROGRAM_ENDPOINTS.STATS(programId));
  }, [api]);

  const getProgramPlayers = useCallback(async (programId: number, page = 1, perPage = 20) => {
    return api.get<PaginatedResponse<Player>>(`${PROGRAM_ENDPOINTS.PLAYERS(programId)}?page=${page}&per_page=${perPage}`);
  }, [api]);

  const addPlayerToProgram = useCallback(async (programId: number, playerId: number) => {
    return api.post(PROGRAM_ENDPOINTS.ADD_PLAYER(programId), { player_id: playerId });
  }, [api]);

  const addPlayersToProgram = useCallback(async (programId: number, playerIds: number[]) => {
    // Bulk add - send multiple requests in parallel
    const results = await Promise.allSettled(
      playerIds.map(playerId => api.post(PROGRAM_ENDPOINTS.ADD_PLAYER(programId), { player_id: playerId }))
    );
    const successful = results.filter(r => r.status === 'fulfilled').length;
    const failed = results.filter(r => r.status === 'rejected').length;
    return { success: true, data: { successful, failed, total: playerIds.length } };
  }, [api]);

  const removePlayerFromProgram = useCallback(async (programId: number, playerId: number) => {
    return api.delete(PROGRAM_ENDPOINTS.REMOVE_PLAYER(programId, playerId));
  }, [api]);

  // ==================== SEGMENTS ====================

  const listSegments = useCallback(async () => {
    return api.get<Segment[]>(SEGMENT_ENDPOINTS.LIST);
  }, [api]);

  const getSegment = useCallback(async (segmentId: number) => {
    return api.get<Segment>(SEGMENT_ENDPOINTS.SHOW(segmentId));
  }, [api]);

  const createSegment = useCallback(async (data: CreateSegmentData) => {
    return api.post<Segment>(SEGMENT_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateSegment = useCallback(async (segmentId: number, data: Partial<CreateSegmentData>) => {
    return api.put<Segment>(SEGMENT_ENDPOINTS.UPDATE(segmentId), data);
  }, [api]);

  const deleteSegment = useCallback(async (segmentId: number) => {
    return api.delete(SEGMENT_ENDPOINTS.DELETE(segmentId));
  }, [api]);

  const getSegmentPlayers = useCallback(async (segmentId: number, page = 1, perPage = 20) => {
    return api.get<PaginatedResponse<Player>>(`${SEGMENT_ENDPOINTS.PLAYERS(segmentId)}?page=${page}&per_page=${perPage}`);
  }, [api]);

  const addPlayerToSegment = useCallback(async (segmentId: number, playerId: number) => {
    return api.post(SEGMENT_ENDPOINTS.ADD_PLAYER(segmentId), { player_id: playerId });
  }, [api]);

  const removePlayerFromSegment = useCallback(async (segmentId: number, playerId: number) => {
    return api.delete(SEGMENT_ENDPOINTS.REMOVE_PLAYER(segmentId, playerId));
  }, [api]);

  const refreshDynamicSegment = useCallback(async (segmentId: number) => {
    return api.post<Segment>(SEGMENT_ENDPOINTS.REFRESH(segmentId));
  }, [api]);

  const previewSegmentRules = useCallback(async (rules: CreateSegmentData['rules']) => {
    return api.post<{ player_count: number; sample_players: Player[] }>(SEGMENT_ENDPOINTS.PREVIEW, { rules });
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

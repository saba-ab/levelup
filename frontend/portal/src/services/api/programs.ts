import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import type {
  Program,
  CreateProgramData,
  UpdateProgramData,
  ProgramFilters,
  ProgramMember,
  ProgramEnrollment,
  BulkEnrollResult,
  CursorPage,
  CursorParams,
  ID,
} from './types';
import { PROGRAM_ENDPOINTS, toQuery } from '@/lib/api-routes';

/** Concurrent enroll requests during a bulk enroll. */
const BULK_ENROLL_CONCURRENCY = 4;

/** Mutations report errors to the caller, which shows them; skip the client's generic toast. */
const QUIET = { showErrorToast: false } as const;

export function useProgramsService() {
  const api = useApi();

  // ==================== PROGRAMS ====================

  const listPrograms = useCallback(async (filters?: ProgramFilters) => {
    return api.get<CursorPage<Program>>(`${PROGRAM_ENDPOINTS.LIST}${toQuery(filters)}`);
  }, [api]);

  const getProgram = useCallback(async (programId: ID) => {
    return api.get<Program>(PROGRAM_ENDPOINTS.SHOW(programId));
  }, [api]);

  const createProgram = useCallback(async (data: CreateProgramData) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.CREATE, data, QUIET);
  }, [api]);

  const updateProgram = useCallback(async (programId: ID, data: UpdateProgramData) => {
    return api.patch<Program>(PROGRAM_ENDPOINTS.UPDATE(programId), data, QUIET);
  }, [api]);

  const deleteProgram = useCallback(async (programId: ID) => {
    return api.delete(PROGRAM_ENDPOINTS.DELETE(programId), QUIET);
  }, [api]);

  const activateProgram = useCallback(async (programId: ID) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.ACTIVATE(programId), undefined, QUIET);
  }, [api]);

  const pauseProgram = useCallback(async (programId: ID) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.PAUSE(programId), undefined, QUIET);
  }, [api]);

  const endProgram = useCallback(async (programId: ID) => {
    return api.post<Program>(PROGRAM_ENDPOINTS.END(programId), undefined, QUIET);
  }, [api]);

  // ==================== ENROLLMENTS ====================

  const getProgramPlayers = useCallback(async (programId: ID, params?: CursorParams) => {
    return api.get<CursorPage<ProgramMember>>(`${PROGRAM_ENDPOINTS.PLAYERS(programId)}${toQuery(params)}`);
  }, [api]);

  /** 201 when newly enrolled, 200 when the player was already enrolled. */
  const addPlayerToProgram = useCallback(async (programId: ID, playerId: ID) => {
    return api.post<ProgramEnrollment>(PROGRAM_ENDPOINTS.ADD_PLAYER(programId), { player_id: playerId }, QUIET);
  }, [api]);

  /**
   * The API enrolls one player per request. Runs a few requests at a time and
   * collects per-player failures (with their problem code) instead of stopping.
   */
  const addPlayersToProgram = useCallback(async (programId: ID, playerIds: ID[]): Promise<BulkEnrollResult> => {
    const result: BulkEnrollResult = { total: playerIds.length, enrolled: 0, alreadyEnrolled: 0, failures: [] };
    const queue = [...playerIds];

    const worker = async () => {
      for (let playerId = queue.shift(); playerId !== undefined; playerId = queue.shift()) {
        const res = await addPlayerToProgram(programId, playerId);
        if (res.success) {
          if (res.status === 200) result.alreadyEnrolled += 1;
          else result.enrolled += 1;
        } else {
          result.failures.push({ playerId, code: res.code, error: res.error || 'Failed to enroll player' });
        }
      }
    };

    await Promise.all(Array.from({ length: Math.min(BULK_ENROLL_CONCURRENCY, playerIds.length) }, worker));
    return result;
  }, [addPlayerToProgram]);

  const removePlayerFromProgram = useCallback(async (programId: ID, playerId: ID) => {
    return api.delete(PROGRAM_ENDPOINTS.REMOVE_PLAYER(programId, playerId), QUIET);
  }, [api]);

  return {
    listPrograms,
    getProgram,
    createProgram,
    updateProgram,
    deleteProgram,
    activateProgram,
    pauseProgram,
    endProgram,
    getProgramPlayers,
    addPlayerToProgram,
    addPlayersToProgram,
    removePlayerFromProgram,
  };
}

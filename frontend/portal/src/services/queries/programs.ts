import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import type { ApiResponse } from '@/hooks/useApi';
import { useProgramsService } from '../api/programs';
import { queryKeys } from './keys';
import type {
  Program,
  CreateProgramData,
  UpdateProgramData,
  ProgramFilters,
  CursorPage,
  CursorParams,
  ID,
} from '../api/types';

/** A failed program API call; carries the problem+json code so the UI can branch on it. */
export class ProgramApiError extends Error {
  constructor(
    message: string,
    public readonly code: string | null = null,
    public readonly validationErrors: Record<string, string[]> | null = null,
    public readonly status = 0,
  ) {
    super(message);
    this.name = 'ProgramApiError';
  }
}

function unwrap<T>(response: ApiResponse<T>, fallback: string): T {
  if (!response.success) {
    throw new ProgramApiError(response.error || fallback, response.code, response.validationErrors, response.status);
  }
  return response.data as T;
}

/** Human-readable message for a failed program mutation, using the problem code when known. */
export function describeProgramError(err: unknown, fallback = 'Something went wrong'): string {
  if (err instanceof ProgramApiError) {
    switch (err.code) {
      case 'invalid_status_transition':
        return 'The program cannot move to that status from its current status.';
      case 'program_window_elapsed':
        return 'The program end date has passed. Move the end date before activating it.';
      case 'program_not_accepting_players':
        return 'This program is not accepting players (ended programs cannot take enrollments).';
      case 'player_not_found':
        return 'Player not found.';
      case 'player_inactive':
        return 'The player is inactive.';
    }
    if (err.validationErrors) {
      return Object.entries(err.validationErrors).map(([field, msgs]) => `${field}: ${msgs.join(', ')}`).join('\n');
    }
    return err.message || fallback;
  }
  return err instanceof Error ? err.message : fallback;
}

// ==================== PROGRAMS ====================

export function useProgramsQuery(filters?: ProgramFilters) {
  const { listPrograms } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.programs.list(filters),
    queryFn: async () => unwrap(await listPrograms(filters), 'Failed to fetch programs'),
  });
}

export function useProgramQuery(programId: ID | undefined) {
  const { getProgram } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.programs.detail(programId ?? ''),
    queryFn: async () => unwrap(await getProgram(programId!), 'Failed to fetch program'),
    enabled: !!programId,
  });
}

/**
 * @deprecated The Go API has no program stats endpoint. This query never runs
 * and always yields undefined data; kept only so existing imports compile.
 */
export function useProgramStatsQuery(programId: ID | undefined) {
  return useQuery({
    queryKey: queryKeys.programs.stats(programId ?? ''),
    queryFn: async (): Promise<null> => null,
    enabled: false,
  });
}

export function useProgramPlayersQuery(programId: ID | undefined, params?: CursorParams) {
  const { getProgramPlayers } = useProgramsService();

  return useQuery({
    queryKey: [...queryKeys.programs.players(programId ?? '', params?.cursor), params?.limit] as const,
    queryFn: async () => unwrap(await getProgramPlayers(programId!, params), 'Failed to fetch program players'),
    enabled: !!programId,
  });
}

export function useCreateProgramMutation() {
  const queryClient = useQueryClient();
  const { createProgram } = useProgramsService();

  return useMutation({
    mutationFn: async (data: CreateProgramData) => unwrap(await createProgram(data), 'Failed to create program'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

export function useUpdateProgramMutation() {
  const queryClient = useQueryClient();
  const { updateProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, data }: { programId: ID; data: UpdateProgramData }) =>
      unwrap(await updateProgram(programId, data), 'Failed to update program'),
    onMutate: async ({ programId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.programs.detail(programId) });
      const previousProgram = queryClient.getQueryData<Program>(queryKeys.programs.detail(programId));

      if (previousProgram) {
        queryClient.setQueryData<Program>(queryKeys.programs.detail(programId), {
          ...previousProgram,
          ...(data.name !== undefined && { name: data.name }),
          ...(data.slug !== undefined && { slug: data.slug }),
          ...(data.description !== undefined && { description: data.description }),
          ...(data.starts_at !== undefined && { starts_at: data.starts_at }),
          ...(data.ends_at !== undefined && { ends_at: data.ends_at }),
          ...(data.settings !== undefined && { settings: data.settings }),
          ...(data.mechanics !== undefined && { mechanics: data.mechanics }),
        });
      }

      return { previousProgram };
    },
    onError: (_err, { programId }, context) => {
      if (context?.previousProgram) {
        queryClient.setQueryData(queryKeys.programs.detail(programId), context.previousProgram);
      }
    },
    onSettled: (_data, _error, { programId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

export function useDeleteProgramMutation() {
  const queryClient = useQueryClient();
  const { deleteProgram } = useProgramsService();

  return useMutation({
    mutationFn: async (programId: ID) => {
      unwrap(await deleteProgram(programId), 'Failed to delete program');
      return programId;
    },
    onMutate: async (programId) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.programs.lists() });

      const previousLists = queryClient.getQueriesData<CursorPage<Program>>({
        queryKey: queryKeys.programs.lists(),
      });

      queryClient.setQueriesData<CursorPage<Program>>(
        { queryKey: queryKeys.programs.lists() },
        (old) => (old ? { ...old, data: old.data.filter((p) => p.id !== programId) } : old),
      );

      return { previousLists };
    },
    onError: (_err, _programId, context) => {
      context?.previousLists.forEach(([queryKey, data]) => {
        queryClient.setQueryData(queryKey, data);
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.all });
    },
  });
}

function useTransitionMutation(
  pick: (svc: ReturnType<typeof useProgramsService>) => (id: ID) => Promise<ApiResponse<Program>>,
  fallback: string,
) {
  const queryClient = useQueryClient();
  const transition = pick(useProgramsService());

  return useMutation({
    mutationFn: async (programId: ID) => unwrap(await transition(programId), fallback),
    onSuccess: (program, programId) => {
      queryClient.setQueryData(queryKeys.programs.detail(programId), program);
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
    onError: (_err, programId) => {
      // A 409 means our cached status is stale; refetch the real one.
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

/** draft|paused → active. 409 invalid_status_transition or program_window_elapsed. */
export function useActivateProgramMutation() {
  return useTransitionMutation((s) => s.activateProgram, 'Failed to activate program');
}

/** active → paused. 409 invalid_status_transition. */
export function usePauseProgramMutation() {
  return useTransitionMutation((s) => s.pauseProgram, 'Failed to pause program');
}

/** active|paused → ended. 409 invalid_status_transition. */
export function useEndProgramMutation() {
  return useTransitionMutation((s) => s.endProgram, 'Failed to end program');
}

/**
 * The Go API has no duplicate endpoint: this reads the source program and
 * creates a new draft with the same description, dates, settings and mechanics.
 */
export function useDuplicateProgramMutation() {
  const queryClient = useQueryClient();
  const { getProgram, createProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, newName }: { programId: ID; newName: string }) => {
      const source = unwrap(await getProgram(programId), 'Failed to load program to duplicate');
      const data: CreateProgramData = {
        name: newName,
        ...(source.description !== null && { description: source.description }),
        ...(source.starts_at !== null && { starts_at: source.starts_at }),
        ...(source.ends_at !== null && { ends_at: source.ends_at }),
        settings: source.settings,
        mechanics: source.mechanics,
      };
      return unwrap(await createProgram(data), 'Failed to duplicate program');
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

// ==================== ENROLLMENTS ====================

/** Invalidates every cursor page of the program's member list. */
/** Members and the program itself (its member_count), plus lists showing member counts. */
function invalidateMembers(queryClient: ReturnType<typeof useQueryClient>, programId: ID) {
  queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
  queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
}

export function useAddPlayerToProgramMutation() {
  const queryClient = useQueryClient();
  const { addPlayerToProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, playerId }: { programId: ID; playerId: ID }) =>
      unwrap(await addPlayerToProgram(programId, playerId), 'Failed to add player to program'),
    onSuccess: (_, { programId }) => invalidateMembers(queryClient, programId),
  });
}

/** Never throws for per-player failures; inspect result.failures (each has the problem code). */
export function useBulkAddPlayersToProgramMutation() {
  const queryClient = useQueryClient();
  const { addPlayersToProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, playerIds }: { programId: ID; playerIds: ID[] }) =>
      addPlayersToProgram(programId, playerIds),
    onSettled: (_data, _error, { programId }) => invalidateMembers(queryClient, programId),
  });
}

export function useRemovePlayerFromProgramMutation() {
  const queryClient = useQueryClient();
  const { removePlayerFromProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, playerId }: { programId: ID; playerId: ID }) => {
      unwrap(await removePlayerFromProgram(programId, playerId), 'Failed to remove player from program');
      return playerId;
    },
    onSuccess: (_, { programId }) => invalidateMembers(queryClient, programId),
  });
}

// ==================== SEGMENTS (not in the Go API) ====================

const SEGMENTS_UNAVAILABLE = 'Segments are not available in this API version.';

function useUnavailableQuery(key: readonly unknown[]) {
  return useQuery({ queryKey: key, queryFn: async (): Promise<null> => null, enabled: false });
}

function useUnavailableMutation<TVars>() {
  return useMutation({
    mutationFn: async (_vars: TVars): Promise<never> => {
      throw new ProgramApiError(SEGMENTS_UNAVAILABLE, 'not_available');
    },
  });
}

/** @deprecated Segments do not exist in the Go API; never fetches. */
export function useSegmentsQuery() {
  return useUnavailableQuery(queryKeys.segments.list());
}

/** @deprecated Segments do not exist in the Go API; never fetches. */
export function useSegmentQuery(segmentId: ID) {
  return useUnavailableQuery(queryKeys.segments.detail(segmentId));
}

/** @deprecated Segments do not exist in the Go API; never fetches. */
export function useSegmentPlayersQuery(segmentId: ID) {
  return useUnavailableQuery(queryKeys.segments.players(segmentId));
}

/** @deprecated Segments do not exist in the Go API; always rejects. */
export function useCreateSegmentMutation() {
  return useUnavailableMutation<unknown>();
}

/** @deprecated Segments do not exist in the Go API; always rejects. */
export function useUpdateSegmentMutation() {
  return useUnavailableMutation<unknown>();
}

/** @deprecated Segments do not exist in the Go API; always rejects. */
export function useDeleteSegmentMutation() {
  return useUnavailableMutation<ID>();
}

/** @deprecated Segments do not exist in the Go API; always rejects. */
export function useRefreshDynamicSegmentMutation() {
  return useUnavailableMutation<ID>();
}

/** @deprecated Segments do not exist in the Go API; always rejects. */
export function usePreviewSegmentRulesMutation() {
  return useUnavailableMutation<unknown>();
}

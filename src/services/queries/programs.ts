import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useProgramsService } from '../api/programs';
import { queryKeys } from './keys';
import {
  Program,
  CreateProgramData,
  UpdateProgramData,
  Segment,
  CreateSegmentData,
  ProgramFilters,
  PaginatedResponse,
} from '../api/types';

// ==================== PROGRAMS ====================

export function useProgramsQuery(filters?: ProgramFilters) {
  const { listPrograms } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.programs.list(filters),
    queryFn: async () => {
      const response = await listPrograms(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch programs');
      return response.data!;
    },
  });
}

export function useProgramQuery(programId: number) {
  const { getProgram } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.programs.detail(programId),
    queryFn: async () => {
      const response = await getProgram(programId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch program');
      return response.data!;
    },
    enabled: !!programId,
  });
}

export function useProgramStatsQuery(programId: number) {
  const { getProgramStats } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.programs.stats(programId),
    queryFn: async () => {
      const response = await getProgramStats(programId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch program stats');
      return response.data!;
    },
    enabled: !!programId,
  });
}

export function useProgramPlayersQuery(programId: number, page = 1, perPage = 20) {
  const { getProgramPlayers } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.programs.players(programId, page),
    queryFn: async () => {
      const response = await getProgramPlayers(programId, page, perPage);
      if (!response.success) throw new Error(response.error || 'Failed to fetch program players');
      return response.data!;
    },
    enabled: !!programId,
  });
}

export function useCreateProgramMutation() {
  const queryClient = useQueryClient();
  const { createProgram } = useProgramsService();

  return useMutation({
    mutationFn: async (data: CreateProgramData) => {
      const response = await createProgram(data);
      if (!response.success) throw new Error(response.error || 'Failed to create program');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

export function useUpdateProgramMutation() {
  const queryClient = useQueryClient();
  const { updateProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, data }: { programId: number; data: UpdateProgramData }) => {
      const response = await updateProgram(programId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update program');
      return response.data!;
    },
    onMutate: async ({ programId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.programs.detail(programId) });
      const previousProgram = queryClient.getQueryData<Program>(queryKeys.programs.detail(programId));

      if (previousProgram) {
        queryClient.setQueryData<Program>(queryKeys.programs.detail(programId), {
          ...previousProgram,
          name: data.name ?? previousProgram.name,
          description: data.description ?? previousProgram.description,
          status: data.status ?? previousProgram.status,
          start_date: data.start_date ?? previousProgram.start_date,
          end_date: data.end_date ?? previousProgram.end_date,
          settings: { ...previousProgram.settings, ...data.settings },
          mechanics: { ...previousProgram.mechanics, ...data.mechanics },
        });
      }

      return { previousProgram };
    },
    onError: (err, { programId }, context) => {
      if (context?.previousProgram) {
        queryClient.setQueryData(queryKeys.programs.detail(programId), context.previousProgram);
      }
    },
    onSettled: (data, error, { programId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

export function useDeleteProgramMutation() {
  const queryClient = useQueryClient();
  const { deleteProgram } = useProgramsService();

  return useMutation({
    mutationFn: async (programId: number) => {
      const response = await deleteProgram(programId);
      if (!response.success) throw new Error(response.error || 'Failed to delete program');
      return programId;
    },
    onMutate: async (programId) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.programs.lists() });

      const previousLists = queryClient.getQueriesData<PaginatedResponse<Program>>({
        queryKey: queryKeys.programs.lists(),
      });

      queryClient.setQueriesData<PaginatedResponse<Program>>(
        { queryKey: queryKeys.programs.lists() },
        (old) => {
          if (!old) return old;
          return {
            ...old,
            data: old.data.filter((p) => p.id !== programId),
            meta: { ...old.meta, total: old.meta.total - 1 },
          };
        }
      );

      return { previousLists };
    },
    onError: (err, programId, context) => {
      context?.previousLists.forEach(([queryKey, data]) => {
        queryClient.setQueryData(queryKey, data);
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.all });
    },
  });
}

export function useActivateProgramMutation() {
  const queryClient = useQueryClient();
  const { activateProgram } = useProgramsService();

  return useMutation({
    mutationFn: async (programId: number) => {
      const response = await activateProgram(programId);
      if (!response.success) throw new Error(response.error || 'Failed to activate program');
      return response.data!;
    },
    onSuccess: (data, programId) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

export function usePauseProgramMutation() {
  const queryClient = useQueryClient();
  const { pauseProgram } = useProgramsService();

  return useMutation({
    mutationFn: async (programId: number) => {
      const response = await pauseProgram(programId);
      if (!response.success) throw new Error(response.error || 'Failed to pause program');
      return response.data!;
    },
    onSuccess: (data, programId) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

export function useEndProgramMutation() {
  const queryClient = useQueryClient();
  const { endProgram } = useProgramsService();

  return useMutation({
    mutationFn: async (programId: number) => {
      const response = await endProgram(programId);
      if (!response.success) throw new Error(response.error || 'Failed to end program');
      return response.data!;
    },
    onSuccess: (data, programId) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

export function useDuplicateProgramMutation() {
  const queryClient = useQueryClient();
  const { duplicateProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, newName }: { programId: number; newName: string }) => {
      const response = await duplicateProgram(programId, newName);
      if (!response.success) throw new Error(response.error || 'Failed to duplicate program');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.lists() });
    },
  });
}

export function useAddPlayerToProgramMutation() {
  const queryClient = useQueryClient();
  const { addPlayerToProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, playerId }: { programId: number; playerId: number }) => {
      const response = await addPlayerToProgram(programId, playerId);
      if (!response.success) throw new Error(response.error || 'Failed to add player to program');
      return response.data;
    },
    onSuccess: (_, { programId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.players(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.stats(programId) });
    },
  });
}

export function useRemovePlayerFromProgramMutation() {
  const queryClient = useQueryClient();
  const { removePlayerFromProgram } = useProgramsService();

  return useMutation({
    mutationFn: async ({ programId, playerId }: { programId: number; playerId: number }) => {
      const response = await removePlayerFromProgram(programId, playerId);
      if (!response.success) throw new Error(response.error || 'Failed to remove player from program');
      return playerId;
    },
    onSuccess: (_, { programId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.players(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.detail(programId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.programs.stats(programId) });
    },
  });
}

// ==================== SEGMENTS ====================

export function useSegmentsQuery() {
  const { listSegments } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.segments.list(),
    queryFn: async () => {
      const response = await listSegments();
      if (!response.success) throw new Error(response.error || 'Failed to fetch segments');
      return response.data!;
    },
  });
}

export function useSegmentQuery(segmentId: number) {
  const { getSegment } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.segments.detail(segmentId),
    queryFn: async () => {
      const response = await getSegment(segmentId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch segment');
      return response.data!;
    },
    enabled: !!segmentId,
  });
}

export function useSegmentPlayersQuery(segmentId: number, page = 1, perPage = 20) {
  const { getSegmentPlayers } = useProgramsService();

  return useQuery({
    queryKey: queryKeys.segments.players(segmentId, page),
    queryFn: async () => {
      const response = await getSegmentPlayers(segmentId, page, perPage);
      if (!response.success) throw new Error(response.error || 'Failed to fetch segment players');
      return response.data!;
    },
    enabled: !!segmentId,
  });
}

export function useCreateSegmentMutation() {
  const queryClient = useQueryClient();
  const { createSegment } = useProgramsService();

  return useMutation({
    mutationFn: async (data: CreateSegmentData) => {
      const response = await createSegment(data);
      if (!response.success) throw new Error(response.error || 'Failed to create segment');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.segments.list() });
    },
  });
}

export function useUpdateSegmentMutation() {
  const queryClient = useQueryClient();
  const { updateSegment } = useProgramsService();

  return useMutation({
    mutationFn: async ({ segmentId, data }: { segmentId: number; data: Partial<CreateSegmentData> }) => {
      const response = await updateSegment(segmentId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update segment');
      return response.data!;
    },
    onMutate: async ({ segmentId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.segments.detail(segmentId) });
      const previousSegment = queryClient.getQueryData<Segment>(queryKeys.segments.detail(segmentId));

      if (previousSegment) {
        queryClient.setQueryData<Segment>(queryKeys.segments.detail(segmentId), {
          ...previousSegment,
          ...data,
        });
      }

      return { previousSegment };
    },
    onError: (err, { segmentId }, context) => {
      if (context?.previousSegment) {
        queryClient.setQueryData(queryKeys.segments.detail(segmentId), context.previousSegment);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.segments.all });
    },
  });
}

export function useDeleteSegmentMutation() {
  const queryClient = useQueryClient();
  const { deleteSegment } = useProgramsService();

  return useMutation({
    mutationFn: async (segmentId: number) => {
      const response = await deleteSegment(segmentId);
      if (!response.success) throw new Error(response.error || 'Failed to delete segment');
      return segmentId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.segments.all });
    },
  });
}

export function useRefreshDynamicSegmentMutation() {
  const queryClient = useQueryClient();
  const { refreshDynamicSegment } = useProgramsService();

  return useMutation({
    mutationFn: async (segmentId: number) => {
      const response = await refreshDynamicSegment(segmentId);
      if (!response.success) throw new Error(response.error || 'Failed to refresh segment');
      return response.data!;
    },
    onSuccess: (data, segmentId) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.segments.detail(segmentId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.segments.players(segmentId) });
    },
  });
}

export function usePreviewSegmentRulesMutation() {
  const { previewSegmentRules } = useProgramsService();

  return useMutation({
    mutationFn: async (rules: CreateSegmentData['rules']) => {
      const response = await previewSegmentRules(rules);
      if (!response.success) throw new Error(response.error || 'Failed to preview segment rules');
      return response.data!;
    },
  });
}

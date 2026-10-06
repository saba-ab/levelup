import { useCallback } from 'react';
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useSegmentsService } from '@/services/api/segments';
import type { CursorPage, CursorParams, ID } from '@/services/api/models/common';
import type {
  Segment,
  CreateSegmentData,
  UpdateSegmentData,
  SegmentConditionGroup,
} from '@/services/api/models/segments';
import { ApiRequestError, unwrap } from './rules';

/** Root matches queryKeys.segments.all so AI-created segments refresh the list. */
const segmentKeys = {
  all: ['segments'] as const,
  lists: () => [...segmentKeys.all, 'list'] as const,
  list: (params?: CursorParams) => [...segmentKeys.lists(), params] as const,
  details: () => [...segmentKeys.all, 'detail'] as const,
  detail: (id: ID) => [...segmentKeys.details(), id] as const,
  players: (id: ID, params?: CursorParams) => [...segmentKeys.detail(id), 'players', params] as const,
  preview: (conditions: SegmentConditionGroup | null) =>
    [...segmentKeys.all, 'preview', conditions ? JSON.stringify(conditions) : null] as const,
};

const ERROR_MESSAGES: Record<string, string> = {
  segment_name_taken: 'A segment with this name already exists.',
  segment_version_conflict: 'The segment was changed by someone else at the same time. Try again.',
  segment_not_found: 'The segment no longer exists.',
};

/** A user-facing message for a failed segments call. */
export function describeSegmentError(err: unknown, fallback: string): string {
  if (err instanceof ApiRequestError) {
    if (err.code && ERROR_MESSAGES[err.code]) return ERROR_MESSAGES[err.code];
    if (err.status === 403) return 'You do not have permission to manage segments.';
    if (err.validationErrors) {
      return Object.entries(err.validationErrors).map(([f, m]) => `${f}: ${m.join(', ')}`).join('\n');
    }
    // invalid_segment_conditions details name the path, e.g. "all[1].any[0]: ...".
    return err.message;
  }
  return fallback;
}

// ---- refresh polling ----------------------------------------------------

const POLL_MS = 3000;
/** Stop waiting for a queued refresh after this long (worker down, etc.). */
const AWAIT_REFRESH_MS = 3 * 60_000;

/**
 * Segment id -> the last_refreshed_at seen when a refresh was queued, and
 * when (client ms). Comparing against the previous server value avoids
 * client/server clock skew.
 */
const awaitedRefreshes = new Map<ID, { previous: string | null; at: number }>();

function awaitRefresh(s: Segment) {
  awaitedRefreshes.set(s.id, { previous: s.last_refreshed_at, at: Date.now() });
}

/**
 * A refresh is running, or one this client queued (create, conditions
 * change, "Refresh now") has not completed yet. A queued refresh reads
 * refreshing=false until the worker leases it, so pending is inferred from
 * last_refreshed_at changing, bounded in time. A never-refreshed segment
 * counts as pending for a while after creation.
 */
export function isSegmentRefreshPending(s: Segment, now = Date.now()): boolean {
  if (s.refreshing) return true;
  const awaited = awaitedRefreshes.get(s.id);
  if (awaited) {
    if (s.last_refreshed_at !== awaited.previous || now - awaited.at > AWAIT_REFRESH_MS) {
      awaitedRefreshes.delete(s.id);
    } else {
      return true;
    }
  }
  return s.last_refreshed_at === null && now - Date.parse(s.created_at) < AWAIT_REFRESH_MS;
}

// ---- queries ------------------------------------------------------------

/** One page of segments; polls while any listed segment is refreshing. */
export function useSegmentsQuery(params?: CursorParams) {
  const { listSegments } = useSegmentsService();
  return useQuery({
    queryKey: segmentKeys.list(params),
    queryFn: async () => unwrap(await listSegments(params), 'Failed to load segments'),
    placeholderData: keepPreviousData,
    refetchInterval: (query) =>
      (query.state.data as CursorPage<Segment> | undefined)?.data.some((s) => isSegmentRefreshPending(s)) ? POLL_MS : false,
  });
}

/** One segment; polls while it is refreshing. */
export function useSegmentQuery(id: ID | undefined) {
  const { getSegment } = useSegmentsService();
  return useQuery({
    queryKey: segmentKeys.detail(id ?? ''),
    queryFn: async () => unwrap(await getSegment(id!), 'Failed to load segment'),
    enabled: !!id,
    refetchInterval: (query) => {
      const s = query.state.data as Segment | undefined;
      return s && isSegmentRefreshPending(s) ? POLL_MS : false;
    },
  });
}

/** Materialized members as of the last refresh, newest first. */
export function useSegmentPlayersQuery(id: ID | undefined, params?: CursorParams) {
  const { listSegmentPlayers } = useSegmentsService();
  return useQuery({
    queryKey: segmentKeys.players(id ?? '', params),
    queryFn: async () => unwrap(await listSegmentPlayers(id!, params), 'Failed to load segment members'),
    enabled: !!id,
    placeholderData: keepPreviousData,
  });
}

/**
 * Live estimate for unsaved conditions. Pass debounced, complete conditions
 * (or null to disable); validation errors surface as the query error.
 */
export function useSegmentPreviewQuery(conditions: SegmentConditionGroup | null) {
  const { previewSegment } = useSegmentsService();
  return useQuery({
    queryKey: segmentKeys.preview(conditions),
    queryFn: async () => unwrap(await previewSegment(conditions!), 'Failed to preview segment'),
    enabled: !!conditions,
    placeholderData: keepPreviousData,
    retry: false,
    staleTime: 30_000,
  });
}

// ---- mutations ----------------------------------------------------------

export function useCreateSegmentMutation() {
  const queryClient = useQueryClient();
  const { createSegment } = useSegmentsService();
  return useMutation({
    mutationFn: async (data: CreateSegmentData) => unwrap(await createSegment(data), 'Failed to create segment'),
    onSuccess: (segment) => {
      awaitRefresh(segment);
      queryClient.setQueryData(segmentKeys.detail(segment.id), segment);
      return queryClient.invalidateQueries({ queryKey: segmentKeys.lists() });
    },
  });
}

export function useUpdateSegmentMutation() {
  const queryClient = useQueryClient();
  const { updateSegment } = useSegmentsService();
  return useMutation({
    mutationFn: async ({ id, data }: { id: ID; data: UpdateSegmentData }) =>
      unwrap(await updateSegment(id, data), 'Failed to update segment'),
    onSuccess: (segment, { data }) => {
      if (data.conditions) awaitRefresh(segment);
      queryClient.setQueryData(segmentKeys.detail(segment.id), segment);
      return queryClient.invalidateQueries({ queryKey: segmentKeys.lists() });
    },
  });
}

export function useDeleteSegmentMutation() {
  const queryClient = useQueryClient();
  const { deleteSegment } = useSegmentsService();
  return useMutation({
    mutationFn: async (id: ID) => {
      unwrap(await deleteSegment(id), 'Failed to delete segment');
      return id;
    },
    onSuccess: (id) => {
      awaitedRefreshes.delete(id);
      queryClient.removeQueries({ queryKey: segmentKeys.detail(id) });
      return queryClient.invalidateQueries({ queryKey: segmentKeys.lists() });
    },
  });
}

/** Queues a membership recomputation (202); the queries poll until it lands. */
export function useRefreshSegmentMutation() {
  const queryClient = useQueryClient();
  const { refreshSegment } = useSegmentsService();
  return useMutation({
    mutationFn: async (id: ID) => unwrap(await refreshSegment(id), 'Failed to refresh segment'),
    onSuccess: (res, id) => {
      awaitRefresh(res.segment);
      queryClient.setQueryData(segmentKeys.detail(id), res.segment);
      return queryClient.invalidateQueries({ queryKey: segmentKeys.all });
    },
  });
}

/** Re-fetch members once a refresh completes (called by the detail view). */
export function useInvalidateSegmentPlayers() {
  const queryClient = useQueryClient();
  return useCallback(
    (id: ID) => queryClient.invalidateQueries({ queryKey: [...segmentKeys.detail(id), 'players'] }),
    [queryClient],
  );
}

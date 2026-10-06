import { useCallback, useMemo } from 'react';
import { useApi } from '@/hooks/useApi';
import { API_VERSION, toQuery } from '@/lib/api-routes';
import type { CursorPage, CursorParams, ID } from './models/common';
import type {
  Segment,
  CreateSegmentData,
  UpdateSegmentData,
  SegmentConditionGroup,
  SegmentPreview,
  SegmentRefreshResponse,
  SegmentMember,
} from './models/segments';

export const SEGMENT_ENDPOINTS = {
  LIST: `${API_VERSION}/segments`,
  PREVIEW: `${API_VERSION}/segments/preview`,
  SEGMENT: (id: ID) => `${API_VERSION}/segments/${id}`,
  REFRESH: (id: ID) => `${API_VERSION}/segments/${id}/refresh`,
  PLAYERS: (id: ID) => `${API_VERSION}/segments/${id}/players`,
} as const;

/** Writes and previews report errors to the caller (forms render them), not as toasts. */
const QUIET = { showErrorToast: false } as const;

export function useSegmentsService() {
  const api = useApi();

  const listSegments = useCallback(
    (params?: CursorParams) => api.get<CursorPage<Segment>>(`${SEGMENT_ENDPOINTS.LIST}${toQuery(params)}`),
    [api],
  );

  const getSegment = useCallback((id: ID) => api.get<Segment>(SEGMENT_ENDPOINTS.SEGMENT(id)), [api]);

  /** 201; the first membership refresh is queued. */
  const createSegment = useCallback(
    (data: CreateSegmentData) => api.post<Segment>(SEGMENT_ENDPOINTS.LIST, data, QUIET),
    [api],
  );

  const updateSegment = useCallback(
    (id: ID, data: UpdateSegmentData) => api.patch<Segment>(SEGMENT_ENDPOINTS.SEGMENT(id), data, QUIET),
    [api],
  );

  const deleteSegment = useCallback((id: ID) => api.delete<void>(SEGMENT_ENDPOINTS.SEGMENT(id), QUIET), [api]);

  /** Evaluates unsaved conditions without changing membership. */
  const previewSegment = useCallback(
    (conditions: SegmentConditionGroup) =>
      api.post<SegmentPreview>(SEGMENT_ENDPOINTS.PREVIEW, { conditions }, QUIET),
    [api],
  );

  /** 202; poll the segment for refreshing / last_refreshed_at. */
  const refreshSegment = useCallback(
    (id: ID) => api.post<SegmentRefreshResponse>(SEGMENT_ENDPOINTS.REFRESH(id), undefined, QUIET),
    [api],
  );

  const listSegmentPlayers = useCallback(
    (id: ID, params?: CursorParams) =>
      api.get<CursorPage<SegmentMember>>(`${SEGMENT_ENDPOINTS.PLAYERS(id)}${toQuery(params)}`),
    [api],
  );

  return useMemo(
    () => ({
      listSegments,
      getSegment,
      createSegment,
      updateSegment,
      deleteSegment,
      previewSegment,
      refreshSegment,
      listSegmentPlayers,
    }),
    [listSegments, getSegment, createSegment, updateSegment, deleteSegment, previewSegment, refreshSegment, listSegmentPlayers],
  );
}

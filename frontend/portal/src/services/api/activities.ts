import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import type {
  Activity,
  ActivityFilters,
  IngestActivityData,
  IngestActivityResponse,
  CursorPage,
  ID,
} from './types';
import { ACTIVITY_ENDPOINTS, toQuery } from '@/lib/api-routes';

/** Activity ingestion: rules react to activities asynchronously. */
export function useActivitiesService() {
  const api = useApi();

  const listActivities = useCallback(
    (filters?: ActivityFilters) => api.get<CursorPage<Activity>>(`${ACTIVITY_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );

  const getActivity = useCallback((activityId: ID) => api.get<Activity>(ACTIVITY_ENDPOINTS.SHOW(activityId)), [api]);

  /**
   * 202 {status: "pending"} for a new activity; 200 duplicate=true when the
   * event_id was already ingested (event_id is the idempotency key).
   */
  const ingestActivity = useCallback(
    (data: IngestActivityData) => api.post<IngestActivityResponse>(ACTIVITY_ENDPOINTS.INGEST, data, { showErrorToast: false }),
    [api],
  );

  return useMemo(
    () => ({ listActivities, getActivity, ingestActivity }),
    [listActivities, getActivity, ingestActivity],
  );
}

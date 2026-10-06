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
import type { PlayerLastSeen } from './models/players';
import { ACTIVITY_ENDPOINTS, API_VERSION, toQuery } from '@/lib/api-routes';

/** Activity endpoints added after the shared ACTIVITY_ENDPOINTS. */
export const ACTIVITY_V2_ENDPOINTS = {
  /** GET ?player_ids=a,b (max 100): each player's latest activity (activity.view_any). */
  LAST_SEEN: `${API_VERSION}/activities/last-seen`,
} as const;

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

  /** Players with no activity are absent; order follows playerIds. */
  const getLastSeen = useCallback(
    (playerIds: ID[]) =>
      api.get<{ data: PlayerLastSeen[] }>(
        `${ACTIVITY_V2_ENDPOINTS.LAST_SEEN}${toQuery({ player_ids: playerIds.join(',') })}`,
        { showErrorToast: false },
      ),
    [api],
  );

  return useMemo(
    () => ({ listActivities, getActivity, ingestActivity, getLastSeen }),
    [listActivities, getActivity, ingestActivity, getLastSeen],
  );
}

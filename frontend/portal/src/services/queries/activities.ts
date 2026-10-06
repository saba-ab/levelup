import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useActivitiesService } from '@/services/api/activities';
import type { ID, ActivityFilters, IngestActivityData } from '@/services/api/types';
import { ApiRequestError, unwrap } from './rules';
import type { PlayerLastSeen } from '@/services/api/models/players';

/** At most 100 ids per GET /activities/last-seen call. */
const LAST_SEEN_BATCH = 100;

export const activityKeys = {
  all: ['activities'] as const,
  lists: () => [...activityKeys.all, 'list'] as const,
  list: (filters?: ActivityFilters) => [...activityKeys.lists(), filters] as const,
  detail: (id: ID) => [...activityKeys.all, 'detail', id] as const,
  lastSeen: (playerIds: ID[]) => [...activityKeys.all, 'last-seen', playerIds] as const,
};

/** One cursor page of activities, newest first. */
export function useActivitiesQuery(filters?: ActivityFilters, options?: { refetchInterval?: number }) {
  const { listActivities } = useActivitiesService();
  return useQuery({
    queryKey: activityKeys.list(filters),
    queryFn: async () => unwrap(await listActivities(filters), 'Failed to fetch activities'),
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useActivityQuery(activityId: ID | undefined) {
  const { getActivity } = useActivitiesService();
  return useQuery({
    queryKey: activityKeys.detail(activityId ?? ''),
    queryFn: async () => unwrap(await getActivity(activityId!), 'Failed to fetch activity'),
    enabled: !!activityId,
  });
}

/** POST /activities: 202 pending, or duplicate=true when event_id was seen before. */
export function useIngestActivityMutation() {
  const queryClient = useQueryClient();
  const { ingestActivity } = useActivitiesService();
  return useMutation({
    mutationFn: async (data: IngestActivityData) => unwrap(await ingestActivity(data), 'Failed to send activity'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: activityKeys.all }),
  });
}

/**
 * Each player's latest activity in one request per page of players
 * (GET /activities/last-seen), keyed by player id. Players with no activity
 * are absent from the map.
 */
export function usePlayersLastSeenQuery(playerIds: ID[], options?: { enabled?: boolean }) {
  const { getLastSeen } = useActivitiesService();
  return useQuery({
    queryKey: activityKeys.lastSeen(playerIds),
    queryFn: async () => {
      const byPlayer: Record<ID, PlayerLastSeen> = {};
      for (let i = 0; i < playerIds.length; i += LAST_SEEN_BATCH) {
        unwrap(await getLastSeen(playerIds.slice(i, i + LAST_SEEN_BATCH)), 'Failed to fetch last seen').data.forEach(
          row => {
            byPlayer[row.player_id] = row;
          },
        );
      }
      return byPlayer;
    },
    enabled: (options?.enabled ?? true) && playerIds.length > 0,
    placeholderData: keepPreviousData,
    retry: (count, err) => !(err instanceof ApiRequestError && err.status === 403) && count < 2,
  });
}

/** One player's latest activity, or null when they have none. */
export function usePlayerLastSeenQuery(playerId: ID | undefined) {
  const query = usePlayersLastSeenQuery(playerId ? [playerId] : [], { enabled: !!playerId });
  const ready = !!playerId && !!query.data && !query.isPlaceholderData;
  return { ...query, data: ready ? (query.data![playerId!] ?? null) : undefined };
}

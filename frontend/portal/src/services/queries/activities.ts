import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useActivitiesService } from '@/services/api/activities';
import type { ID, ActivityFilters, IngestActivityData } from '@/services/api/types';
import { unwrap } from './rules';

export const activityKeys = {
  all: ['activities'] as const,
  lists: () => [...activityKeys.all, 'list'] as const,
  list: (filters?: ActivityFilters) => [...activityKeys.lists(), filters] as const,
  detail: (id: ID) => [...activityKeys.all, 'detail', id] as const,
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

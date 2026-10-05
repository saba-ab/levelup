import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useEventsService } from '@/services/api/events';
import { fetchAllPages } from '@/services/api/pagination';
import type {
  ID,
  EventType,
  EventCategory,
  EventTypeFilters,
  CreateEventTypeData,
  UpdateEventTypeData,
} from '@/services/api/types';
import { unwrap } from './rules';

export const eventKeys = {
  all: ['events'] as const,
  lists: () => [...eventKeys.all, 'list'] as const,
  list: (filters?: EventTypeFilters) => [...eventKeys.lists(), filters] as const,
  categories: () => [...eventKeys.all, 'categories'] as const,
  details: () => [...eventKeys.all, 'detail'] as const,
  detail: (id: ID) => [...eventKeys.details(), id] as const,
};

/**
 * The whole event-type catalogue matching the filters (global + tenant
 * types). It is small, so every page is loaded for selects and filtering.
 */
export function useEventsQuery(filters?: Omit<EventTypeFilters, 'cursor' | 'limit'>) {
  const { listEvents } = useEventsService();
  return useQuery({
    queryKey: eventKeys.list(filters),
    queryFn: async (): Promise<EventType[]> =>
      unwrap(
        await fetchAllPages(cursor => listEvents({ ...filters, limit: 100, cursor })),
        'Failed to fetch event types',
      ).data,
  });
}

export function useEventCategoriesQuery() {
  const { listCategories } = useEventsService();
  return useQuery({
    queryKey: eventKeys.categories(),
    queryFn: async (): Promise<EventCategory[]> => unwrap(await listCategories(), 'Failed to fetch categories').data,
    staleTime: 5 * 60 * 1000,
  });
}

export function useEventQuery(eventId: ID | undefined) {
  const { getEvent } = useEventsService();
  return useQuery({
    queryKey: eventKeys.detail(eventId ?? ''),
    queryFn: async () => unwrap(await getEvent(eventId!), 'Failed to fetch event type'),
    enabled: !!eventId,
  });
}

export function useCreateEventMutation() {
  const queryClient = useQueryClient();
  const { createEvent } = useEventsService();
  return useMutation({
    mutationFn: async (data: CreateEventTypeData) => unwrap(await createEvent(data), 'Failed to create event type'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: eventKeys.all }),
  });
}

/** PATCH; global types fail with code global_event_type_read_only. */
export function useUpdateEventMutation() {
  const queryClient = useQueryClient();
  const { updateEvent } = useEventsService();
  return useMutation({
    mutationFn: async ({ eventId, data }: { eventId: ID; data: UpdateEventTypeData }) =>
      unwrap(await updateEvent(eventId, data), 'Failed to update event type'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: eventKeys.all }),
  });
}

export function useDeleteEventMutation() {
  const queryClient = useQueryClient();
  const { deleteEvent } = useEventsService();
  return useMutation({
    mutationFn: async (eventId: ID) => {
      unwrap(await deleteEvent(eventId), 'Failed to delete event type');
      return eventId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: eventKeys.all }),
  });
}

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useEventsService } from '@/services/api/events';
import type { TriggerEventFilters, CreateTriggerEventData, TriggerEvent } from '@/services/api/types';

export const eventKeys = {
  all: ['events'] as const,
  lists: () => [...eventKeys.all, 'list'] as const,
  list: (filters?: TriggerEventFilters) => [...eventKeys.lists(), filters] as const,
  details: () => [...eventKeys.all, 'detail'] as const,
  detail: (id: number) => [...eventKeys.details(), id] as const,
};

export function useEventsQuery(filters?: TriggerEventFilters) {
  const eventsService = useEventsService();
  
  return useQuery({
    queryKey: eventKeys.list(filters),
    queryFn: async (): Promise<TriggerEvent[]> => {
      const response = await eventsService.listEvents(filters);
      return response.data?.data || [];
    },
  });
}

export function useEventQuery(eventId: number) {
  const eventsService = useEventsService();
  
  return useQuery({
    queryKey: eventKeys.detail(eventId),
    queryFn: async () => {
      const response = await eventsService.getEvent(eventId);
      return response.data;
    },
    enabled: !!eventId,
  });
}

export function useCreateEventMutation() {
  const queryClient = useQueryClient();
  const eventsService = useEventsService();
  
  return useMutation({
    mutationFn: async (data: CreateTriggerEventData) => {
      const response = await eventsService.createEvent(data);
      if (!response.success) throw new Error(response.error || 'Failed to create event');
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: eventKeys.lists() });
    },
  });
}

export function useUpdateEventMutation() {
  const queryClient = useQueryClient();
  const eventsService = useEventsService();
  
  return useMutation({
    mutationFn: async ({ eventId, data }: { eventId: number; data: Partial<CreateTriggerEventData> }) => {
      const response = await eventsService.updateEvent(eventId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update event');
      return response.data;
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: eventKeys.lists() });
      queryClient.invalidateQueries({ queryKey: eventKeys.detail(variables.eventId) });
    },
  });
}

export function useDeleteEventMutation() {
  const queryClient = useQueryClient();
  const eventsService = useEventsService();
  
  return useMutation({
    mutationFn: async (eventId: number) => {
      const response = await eventsService.deleteEvent(eventId);
      if (!response.success) throw new Error(response.error || 'Failed to delete event');
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: eventKeys.lists() });
    },
  });
}

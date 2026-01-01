import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  TriggerEvent,
  CreateTriggerEventData,
  TriggerEventFilters,
  PaginatedResponse,
} from './types';

export function useEventsService() {
  const api = useApi();

  const listEvents = useCallback(async (filters?: TriggerEventFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<TriggerEvent>>(`/events${query ? `?${query}` : ''}`);
  }, [api]);

  const getEvent = useCallback(async (eventId: number) => {
    return api.get<TriggerEvent>(`/events/${eventId}`);
  }, [api]);

  const createEvent = useCallback(async (data: CreateTriggerEventData) => {
    return api.post<TriggerEvent>('/events', data);
  }, [api]);

  const updateEvent = useCallback(async (eventId: number, data: Partial<CreateTriggerEventData>) => {
    return api.put<TriggerEvent>(`/events/${eventId}`, data);
  }, [api]);

  const deleteEvent = useCallback(async (eventId: number) => {
    return api.delete(`/events/${eventId}`);
  }, [api]);

  return {
    listEvents,
    getEvent,
    createEvent,
    updateEvent,
    deleteEvent,
  };
}

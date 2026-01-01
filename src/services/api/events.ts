import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  TriggerEvent,
  CreateTriggerEventData,
  UpdateTriggerEventData,
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

  const getPredefinedEvents = useCallback(async () => {
    return api.get<{ data: TriggerEvent[] }>('/events/predefined');
  }, [api]);

  const getCustomEvents = useCallback(async () => {
    return api.get<{ data: TriggerEvent[] }>('/events/custom');
  }, [api]);

  const getEvent = useCallback(async (eventId: number) => {
    return api.get<{ data: TriggerEvent }>(`/events/${eventId}`);
  }, [api]);

  const createEvent = useCallback(async (data: CreateTriggerEventData) => {
    return api.post<{ data: TriggerEvent }>('/events', data);
  }, [api]);

  const updateEvent = useCallback(async (eventId: number, data: UpdateTriggerEventData) => {
    return api.put<{ data: TriggerEvent }>(`/events/${eventId}`, data);
  }, [api]);

  const deleteEvent = useCallback(async (eventId: number) => {
    return api.delete(`/events/${eventId}`);
  }, [api]);

  return {
    listEvents,
    getPredefinedEvents,
    getCustomEvents,
    getEvent,
    createEvent,
    updateEvent,
    deleteEvent,
  };
}

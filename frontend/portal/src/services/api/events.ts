import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  TriggerEvent,
  CreateTriggerEventData,
  UpdateTriggerEventData,
  TriggerEventFilters,
  PaginatedResponse,
} from './types';
import { EVENT_ENDPOINTS } from '@/lib/api-routes';

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
    return api.get<PaginatedResponse<TriggerEvent>>(`${EVENT_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getPredefinedEvents = useCallback(async () => {
    return api.get<{ data: TriggerEvent[] }>(EVENT_ENDPOINTS.PREDEFINED);
  }, [api]);

  const getCustomEvents = useCallback(async () => {
    return api.get<{ data: TriggerEvent[] }>(EVENT_ENDPOINTS.CUSTOM);
  }, [api]);

  const getEvent = useCallback(async (eventId: number) => {
    return api.get<{ data: TriggerEvent }>(EVENT_ENDPOINTS.SHOW(eventId));
  }, [api]);

  const createEvent = useCallback(async (data: CreateTriggerEventData) => {
    return api.post<{ data: TriggerEvent }>(EVENT_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateEvent = useCallback(async (eventId: number, data: UpdateTriggerEventData) => {
    return api.put<{ data: TriggerEvent }>(EVENT_ENDPOINTS.UPDATE(eventId), data);
  }, [api]);

  const deleteEvent = useCallback(async (eventId: number) => {
    return api.delete(EVENT_ENDPOINTS.DELETE(eventId));
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

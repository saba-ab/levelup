import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import type {
  EventType,
  EventCategory,
  CreateEventTypeData,
  UpdateEventTypeData,
  EventTypeFilters,
  CursorPage,
  ID,
} from './types';
import { EVENT_ENDPOINTS, toQuery } from '@/lib/api-routes';

/** Writes report errors to the caller (it branches on res.code), not as toasts. */
const QUIET = { showErrorToast: false } as const;

/** Event types (the trigger catalogue at /events) and their categories. */
export function useEventsService() {
  const api = useApi();

  const listEvents = useCallback(
    (filters?: EventTypeFilters) => api.get<CursorPage<EventType>>(`${EVENT_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );

  const getEvent = useCallback((eventId: ID) => api.get<EventType>(EVENT_ENDPOINTS.SHOW(eventId)), [api]);

  const createEvent = useCallback(
    (data: CreateEventTypeData) => api.post<EventType>(EVENT_ENDPOINTS.CREATE, data, QUIET),
    [api],
  );

  /** Partial update. Global types answer 403 global_event_type_read_only. */
  const updateEvent = useCallback(
    (eventId: ID, data: UpdateEventTypeData) => api.patch<EventType>(EVENT_ENDPOINTS.UPDATE(eventId), data, QUIET),
    [api],
  );

  const deleteEvent = useCallback((eventId: ID) => api.delete<void>(EVENT_ENDPOINTS.DELETE(eventId), QUIET), [api]);

  /** Bounded catalogue, ordered by sort_order; next_cursor is always "". */
  const listCategories = useCallback(
    () => api.get<CursorPage<EventCategory>>(EVENT_ENDPOINTS.CATEGORIES),
    [api],
  );

  return useMemo(
    () => ({ listEvents, getEvent, createEvent, updateEvent, deleteEvent, listCategories }),
    [listEvents, getEvent, createEvent, updateEvent, deleteEvent, listCategories],
  );
}

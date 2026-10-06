import { useCallback, useMemo } from 'react';
import { useApi } from '@/hooks/useApi';
import { API_VERSION, toQuery } from '@/lib/api-routes';
import type { CursorPage, ID } from './models/common';
import type {
  PlatformTenant,
  PlatformTenantFilters,
  PlatformUpdateTenantData,
  PlatformEventType,
  PlatformEventTypeFilters,
  PlatformCreateEventTypeData,
  PlatformUpdateEventTypeData,
  PlatformEventCategory,
  PlatformCreateEventCategoryData,
  PlatformUpdateEventCategoryData,
} from './models/platform';

/**
 * Platform surface: only a principal holding platform_admin and no tenant may
 * call it (403 platform_only / platform_principal_required otherwise).
 */
export const PLATFORM_ENDPOINTS = {
  TENANTS: `${API_VERSION}/platform/tenants`,
  TENANT: (id: ID) => `${API_VERSION}/platform/tenants/${id}`,
  EVENT_TYPES: `${API_VERSION}/platform/event-types`,
  EVENT_TYPE: (id: ID) => `${API_VERSION}/platform/event-types/${id}`,
  EVENT_CATEGORIES: `${API_VERSION}/platform/event-categories`,
  EVENT_CATEGORY: (id: ID) => `${API_VERSION}/platform/event-categories/${id}`,
} as const;

/** Writes report errors to the caller (forms render them per field), not as toasts. */
const QUIET = { showErrorToast: false } as const;

export function usePlatformService() {
  const api = useApi();

  const listTenants = useCallback(
    (params?: PlatformTenantFilters) =>
      api.get<CursorPage<PlatformTenant>>(`${PLATFORM_ENDPOINTS.TENANTS}${toQuery(params)}`),
    [api],
  );

  const updateTenant = useCallback(
    (tenantId: ID, data: PlatformUpdateTenantData) =>
      api.patch<PlatformTenant>(PLATFORM_ENDPOINTS.TENANT(tenantId), data, QUIET),
    [api],
  );

  const listEventTypes = useCallback(
    (filters?: PlatformEventTypeFilters) =>
      api.get<CursorPage<PlatformEventType>>(`${PLATFORM_ENDPOINTS.EVENT_TYPES}${toQuery(filters)}`),
    [api],
  );

  const getEventType = useCallback(
    (eventTypeId: ID) => api.get<PlatformEventType>(PLATFORM_ENDPOINTS.EVENT_TYPE(eventTypeId)),
    [api],
  );

  const createEventType = useCallback(
    (data: PlatformCreateEventTypeData) => api.post<PlatformEventType>(PLATFORM_ENDPOINTS.EVENT_TYPES, data, QUIET),
    [api],
  );

  const updateEventType = useCallback(
    (eventTypeId: ID, data: PlatformUpdateEventTypeData) =>
      api.patch<PlatformEventType>(PLATFORM_ENDPOINTS.EVENT_TYPE(eventTypeId), data, QUIET),
    [api],
  );

  /** Soft delete. */
  const deleteEventType = useCallback(
    (eventTypeId: ID) => api.delete<void>(PLATFORM_ENDPOINTS.EVENT_TYPE(eventTypeId), QUIET),
    [api],
  );

  /** Bounded catalogue ordered by sort_order; next_cursor is always "". */
  const listEventCategories = useCallback(
    () => api.get<CursorPage<PlatformEventCategory>>(PLATFORM_ENDPOINTS.EVENT_CATEGORIES),
    [api],
  );

  const createEventCategory = useCallback(
    (data: PlatformCreateEventCategoryData) =>
      api.post<PlatformEventCategory>(PLATFORM_ENDPOINTS.EVENT_CATEGORIES, data, QUIET),
    [api],
  );

  const updateEventCategory = useCallback(
    (categoryId: ID, data: PlatformUpdateEventCategoryData) =>
      api.patch<PlatformEventCategory>(PLATFORM_ENDPOINTS.EVENT_CATEGORY(categoryId), data, QUIET),
    [api],
  );

  const deleteEventCategory = useCallback(
    (categoryId: ID) => api.delete<void>(PLATFORM_ENDPOINTS.EVENT_CATEGORY(categoryId), QUIET),
    [api],
  );

  return useMemo(
    () => ({
      listTenants,
      updateTenant,
      listEventTypes,
      getEventType,
      createEventType,
      updateEventType,
      deleteEventType,
      listEventCategories,
      createEventCategory,
      updateEventCategory,
      deleteEventCategory,
    }),
    [
      listTenants,
      updateTenant,
      listEventTypes,
      getEventType,
      createEventType,
      updateEventType,
      deleteEventType,
      listEventCategories,
      createEventCategory,
      updateEventCategory,
      deleteEventCategory,
    ],
  );
}

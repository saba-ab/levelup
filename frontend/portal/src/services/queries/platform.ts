import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { usePlatformService } from '@/services/api/platform';
import type { ID } from '@/services/api/models/common';
import type {
  PlatformEventCategory,
  PlatformTenantFilters,
  PlatformEventTypeFilters,
  PlatformCreateEventTypeData,
  PlatformUpdateEventTypeData,
  PlatformCreateEventCategoryData,
  PlatformUpdateEventCategoryData,
} from '@/services/api/models/platform';
import { eventKeys } from './events';
import { unwrap } from './rules';

export const platformKeys = {
  all: ['platform'] as const,
  tenants: (params?: PlatformTenantFilters) => [...platformKeys.all, 'tenants', params] as const,
  eventTypes: () => [...platformKeys.all, 'event-types'] as const,
  eventTypeList: (filters?: PlatformEventTypeFilters) => [...platformKeys.eventTypes(), 'list', filters] as const,
  eventType: (id: ID) => [...platformKeys.eventTypes(), 'detail', id] as const,
  eventCategories: () => [...platformKeys.all, 'event-categories'] as const,
};

// ---- tenants ------------------------------------------------------------------

/** One cursor page of tenants: { data, next_cursor }. */
export function usePlatformTenantsQuery(params?: PlatformTenantFilters, enabled = true) {
  const { listTenants } = usePlatformService();
  return useQuery({
    queryKey: platformKeys.tenants(params),
    queryFn: async () => unwrap(await listTenants(params), 'Failed to fetch tenants'),
    placeholderData: keepPreviousData,
    enabled,
  });
}

/** Activates or deactivates a tenant; deactivation revokes members' sessions. */
export function useSetPlatformTenantActiveMutation() {
  const queryClient = useQueryClient();
  const { updateTenant } = usePlatformService();
  return useMutation({
    mutationFn: async ({ tenantId, active }: { tenantId: ID; active: boolean }) =>
      unwrap(await updateTenant(tenantId, { active }), 'Failed to update tenant'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [...platformKeys.all, 'tenants'] }),
  });
}

// ---- global event types -------------------------------------------------------

/** One cursor page of global event types. */
export function usePlatformEventTypesQuery(filters?: PlatformEventTypeFilters, enabled = true) {
  const { listEventTypes } = usePlatformService();
  return useQuery({
    queryKey: platformKeys.eventTypeList(filters),
    queryFn: async () => unwrap(await listEventTypes(filters), 'Failed to fetch event types'),
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function usePlatformEventTypeQuery(eventTypeId: ID | undefined) {
  const { getEventType } = usePlatformService();
  return useQuery({
    queryKey: platformKeys.eventType(eventTypeId ?? ''),
    queryFn: async () => unwrap(await getEventType(eventTypeId!), 'Failed to fetch event type'),
    enabled: !!eventTypeId,
  });
}

/** Global types show up in every tenant catalogue, so tenant event lists are invalidated too. */
function useInvalidateEventTypes() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: platformKeys.eventTypes() }),
      queryClient.invalidateQueries({ queryKey: eventKeys.all }),
    ]);
}

export function useCreatePlatformEventTypeMutation() {
  const invalidate = useInvalidateEventTypes();
  const { createEventType } = usePlatformService();
  return useMutation({
    mutationFn: async (data: PlatformCreateEventTypeData) =>
      unwrap(await createEventType(data), 'Failed to create event type'),
    onSuccess: invalidate,
  });
}

export function useUpdatePlatformEventTypeMutation() {
  const invalidate = useInvalidateEventTypes();
  const { updateEventType } = usePlatformService();
  return useMutation({
    mutationFn: async ({ eventTypeId, data }: { eventTypeId: ID; data: PlatformUpdateEventTypeData }) =>
      unwrap(await updateEventType(eventTypeId, data), 'Failed to update event type'),
    onSuccess: invalidate,
  });
}

export function useDeletePlatformEventTypeMutation() {
  const invalidate = useInvalidateEventTypes();
  const { deleteEventType } = usePlatformService();
  return useMutation({
    mutationFn: async (eventTypeId: ID) => {
      unwrap(await deleteEventType(eventTypeId), 'Failed to delete event type');
      return eventTypeId;
    },
    onSuccess: invalidate,
  });
}

// ---- global event categories ----------------------------------------------------

/** The whole global category catalogue, ordered by sort_order. */
export function usePlatformEventCategoriesQuery(enabled = true) {
  const { listEventCategories } = usePlatformService();
  return useQuery({
    queryKey: platformKeys.eventCategories(),
    queryFn: async (): Promise<PlatformEventCategory[]> =>
      unwrap(await listEventCategories(), 'Failed to fetch event categories').data,
    enabled,
  });
}

function useInvalidateCategories() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: platformKeys.eventCategories() }),
      queryClient.invalidateQueries({ queryKey: eventKeys.categories() }),
    ]);
}

export function useCreatePlatformEventCategoryMutation() {
  const invalidate = useInvalidateCategories();
  const { createEventCategory } = usePlatformService();
  return useMutation({
    mutationFn: async (data: PlatformCreateEventCategoryData) =>
      unwrap(await createEventCategory(data), 'Failed to create category'),
    onSuccess: invalidate,
  });
}

export function useUpdatePlatformEventCategoryMutation() {
  const invalidate = useInvalidateCategories();
  const { updateEventCategory } = usePlatformService();
  return useMutation({
    mutationFn: async ({ categoryId, data }: { categoryId: ID; data: PlatformUpdateEventCategoryData }) =>
      unwrap(await updateEventCategory(categoryId, data), 'Failed to update category'),
    onSuccess: invalidate,
  });
}

export function useDeletePlatformEventCategoryMutation() {
  const invalidate = useInvalidateCategories();
  const { deleteEventCategory } = usePlatformService();
  return useMutation({
    mutationFn: async (categoryId: ID) => {
      unwrap(await deleteEventCategory(categoryId), 'Failed to delete category');
      return categoryId;
    },
    onSuccess: invalidate,
  });
}

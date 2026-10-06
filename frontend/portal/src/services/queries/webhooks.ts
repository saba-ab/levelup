import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useWebhooksService } from '@/services/api/webhooks';
import { fetchAllPages } from '@/services/api/pagination';
import type { CursorParams, ID } from '@/services/api/models/common';
import type {
  CreateWebhookEndpointData,
  UpdateWebhookEndpointData,
  WebhookDeliveryFilters,
} from '@/services/api/models/webhooks';
import { ApiRequestError, unwrap } from './rules';

const webhookKeys = {
  all: ['webhooks'] as const,
  endpoints: () => [...webhookKeys.all, 'endpoints'] as const,
  endpointList: (params?: CursorParams) => [...webhookKeys.endpoints(), 'list', params] as const,
  endpointAll: () => [...webhookKeys.endpoints(), 'all'] as const,
  endpoint: (id: ID) => [...webhookKeys.endpoints(), 'detail', id] as const,
  eventTypes: () => [...webhookKeys.all, 'event-types'] as const,
  deliveries: () => [...webhookKeys.all, 'deliveries'] as const,
  deliveryList: (filters?: WebhookDeliveryFilters) => [...webhookKeys.deliveries(), 'list', filters] as const,
  delivery: (id: ID) => [...webhookKeys.deliveries(), 'detail', id] as const,
};

const ERROR_MESSAGES: Record<string, string> = {
  webhook_url_insecure: 'The URL must use https.',
  webhook_url_forbidden_host: 'The URL must not point at a private, loopback, link-local or reserved address.',
  webhook_unknown_event_type: 'One of the selected event types is not in the catalogue.',
  webhook_endpoint_limit_reached: 'You already have the maximum number of webhook endpoints.',
  webhook_endpoint_inactive: 'The endpoint is disabled. Enable it first.',
  webhook_delivery_in_progress: 'A delivery attempt is in progress. Try again in a moment.',
  webhook_endpoint_version_conflict: 'The endpoint was changed by someone else. Reload and try again.',
  webhook_endpoint_not_found: 'The endpoint no longer exists.',
  webhook_delivery_not_found: 'The delivery no longer exists.',
};

/** A user-facing message for a failed webhooks call. */
export function describeWebhookError(err: unknown, fallback: string): string {
  if (err instanceof ApiRequestError) {
    if (err.code && ERROR_MESSAGES[err.code]) return ERROR_MESSAGES[err.code];
    if (err.status === 403) return 'You do not have permission to manage webhooks.';
    if (err.validationErrors) {
      return Object.entries(err.validationErrors).map(([f, m]) => `${f}: ${m.join(', ')}`).join('\n');
    }
    return err.message;
  }
  return fallback;
}

/** One cursor page of endpoints. */
export function useWebhookEndpointsQuery(params?: CursorParams, enabled = true) {
  const { listEndpoints } = useWebhooksService();
  return useQuery({
    queryKey: webhookKeys.endpointList(params),
    queryFn: async () => unwrap(await listEndpoints(params), 'Failed to load webhook endpoints'),
    placeholderData: keepPreviousData,
    enabled,
  });
}

/** Every endpoint (small, capped list) for filters and URL lookups. */
export function useAllWebhookEndpointsQuery(enabled = true) {
  const { listEndpoints } = useWebhooksService();
  return useQuery({
    queryKey: webhookKeys.endpointAll(),
    queryFn: async () =>
      unwrap(await fetchAllPages(cursor => listEndpoints({ limit: 100, cursor }), 500), 'Failed to load webhook endpoints').data,
    enabled,
    staleTime: 60_000,
  });
}

export function useWebhookEventTypesQuery(enabled = true) {
  const { listEventTypes } = useWebhooksService();
  return useQuery({
    queryKey: webhookKeys.eventTypes(),
    queryFn: async () => unwrap(await listEventTypes(), 'Failed to load webhook event types').data,
    enabled,
    staleTime: 5 * 60_000,
  });
}

export function useWebhookDeliveriesQuery(filters?: WebhookDeliveryFilters, enabled = true) {
  const { listDeliveries } = useWebhooksService();
  return useQuery({
    queryKey: webhookKeys.deliveryList(filters),
    queryFn: async () => unwrap(await listDeliveries(filters), 'Failed to load webhook deliveries'),
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function useWebhookDeliveryQuery(deliveryId: ID | undefined) {
  const { getDelivery } = useWebhooksService();
  return useQuery({
    queryKey: webhookKeys.delivery(deliveryId ?? ''),
    queryFn: async () => unwrap(await getDelivery(deliveryId!), 'Failed to load delivery'),
    enabled: !!deliveryId,
  });
}

export function useCreateWebhookEndpointMutation() {
  const queryClient = useQueryClient();
  const { createEndpoint } = useWebhooksService();
  return useMutation({
    mutationFn: async (data: CreateWebhookEndpointData) => unwrap(await createEndpoint(data), 'Failed to create endpoint'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: webhookKeys.endpoints() }),
  });
}

export function useUpdateWebhookEndpointMutation() {
  const queryClient = useQueryClient();
  const { updateEndpoint } = useWebhooksService();
  return useMutation({
    mutationFn: async ({ id, data }: { id: ID; data: UpdateWebhookEndpointData }) =>
      unwrap(await updateEndpoint(id, data), 'Failed to update endpoint'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: webhookKeys.endpoints() }),
  });
}

export function useDeleteWebhookEndpointMutation() {
  const queryClient = useQueryClient();
  const { deleteEndpoint } = useWebhooksService();
  return useMutation({
    mutationFn: async (id: ID) => {
      unwrap(await deleteEndpoint(id), 'Failed to delete endpoint');
      return id;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: webhookKeys.all }),
  });
}

export function useRotateWebhookSecretMutation() {
  const queryClient = useQueryClient();
  const { rotateSecret } = useWebhooksService();
  return useMutation({
    mutationFn: async (id: ID) => unwrap(await rotateSecret(id), 'Failed to rotate secret'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: webhookKeys.endpoints() }),
  });
}

/** Sends "webhook.test" synchronously; the delivery is recorded and the endpoint's streak may change. */
export function useTestWebhookEndpointMutation() {
  const queryClient = useQueryClient();
  const { testEndpoint } = useWebhooksService();
  return useMutation({
    mutationFn: async (id: ID) => unwrap(await testEndpoint(id), 'Failed to send test event'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: webhookKeys.all }),
  });
}

export function useRedeliverWebhookMutation() {
  const queryClient = useQueryClient();
  const { redeliver } = useWebhooksService();
  return useMutation({
    mutationFn: async (deliveryId: ID) => unwrap(await redeliver(deliveryId), 'Failed to redeliver'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: webhookKeys.deliveries() }),
  });
}

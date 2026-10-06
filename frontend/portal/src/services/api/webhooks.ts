import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import { API_VERSION, toQuery } from '@/lib/api-routes';
import type { CursorPage, CursorParams, ID } from './models/common';
import type {
  CreateWebhookEndpointData,
  UpdateWebhookEndpointData,
  WebhookDelivery,
  WebhookDeliveryFilters,
  WebhookEndpoint,
  WebhookEndpointWithSecret,
  WebhookEventType,
} from './models/webhooks';

/** Webhooks module (webhooks:view to read, webhooks:manage to write). */
export const WEBHOOK_ENDPOINTS = {
  LIST: `${API_VERSION}/webhooks`,
  CREATE: `${API_VERSION}/webhooks`,
  EVENT_TYPES: `${API_VERSION}/webhooks/event-types`,
  SHOW: (id: ID) => `${API_VERSION}/webhooks/${id}`,
  UPDATE: (id: ID) => `${API_VERSION}/webhooks/${id}`,
  DELETE: (id: ID) => `${API_VERSION}/webhooks/${id}`,
  ROTATE_SECRET: (id: ID) => `${API_VERSION}/webhooks/${id}/rotate-secret`,
  TEST: (id: ID) => `${API_VERSION}/webhooks/${id}/test`,
  DELIVERIES: `${API_VERSION}/webhooks/deliveries`,
  DELIVERY: (deliveryId: ID) => `${API_VERSION}/webhooks/deliveries/${deliveryId}`,
  REDELIVER: (deliveryId: ID) => `${API_VERSION}/webhooks/deliveries/${deliveryId}/redeliver`,
} as const;

/** Writes report errors to the caller (dialogs render them), not as toasts. */
const QUIET = { showErrorToast: false } as const;

export function useWebhooksService() {
  const api = useApi();

  const listEndpoints = useCallback(
    (params?: CursorParams) => api.get<CursorPage<WebhookEndpoint>>(`${WEBHOOK_ENDPOINTS.LIST}${toQuery(params)}`),
    [api],
  );
  const getEndpoint = useCallback((id: ID) => api.get<WebhookEndpoint>(WEBHOOK_ENDPOINTS.SHOW(id)), [api]);
  const createEndpoint = useCallback(
    (data: CreateWebhookEndpointData) => api.post<WebhookEndpointWithSecret>(WEBHOOK_ENDPOINTS.CREATE, data, QUIET),
    [api],
  );
  const updateEndpoint = useCallback(
    (id: ID, data: UpdateWebhookEndpointData) => api.patch<WebhookEndpoint>(WEBHOOK_ENDPOINTS.UPDATE(id), data, QUIET),
    [api],
  );
  const deleteEndpoint = useCallback((id: ID) => api.delete<void>(WEBHOOK_ENDPOINTS.DELETE(id), QUIET), [api]);
  const rotateSecret = useCallback(
    (id: ID) => api.post<WebhookEndpointWithSecret>(WEBHOOK_ENDPOINTS.ROTATE_SECRET(id), undefined, QUIET),
    [api],
  );
  /** Synchronous: returns the recorded "webhook.test" delivery (succeeded or failed). */
  const testEndpoint = useCallback(
    (id: ID) => api.post<WebhookDelivery>(WEBHOOK_ENDPOINTS.TEST(id), undefined, QUIET),
    [api],
  );
  const listEventTypes = useCallback(
    () => api.get<{ data: WebhookEventType[] }>(WEBHOOK_ENDPOINTS.EVENT_TYPES),
    [api],
  );
  const listDeliveries = useCallback(
    (filters?: WebhookDeliveryFilters) =>
      api.get<CursorPage<WebhookDelivery>>(`${WEBHOOK_ENDPOINTS.DELIVERIES}${toQuery(filters)}`),
    [api],
  );
  const getDelivery = useCallback((id: ID) => api.get<WebhookDelivery>(WEBHOOK_ENDPOINTS.DELIVERY(id)), [api]);
  /** Re-arms the delivery for a fresh retry cycle, signed with the current secret. */
  const redeliver = useCallback(
    (id: ID) => api.post<WebhookDelivery>(WEBHOOK_ENDPOINTS.REDELIVER(id), undefined, QUIET),
    [api],
  );

  return useMemo(
    () => ({
      listEndpoints,
      getEndpoint,
      createEndpoint,
      updateEndpoint,
      deleteEndpoint,
      rotateSecret,
      testEndpoint,
      listEventTypes,
      listDeliveries,
      getDelivery,
      redeliver,
    }),
    [listEndpoints, getEndpoint, createEndpoint, updateEndpoint, deleteEndpoint, rotateSecret, testEndpoint, listEventTypes, listDeliveries, getDelivery, redeliver],
  );
}

// Webhooks module (backend/internal/modules/webhooks): endpoints the tenant
// registers, the event catalogue they subscribe to, and recorded deliveries.
import type { CursorParams, ID } from './common';

/** "*" subscribes an endpoint to every catalogue event; it must be the only entry. */
export const WEBHOOK_WILDCARD = '*';

/** Synthetic event sent by POST /webhooks/{id}/test (not subscribable). */
export const WEBHOOK_TEST_EVENT = 'webhook.test';

/** A registered endpoint. The signing secret is never part of it. */
export interface WebhookEndpoint {
  id: ID;
  tenant_id: ID;
  url: string;
  description: string;
  event_types: string[];
  is_active: boolean;
  consecutive_failures: number;
  /** Why the endpoint was disabled ("" when active or disabled by hand). */
  disabled_reason: string;
  disabled_at: string | null;
  created_at: string;
  updated_at: string;
}

/** Returned by create and rotate-secret only: the one time the secret is shown. */
export interface WebhookEndpointWithSecret extends WebhookEndpoint {
  /** whsec_… */
  secret: string;
}

export interface CreateWebhookEndpointData {
  url: string;
  description?: string;
  /** Names from GET /webhooks/event-types, or ["*"]. */
  event_types: string[];
  is_active?: boolean;
}

/** Partial update. Setting is_active=true on a disabled endpoint clears its failure streak. */
export interface UpdateWebhookEndpointData {
  url?: string;
  description?: string;
  event_types?: string[];
  is_active?: boolean;
}

export interface WebhookEventType {
  /** e.g. "badges.awarded" */
  event: string;
  description: string;
}

export type WebhookDeliveryStatus = 'pending' | 'succeeded' | 'failed';

/**
 * One delivery. payload (the exact request body) is present on GET
 * /webhooks/deliveries/{id}, redeliver and test, and omitted from lists.
 */
export interface WebhookDelivery {
  id: ID;
  endpoint_id: ID;
  event: string;
  event_id: string;
  status: WebhookDeliveryStatus;
  attempts: number;
  response_status: number | null;
  response_body: string;
  latency_ms: number | null;
  last_error: string;
  last_attempt_at: string | null;
  delivered_at: string | null;
  created_at: string;
  updated_at: string;
  payload?: unknown;
}

export interface WebhookDeliveryFilters extends CursorParams {
  endpoint_id?: ID;
  status?: WebhookDeliveryStatus;
  /** e.g. "badges.awarded" */
  event?: string;
}

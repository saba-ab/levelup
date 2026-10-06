// Platform-admin models (principal with the platform_admin role and no
// tenant). Field-for-field with the Go DTOs in
// backend/internal/modules/identity/internal/transport/dto.go (tenants) and
// backend/internal/modules/eventcatalog/internal/transport/dto.go (global
// event types and categories). Responses reuse Tenant / EventType /
// EventCategory: the platform endpoints return the same shapes.
import type { CursorParams } from './common';
import type { Tenant } from './identity';
import type { EventCategory, EventPropertySchema, EventType, EventTypeFilters } from './events';

// ==================== TENANTS ====================

/** A row of GET /platform/tenants (same shape as TenantResp). */
export type PlatformTenant = Tenant;

export type PlatformTenantFilters = CursorParams;

/** PATCH /platform/tenants/{id}. Deactivating revokes every member's sessions. */
export interface PlatformUpdateTenantData {
  active: boolean;
}

// ==================== GLOBAL EVENT TYPES ====================

/** A platform-global event type: tenant_id is null and is_global is true. */
export type PlatformEventType = EventType;

/** GET /platform/event-types; include_global does not apply here. */
export type PlatformEventTypeFilters = Omit<EventTypeFilters, 'include_global'>;

export interface PlatformCreateEventTypeData {
  name: string;
  /** Defaults to the snake_case form of name. Immutable afterwards. */
  slug?: string;
  description?: string;
  /** Must be a global category. */
  category_id?: string;
  property_schema?: EventPropertySchema;
  /** Defaults to true. */
  is_active?: boolean;
}

/** PATCH body: omitted fields stay untouched; category_id / property_schema null clears; description "" clears. */
export interface PlatformUpdateEventTypeData {
  name?: string;
  description?: string;
  category_id?: string | null;
  property_schema?: EventPropertySchema | null;
  is_active?: boolean;
}

// ==================== GLOBAL EVENT CATEGORIES ====================

/** A platform-global event category (tenant_id null, is_global true). */
export type PlatformEventCategory = EventCategory;

export interface PlatformCreateEventCategoryData {
  name: string;
  /** Defaults to the slug form of name. */
  slug?: string;
  description?: string;
  sort_order?: number;
}

/** PATCH body: omitted fields stay untouched; description "" clears. */
export interface PlatformUpdateEventCategoryData {
  name?: string;
  slug?: string;
  description?: string;
  sort_order?: number;
}

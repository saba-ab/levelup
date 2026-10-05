// Segments: the Go API has no segments module (see backend/api/docs/swagger.json).
// These types remain only so legacy callers compile; nothing calls a
// segments endpoint and the Segments page shows a "not available" state.
import type { ID } from './common';

/** @deprecated not available in the Go API. */
export type SegmentType = 'static' | 'dynamic';

/** @deprecated not available in the Go API. */
export interface SegmentRule {
  field: string;
  operator: 'eq' | 'neq' | 'gt' | 'gte' | 'lt' | 'lte' | 'contains' | 'in';
  value: unknown;
}

/** @deprecated not available in the Go API. */
export interface Segment {
  id: ID;
  tenant_id: ID;
  name: string;
  description?: string;
  type: SegmentType;
  rules?: SegmentRule[];
  player_count: number;
  created_at: string;
  updated_at: string;
}

/** @deprecated not available in the Go API. */
export interface CreateSegmentData {
  name: string;
  description?: string;
  type: SegmentType;
  rules?: SegmentRule[];
}

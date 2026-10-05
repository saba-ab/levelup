// segments models (Go API contract, see docs/rewrite and backend/api/docs/swagger.json)
import type { Timestamps } from './common';

// ==================== SEGMENTS ====================

export type SegmentType = 'static' | 'dynamic';

export interface SegmentRule {
  field: string;
  operator: 'eq' | 'neq' | 'gt' | 'gte' | 'lt' | 'lte' | 'contains' | 'in';
  value: unknown;
}

export interface Segment extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  type: SegmentType;
  rules?: SegmentRule[];
  player_count: number;
}

export interface CreateSegmentData {
  name: string;
  description?: string;
  type: SegmentType;
  rules?: SegmentRule[];
}

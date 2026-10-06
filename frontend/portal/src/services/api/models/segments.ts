// Segments (backend/internal/modules/segments): saved player filters whose
// membership is materialized asynchronously by a refresh run.
import type { CursorParams, ID } from './common';

/** How a group combines its items: all = AND, any = OR. */
export type SegmentMatch = 'all' | 'any';

export type SegmentOperator =
  | 'eq'
  | 'neq'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'
  | 'in'
  | 'not_in'
  | 'contains'
  | 'exists'
  | 'not_exists'
  | 'before'
  | 'after'
  | 'has'
  | 'not_has';

/**
 * One test on a player field. Fields: `attributes.<path>`, `is_active`,
 * `created_at`, `level`, `balance`, `lifetime_earned`, `last_seen_days`,
 * `badges_earned`. `value` is omitted for exists/not_exists, an array for
 * in/not_in, a boolean for is_active, a date for created_at, a badge id for
 * has/not_has, a number otherwise.
 */
export interface SegmentCondition {
  field: string;
  op: SegmentOperator;
  value?: unknown;
}

/**
 * `{"all": [...]}` or `{"any": [...]}`; items are conditions or nested groups
 * (max depth 3, max 50 conditions overall).
 */
export type SegmentConditionGroup =
  | { all: SegmentConditionItem[]; any?: never }
  | { any: SegmentConditionItem[]; all?: never };

export type SegmentConditionItem = SegmentCondition | SegmentConditionGroup;

export interface Segment {
  id: ID;
  name: string;
  description: string;
  conditions: SegmentConditionGroup;
  /** Members as of last_refreshed_at. */
  member_count: number;
  /** null until the first refresh completes. */
  last_refreshed_at: string | null;
  /** A refresh run currently holds the segment. */
  refreshing: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreateSegmentData {
  name: string;
  description?: string;
  conditions: SegmentConditionGroup;
}

/** Partial update; changing conditions queues a refresh. */
export interface UpdateSegmentData {
  name?: string;
  description?: string;
  conditions?: SegmentConditionGroup;
}

export interface SegmentPlayerSummary {
  id: ID;
  external_id: string;
  display_name: string;
}

/** POST /segments/preview: evaluated over the first 1000 players (ascending id). */
export interface SegmentPreview {
  /** Matches among `scanned`; exact when `complete`. */
  count_estimate: number;
  scanned: number;
  complete: boolean;
  sample: SegmentPlayerSummary[];
}

export interface SegmentRefreshResponse {
  status: 'queued';
  segment: Segment;
}

export interface SegmentMember {
  player_id: ID;
  external_id: string;
  display_name: string;
  added_at: string;
}

export type SegmentListParams = CursorParams;

// Common API types for the Go backend (ADR-0016): UUID ids, cursor
// pagination, RFC 9457 problem+json errors with a machine-readable code.

/** Every entity id is a UUID string. */
export type ID = string;

/** One page of a list endpoint. next_cursor is "" on the last page. */
export interface CursorPage<T> {
  data: T[];
  next_cursor: string;
}

/** Query params every list endpoint accepts (limit: default 25, max 100). */
export interface CursorParams {
  limit?: number;
  cursor?: string;
}

/** Error body of every non-2xx response. */
export interface ProblemDetails {
  type: string;
  title: string;
  status: number;
  detail?: string;
  /** Stable code to branch on, e.g. "insufficient_balance". */
  code?: string;
  /** Field-level validation messages, keyed by JSON field name. */
  errors?: Record<string, string>;
  trace_id?: string;
}

export interface Timestamps {
  created_at: string;
  updated_at: string;
}

// ==================== FILTER TYPES ====================

export interface MechanicsFilters extends CursorParams {
  search?: string;
  is_active?: boolean;
  category?: string;
}

// ==================== STATS ====================

export interface PlayerStats {
  total_points: number;
  badges_earned: number;
  missions_completed: number;
  current_streak: number;
  rank: number;
}

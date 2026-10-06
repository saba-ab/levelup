// AI drafting module (backend/internal/modules/ai): drafts that are exactly
// the create body of the owning module's endpoint.
import type { ID } from './common';

export type AIDraftKind = 'badge' | 'level' | 'mission' | 'reward' | 'rule' | 'segment';

export const AI_DRAFT_KINDS: AIDraftKind[] = ['badge', 'level', 'mission', 'reward', 'rule', 'segment'];

export const AI_MAX_PROMPT_LENGTH = 2000;
export const AI_MIN_COUNT = 1;
export const AI_MAX_COUNT = 5;
export const AI_DEFAULT_COUNT = 3;

/** Problem codes the AI endpoints return. */
export const AI_ERROR_CODES = {
  NOT_CONFIGURED: 'ai_not_configured',
  QUOTA_EXCEEDED: 'ai_quota_exceeded',
  UNAVAILABLE: 'ai_unavailable',
  REFUSED: 'ai_refused',
  OUTPUT_TRUNCATED: 'ai_output_truncated',
  BAD_OUTPUT: 'ai_bad_output',
  UPSTREAM_ERROR: 'ai_upstream_error',
  INVALID_CONTEXT: 'invalid_context',
} as const;

export interface AIContextEventType {
  slug: string;
  name?: string;
  description?: string;
}

export interface AIContextRef {
  id: ID;
  name?: string;
}

export interface AIContextLevel {
  level_number: number;
  name?: string;
  xp_required: number;
}

/**
 * What the portal knows about the tenant: prompt data and the allow-list of
 * ids a draft may reference. Caps: 200 items per list, 100 levels.
 */
export interface AIDraftContext {
  event_types?: AIContextEventType[];
  badges?: AIContextRef[];
  missions?: AIContextRef[];
  rewards?: AIContextRef[];
  levels?: AIContextLevel[];
}

export interface AIDraftRequest {
  kind: AIDraftKind;
  /** At most 2000 characters. */
  prompt: string;
  /** 1..5, default 3. */
  count?: number;
  context?: AIDraftContext;
}

/** A draft: exactly the JSON body of the owning module's create endpoint. */
export type AIDraft = Record<string, unknown>;

export interface AIDraftRejection {
  index: number;
  reasons: string[];
}

export interface AIQuota {
  /** 0 = unlimited. */
  daily_limit: number;
  used: number;
  /** -1 = unlimited. */
  remaining: number;
}

export interface AIDraftResponse {
  kind: AIDraftKind;
  model: string;
  drafts: AIDraft[];
  rejected: AIDraftRejection[];
  usage: { input_tokens: number; output_tokens: number };
  quota: AIQuota;
}

export interface AIPromptTemplate {
  id: string;
  name: string;
  kind: AIDraftKind;
  description: string;
  example_prompt: string;
  default_count: number;
}

export interface AIUsageDay {
  /** YYYY-MM-DD (UTC). */
  day: string;
  requests: number;
  input_tokens: number;
  output_tokens: number;
}

export interface AIUsageReport {
  /** False when the server has no AI provider key. */
  enabled: boolean;
  model: string;
  /** 0 = unlimited. */
  daily_limit: number;
  /** -1 = unlimited. */
  remaining: number;
  today: AIUsageDay;
  /** Newest first; days without usage are omitted. */
  data: AIUsageDay[];
}

/** Minimal shape of a created segment (POST /segments), enough to link to it. */
export interface AICreatedSegment {
  id: ID;
  name: string;
}

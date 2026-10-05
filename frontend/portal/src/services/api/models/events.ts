// Event catalogue, activities and rules models. Field-for-field with the Go
// DTOs in backend/internal/modules/{eventcatalog,activity,rules}/internal/transport.
import type { CursorParams, ID } from './common';

// ==================== EVENT TYPES (catalogue) ====================

/** The JSON Schema subset an event type may declare for its properties. */
export interface EventPropertySchema {
  type?: string;
  properties?: Record<string, { type?: string; description?: string; [key: string]: unknown }>;
  required?: string[];
  [key: string]: unknown;
}

/**
 * An event type (trigger) from GET /events. Global types (is_global) belong
 * to the platform and are read-only for tenants (403 global_event_type_read_only).
 */
export interface EventType {
  id: ID;
  /** null for platform-global types. */
  tenant_id: ID | null;
  is_global: boolean;
  /** Mirrors is_global (legacy name), read-only. */
  is_predefined: boolean;
  category_id: ID | null;
  /** Immutable after creation. */
  slug: string;
  name: string;
  description: string | null;
  /** Optional JSON Schema subset: { type, properties, required }. */
  property_schema: EventPropertySchema | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

/** @deprecated Laravel name; use EventType. */
export type TriggerEvent = EventType;

export interface EventCategory {
  id: ID;
  tenant_id: ID | null;
  is_global: boolean;
  slug: string;
  name: string;
  description: string | null;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface CreateEventTypeData {
  name: string;
  /** Defaults to the snake_case form of name. */
  slug?: string;
  description?: string;
  category_id?: ID;
  property_schema?: EventPropertySchema;
  /** Defaults to true. */
  is_active?: boolean;
}

/** PATCH body: omitted fields stay untouched; category_id / property_schema null clears. Slug is immutable. */
export interface UpdateEventTypeData {
  name?: string;
  /** "" clears it. */
  description?: string;
  category_id?: ID | null;
  property_schema?: EventPropertySchema | null;
  is_active?: boolean;
}

/** @deprecated Laravel name; use CreateEventTypeData. */
export type CreateTriggerEventData = CreateEventTypeData;
/** @deprecated Laravel name; use UpdateEventTypeData. */
export type UpdateTriggerEventData = UpdateEventTypeData;

export interface EventTypeFilters extends CursorParams {
  /** Category id or slug. */
  category?: string;
  active?: boolean;
  /** Include platform-global types (default true). */
  include_global?: boolean;
  search?: string;
}

/** @deprecated use EventTypeFilters. */
export type TriggerEventFilters = EventTypeFilters;

// ==================== ACTIVITIES ====================

export type ActivityStatus = 'pending' | 'decided' | 'rejected';

/** What a tenant system reports; rules react to it asynchronously. */
export interface Activity {
  id: ID;
  event_id: string;
  event_type: string;
  player_external_id?: string;
  player_id?: ID;
  properties: Record<string, unknown>;
  context: Record<string, unknown>;
  occurred_at: string;
  received_at: string;
  status: ActivityStatus;
  decision_id?: ID;
  outcome?: string;
  reason?: string;
  decided_at?: string;
  causation_depth: number;
  source_event_id?: string;
  created_at: string;
}

export interface IngestActivityData {
  /** Idempotency: a repeated event_id returns the stored activity with duplicate=true. */
  event_id: string;
  event_type: string;
  player_external_id: string;
  occurred_at?: string;
  properties?: Record<string, unknown>;
  context?: Record<string, unknown>;
}

/** 202 for a new activity, 200 with duplicate=true for a repeated event_id. */
export interface IngestActivityResponse {
  activity_id: ID;
  status: ActivityStatus;
  duplicate: boolean;
  activity?: Activity;
}

export interface ActivityFilters extends CursorParams {
  event_type?: string;
  player_external_id?: string;
  status?: ActivityStatus;
}

// ==================== RULES: grammar v1 ====================
// See backend/internal/modules/rules/internal/domain/eval/grammar.go.

export type ConditionSource = 'trigger' | 'player' | 'activity';

export type ConditionOperator =
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
  | 'not_exists';

/**
 * One comparison. Fields per source:
 * - trigger: <dot.path> into the activity properties;
 * - activity: event_type | event_id | causation_depth | properties.<path> | context.<path>;
 * - player: id | external_id | display_name | email | is_active | level | xp | points | attributes.<path>.
 */
export interface ConditionLeaf {
  source: ConditionSource;
  field: string;
  operator: ConditionOperator;
  /** Omitted for exists / not_exists; in / not_in take an array. */
  value?: unknown;
}

export type ConditionNode =
  | ConditionLeaf
  | { all: ConditionNode[] }
  | { any: ConditionNode[] }
  | { not: ConditionNode };

/** A list is an implicit AND; null or [] always matches. */
export type RuleConditions = ConditionNode[] | ConditionNode | null;

export type RuleActionType =
  | 'credit_points'
  | 'grant_xp'
  | 'award_badge'
  | 'record_streak'
  | 'progress_mission'
  | 'grant_reward';

/** Amounts and increments are integers > 0. */
export type RuleAction =
  | { type: 'credit_points'; amount: number; description?: string }
  | { type: 'grant_xp'; amount: number; description?: string }
  | { type: 'award_badge'; badge_id: ID }
  | { type: 'record_streak'; streak_id?: ID; activity_key?: string }
  | { type: 'progress_mission'; mission_id: ID; increment?: number }
  | { type: 'grant_reward'; reward_id: ID };

/** @deprecated the Go API takes a list of RuleAction. */
export type RuleActions = RuleAction[];

/** Each a positive integer; enforced when a decision is made, not in simulation. */
export interface RuleLimits {
  max_per_player?: number;
  max_per_player_per_day?: number;
  max_per_player_per_week?: number;
  cooldown_seconds?: number;
}

// ==================== RULES ====================

export type RuleStatus = 'draft' | 'active' | 'inactive' | 'archived';

export interface RuleVersion {
  id: ID;
  rule_id: ID;
  version: number;
  conditions: RuleConditions;
  actions: RuleAction[];
  limits: RuleLimits | null;
  published: boolean;
  published_at: string | null;
  created_by?: ID;
  created_at: string;
}

export interface Rule {
  id: ID;
  tenant_id: ID;
  slug: string;
  name: string;
  description: string;
  trigger_event: string;
  program_id: ID | null;
  priority: number;
  status: RuleStatus;
  current_version_id: ID | null;
  /** Only on create / show / update responses, not in lists. */
  current_version?: RuleVersion;
  latest_version?: RuleVersion;
  created_at: string;
  updated_at: string;
}

/** Creates the rule as a draft with version 1. */
export interface CreateRuleData {
  name: string;
  slug?: string;
  description?: string;
  trigger_event: string;
  program_id?: ID;
  priority?: number;
  conditions?: RuleConditions;
  actions: RuleAction[];
  limits?: RuleLimits;
}

/**
 * PATCH body. conditions/actions/limits edit the latest version only while it
 * is a draft (409 no_draft_version). status "active" needs a published version.
 */
export interface UpdateRuleData {
  name?: string;
  description?: string | null;
  trigger_event?: string;
  program_id?: ID | null;
  priority?: number;
  status?: Exclude<RuleStatus, 'draft'>;
  conditions?: RuleConditions;
  actions?: RuleAction[];
  limits?: RuleLimits | null;
}

/** New draft version; omitted parts are copied from from_version (default: latest). */
export interface CreateRuleVersionData {
  from_version?: number;
  conditions?: RuleConditions;
  actions?: RuleAction[];
  limits?: RuleLimits;
}

export interface RuleFilters extends CursorParams {
  status?: RuleStatus;
  trigger_event?: string;
  program_id?: ID;
}

// ==================== SIMULATION ====================

export interface SimulatePlayer {
  external_id?: string;
  is_active?: boolean;
  attributes?: Record<string, unknown>;
  level?: number;
  xp?: number;
  points?: number;
}

/** A hypothetical activity evaluated against the live ruleset; nothing is written. */
export interface SimulateRulesData {
  event_type: string;
  player_id?: ID;
  player_external_id?: string;
  player?: SimulatePlayer;
  properties?: Record<string, unknown>;
  context?: Record<string, unknown>;
  causation_depth?: number;
}

export interface ConditionTrace {
  path: string;
  source: string;
  field: string;
  operator: string;
  expected?: unknown;
  actual?: unknown;
  present: boolean;
  result: boolean;
}

export interface SimulatedEffect {
  action_index: number;
  type: RuleActionType;
  params: Record<string, unknown>;
}

export interface SimulatedRule {
  rule_id: ID;
  rule_version_id: ID;
  name: string;
  priority: number;
  /** matched | not_matched | out_of_scope | invalid */
  status: string;
  matched: boolean;
  condition_results: ConditionTrace[];
  effects: SimulatedEffect[];
  limits?: RuleLimits;
  error?: string;
}

export interface SimulationResult {
  outcome: string;
  reason?: string;
  player_id?: ID;
  ruleset_generation: number;
  /** Simulation reports limits but never enforces them. */
  limits_enforced: boolean;
  rules: SimulatedRule[];
}

// ==================== DECISIONS ====================

export interface RuleDecision {
  id: ID;
  tenant_id: ID;
  activity_id: ID;
  event_id?: string;
  player_id: ID | null;
  player_external_id?: string;
  event_type: string;
  outcome: string;
  reason?: string;
  ruleset_generation: number;
  causation_depth: number;
  occurred_at: string;
  evaluated_at: string;
  duration_us: number;
}

export interface RuleExecutionRecord {
  id: ID;
  rule_id: ID;
  rule_version_id: ID;
  status: string;
  matched: boolean;
  condition_results: ConditionTrace[];
  effects_count: number;
  created_at: string;
}

export interface RuleEffect {
  id: ID;
  execution_id: ID;
  rule_id: ID;
  rule_version_id: ID;
  action_index: number;
  idempotency_key: string;
  type: RuleActionType;
  params: Record<string, unknown>;
  target: string;
  status: string;
  reason?: string;
  attempts: number;
  requested_at: string;
  settled_at: string | null;
}

export interface RuleDecisionDetail extends RuleDecision {
  executions: RuleExecutionRecord[];
  effects: RuleEffect[];
}

export interface RuleDecisionFilters extends CursorParams {
  activity_id?: ID;
  player_id?: ID;
}

// ==================== REMOVED (Laravel) ====================

/**
 * @deprecated POST /rules/execute is gone (410). Report activities with
 * POST /activities and preview with POST /rules/simulate (SimulateRulesData).
 */
export interface ExecuteRulesData {
  player_id: ID;
  trigger_event: string;
  trigger_data?: Record<string, unknown>;
}

/** @deprecated use RuleDecision / RuleExecutionRecord. */
export type RuleExecution = RuleExecutionRecord;

/** @deprecated use RuleDecisionFilters. */
export type RuleExecutionFilters = RuleDecisionFilters;

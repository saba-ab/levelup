// Rules v2 models: schedule, stop_processing, history conditions, draft
// simulation and stats. Field-for-field with the Go DTOs in
// backend/internal/modules/rules/internal/transport/dto.go and the grammar in
// backend/internal/modules/rules/internal/domain/eval/{grammar,schedule,history}.go.
// The v1 shapes (Rule, RuleVersion, ...) stay in ./events.ts.
import type { ID } from './common';
import type {
  ConditionLeaf,
  CreateRuleData,
  CreateRuleVersionData,
  Rule,
  RuleAction,
  RuleConditions,
  RuleLimits,
  RuleVersion,
  SimulatedRule,
  SimulateRulesData,
  SimulationResult,
  UpdateRuleData,
} from './events';

// ==================== SCHEDULE ====================

/** 0 = Sunday … 6 = Saturday, in the schedule's timezone. */
export type RuleWeekday = 0 | 1 | 2 | 3 | 4 | 5 | 6;

/**
 * When a rule may fire, judged on the activity's occurred_at (never the
 * clock). starts_at is inclusive, ends_at exclusive; hours is [from, to) in
 * the timezone and wraps midnight when from > to. Outside it, the rule is
 * recorded as out_of_schedule. null / absent = always.
 */
export interface RuleSchedule {
  /** RFC 3339. */
  starts_at?: string | null;
  /** RFC 3339, must be after starts_at. */
  ends_at?: string | null;
  /** Non-empty when present. */
  days_of_week?: RuleWeekday[] | null;
  /** "HH:MM" each; from must differ from to. */
  hours?: { from: string; to: string } | null;
  /** IANA name; default UTC. */
  timezone?: string | null;
}

// ==================== HISTORY CONDITIONS (grammar v2) ====================

export type HistoryWindow = '1d' | '7d' | '30d' | '90d' | 'all';

/** True when the player has no prior activity of the trigger event type. */
export interface HistoryFirstTimeLeaf {
  source: 'history';
  field: 'first_time';
  /** eq (default) or neq. */
  operator?: 'eq' | 'neq';
  /** Default true. */
  value?: boolean;
}

/**
 * Number of PRIOR activities (the current one excluded) of event_type
 * (default: the trigger) in a window of UTC days ending on the activity's day.
 */
export interface HistoryCountLeaf {
  source: 'history';
  field: 'count';
  event_type?: string;
  window: HistoryWindow;
  operator: 'eq' | 'neq' | 'gt' | 'gte' | 'lt' | 'lte' | 'in' | 'not_in';
  value: number | number[];
}

export type HistoryLeaf = HistoryFirstTimeLeaf | HistoryCountLeaf;

/** Any leaf of grammar v2. */
export type ConditionLeafV2 = ConditionLeaf | HistoryLeaf;

// ==================== RULE / VERSION (v2 fields) ====================

export interface RuleVersionV2 extends RuleVersion {
  schedule: RuleSchedule | null;
  stop_processing: boolean;
}

export interface RuleV2 extends Omit<Rule, 'current_version' | 'latest_version'> {
  current_version?: RuleVersionV2;
  latest_version?: RuleVersionV2;
}

/** The v2 definition parts every write accepts. */
export interface RuleDefinitionV2Parts {
  schedule?: RuleSchedule | null;
  stop_processing?: boolean;
}

export type CreateRuleDataV2 = CreateRuleData & RuleDefinitionV2Parts;
/** schedule: null clears it. Definition parts edit the latest draft only. */
export type UpdateRuleDataV2 = UpdateRuleData & RuleDefinitionV2Parts;
/** Omitted parts are copied from from_version; null clears limits / schedule. */
export type CreateRuleVersionDataV2 = Omit<CreateRuleVersionData, 'limits'> & {
  limits?: RuleLimits | null;
} & RuleDefinitionV2Parts;

// ==================== SIMULATION (draft) ====================

/** An unpublished rule body: simulate it alone instead of the live ruleset. */
export interface DraftRuleDefinition {
  trigger_event: string;
  conditions?: RuleConditions;
  actions: RuleAction[];
  limits?: RuleLimits | null;
  schedule?: RuleSchedule | null;
  stop_processing?: boolean;
}

/**
 * With definition, only the draft is evaluated and event_type defaults to its
 * trigger_event (a different one is 422 draft_trigger_mismatch). Compile
 * errors come back as 422 invalid_rule_definition keyed "definition.<path>".
 * occurred_at (default now) drives schedules and history windows.
 */
export interface SimulateRulesDataV2 extends Omit<SimulateRulesData, 'event_type'> {
  event_type?: string;
  occurred_at?: string;
  definition?: DraftRuleDefinition;
}

/** matched | not_matched | out_of_scope | invalid | out_of_schedule | skipped_by_stop */
export type SimulatedRuleStatus =
  | 'matched'
  | 'not_matched'
  | 'out_of_scope'
  | 'invalid'
  | 'out_of_schedule'
  | 'skipped_by_stop';

export interface SimulatedRuleV2 extends SimulatedRule {
  stop_processing: boolean;
  /** The rule whose firing skipped this one (status skipped_by_stop). */
  stopped_by?: ID;
}

export interface SimulationResultV2 extends Omit<SimulationResult, 'rules'> {
  /** True when the draft definition was simulated (its rule_id is "draft"). */
  draft: boolean;
  /** history.* facts came from the stored player's activity (false for inline players). */
  history_loaded: boolean;
  occurred_at: string;
  rules: SimulatedRuleV2[];
}

/** rule_id / rule_version_id of the draft in a draft simulation. */
export const DRAFT_RULE_ID = 'draft';

// ==================== STATS ====================

/** One rule's executions over [from, to). */
export interface RuleStat {
  rule_id: ID;
  name: string;
  fired: number;
  not_matched: number;
  limited: number;
  out_of_schedule: number;
  skipped_by_stop: number;
  effects_applied: number;
  effects_rejected: number;
  /** Sum of credit_points amounts of fired executions. */
  points_awarded: number;
  /** Sum of grant_xp amounts of fired executions. */
  xp_awarded: number;
}

export type RuleStatsTotals = Omit<RuleStat, 'rule_id' | 'name'>;

/** GET /rules/stats: rows ordered by fired DESC, plus totals. */
export interface RuleStats {
  from: string;
  to: string;
  data: RuleStat[];
  totals: RuleStatsTotals;
}

/** RFC 3339 or YYYY-MM-DD (UTC midnight); default to = now, from = to - 30 days; at most 366 days. */
export interface RuleStatsParams {
  from?: string;
  to?: string;
}

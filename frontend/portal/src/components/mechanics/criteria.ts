// Mission criteria grammar helpers (missions/internal/domain/criteria.go):
// {event_type, where?: [{field, operator, value}], increment?: {by: "count"} | {by: "property", field}}.
import type {
  MissionCriteria,
  MissionCriteriaCondition,
  MissionCriteriaOperator,
  MissionCriteriaScalar,
} from '@/services/api/types';

export const criteriaOperatorLabels: Record<MissionCriteriaOperator, string> = {
  eq: 'equals',
  neq: 'does not equal',
  gt: 'greater than',
  gte: 'at least',
  lt: 'less than',
  lte: 'at most',
  in: 'is one of',
  contains: 'contains',
  exists: 'exists',
};

export const CRITERIA_OPERATORS = Object.keys(criteriaOperatorLabels) as MissionCriteriaOperator[];
export const MAX_CRITERIA_CONDITIONS = 20;
const MAX_IN_VALUES = 100;
const EVENT_TYPE_RE = /^[a-z0-9_.:-]+$/;
const FIELD_PATH_RE = /^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$/;

export type CriteriaValueType = 'string' | 'number' | 'boolean';

/** Operators whose value is always numeric. */
export const NUMERIC_OPERATORS: MissionCriteriaOperator[] = ['gt', 'gte', 'lt', 'lte'];

export interface CriteriaConditionDraft {
  field: string;
  operator: MissionCriteriaOperator;
  /** How `value` is sent for eq/neq/contains/in. */
  valueType: CriteriaValueType;
  /** Raw input; comma-separated for "in", "true"/"false" for exists. */
  value: string;
}

export interface CriteriaDraft {
  event_type: string;
  where: CriteriaConditionDraft[];
  incrementBy: 'count' | 'property';
  incrementField: string;
}

export const emptyCriteriaDraft: CriteriaDraft = { event_type: '', where: [], incrementBy: 'count', incrementField: '' };

const typeOf = (v: unknown): CriteriaValueType =>
  typeof v === 'number' ? 'number' : typeof v === 'boolean' ? 'boolean' : 'string';

/**
 * Reads stored criteria into the editor. `unsupported` is true when the
 * JSON holds keys or shapes outside the grammar (they are dropped on save).
 */
export function criteriaToDraft(raw: Record<string, unknown> | null | undefined): {
  draft: CriteriaDraft;
  unsupported: boolean;
} {
  if (!raw || Object.keys(raw).length === 0) return { draft: emptyCriteriaDraft, unsupported: false };
  let unsupported = Object.keys(raw).some((k) => k !== 'event_type' && k !== 'where' && k !== 'increment');
  const draft: CriteriaDraft = { ...emptyCriteriaDraft, where: [] };
  if (typeof raw.event_type === 'string') draft.event_type = raw.event_type;
  else if (raw.event_type !== undefined) unsupported = true;

  if (Array.isArray(raw.where)) {
    raw.where.forEach((item) => {
      const c = item as Partial<MissionCriteriaCondition> | null;
      if (!c || typeof c !== 'object' || typeof c.field !== 'string' || !CRITERIA_OPERATORS.includes(c.operator as MissionCriteriaOperator)) {
        unsupported = true;
        return;
      }
      const op = c.operator as MissionCriteriaOperator;
      const v = c.value;
      if (op === 'exists') {
        draft.where.push({ field: c.field, operator: op, valueType: 'boolean', value: v === false ? 'false' : 'true' });
      } else if (Array.isArray(v)) {
        draft.where.push({ field: c.field, operator: op, valueType: typeOf(v[0]), value: v.map(String).join(', ') });
      } else {
        draft.where.push({ field: c.field, operator: op, valueType: typeOf(v), value: v === undefined || v === null ? '' : String(v) });
      }
    });
  } else if (raw.where !== undefined && raw.where !== null) {
    unsupported = true;
  }

  const inc = raw.increment as { by?: unknown; field?: unknown } | undefined | null;
  if (inc && typeof inc === 'object') {
    if (inc.by === 'property' && typeof inc.field === 'string') {
      draft.incrementBy = 'property';
      draft.incrementField = inc.field;
    } else if (inc.by !== 'count') {
      unsupported = true;
    }
  }
  return { draft, unsupported };
}

function toScalar(raw: string, type: CriteriaValueType): MissionCriteriaScalar | undefined {
  const s = raw.trim();
  if (type === 'number') {
    if (s === '') return undefined;
    const n = Number(s);
    return Number.isFinite(n) ? n : undefined;
  }
  if (type === 'boolean') {
    if (s === 'true') return true;
    if (s === 'false') return false;
    return undefined;
  }
  return s;
}

function conditionValue(c: CriteriaConditionDraft): MissionCriteriaCondition['value'] {
  if (c.operator === 'exists') return c.value !== 'false';
  if (NUMERIC_OPERATORS.includes(c.operator)) return toScalar(c.value, 'number');
  if (c.operator === 'in') {
    const parts = c.value.split(',').map((p) => p.trim()).filter((p) => p !== '');
    return parts.map((p) => toScalar(p, c.valueType)).filter((v): v is MissionCriteriaScalar => v !== undefined);
  }
  return toScalar(c.value, c.valueType);
}

/**
 * Client-side check mirroring ParseCriteria; returns errors keyed like the
 * server's field errors ("criteria.where[0].value"). Empty when valid.
 */
export function validateCriteriaDraft(draft: CriteriaDraft): Record<string, string> {
  const errors: Record<string, string> = {};
  const hasRules = draft.where.length > 0 || draft.incrementBy === 'property';
  if (!draft.event_type) {
    if (hasRules) errors['criteria.event_type'] = 'is required when conditions or a property increment are set';
    return errors;
  }
  if (draft.event_type.length > 100 || !EVENT_TYPE_RE.test(draft.event_type)) {
    errors['criteria.event_type'] = 'must be an event type slug (a-z, 0-9, _ . : -), at most 100 characters';
  }
  if (draft.where.length > MAX_CRITERIA_CONDITIONS) {
    errors['criteria.where'] = `at most ${MAX_CRITERIA_CONDITIONS} conditions`;
  }
  draft.where.forEach((c, i) => {
    const at = `criteria.where[${i}]`;
    if (!c.field || c.field.length > 200 || !FIELD_PATH_RE.test(c.field)) {
      errors[`${at}.field`] = 'must be a dot path into properties (letters, digits, _ and -)';
    }
    const v = conditionValue(c);
    if (c.operator === 'in') {
      const list = v as MissionCriteriaScalar[];
      const raw = c.value.split(',').filter((p) => p.trim() !== '');
      if (list.length === 0) errors[`${at}.value`] = 'list at least one value, comma-separated';
      else if (list.length !== raw.length) errors[`${at}.value`] = `every value must be a ${c.valueType}`;
      else if (list.length > MAX_IN_VALUES) errors[`${at}.value`] = `at most ${MAX_IN_VALUES} values`;
    } else if (c.operator !== 'exists' && v === undefined) {
      errors[`${at}.value`] = NUMERIC_OPERATORS.includes(c.operator) ? 'must be a number' : `must be a ${c.valueType}`;
    }
  });
  if (draft.incrementBy === 'property' && (!draft.incrementField || !FIELD_PATH_RE.test(draft.incrementField))) {
    errors['criteria.increment.field'] = 'must be a dot path into properties (letters, digits, _ and -)';
  }
  return errors;
}

/** The criteria payload. {} (no event type) keeps the mission manual / rule-driven. */
export function draftToCriteria(draft: CriteriaDraft): MissionCriteria {
  if (!draft.event_type) return {};
  const out: MissionCriteria = { event_type: draft.event_type };
  if (draft.where.length > 0) {
    out.where = draft.where.map((c) => ({ field: c.field.trim(), operator: c.operator, value: conditionValue(c) }));
  }
  if (draft.incrementBy === 'property') out.increment = { by: 'property', field: draft.incrementField.trim() };
  return out;
}

const formatValue = (v: MissionCriteriaCondition['value']) =>
  Array.isArray(v) ? `[${v.map((x) => JSON.stringify(x)).join(', ')}]` : JSON.stringify(v);

/** Human summary of stored criteria for cards, or null when the mission is manual. */
export function summarizeCriteria(raw: Record<string, unknown> | null | undefined): {
  eventType: string;
  conditions: string[];
  increment: string;
} | null {
  const { draft } = criteriaToDraft(raw);
  if (!draft.event_type) return null;
  const criteria = draftToCriteria(draft);
  return {
    eventType: draft.event_type,
    conditions: (criteria.where ?? []).map((c) =>
      c.operator === 'exists'
        ? `${c.field} ${c.value === false ? 'is missing' : 'exists'}`
        : `${c.field} ${criteriaOperatorLabels[c.operator]} ${formatValue(c.value)}`,
    ),
    increment: draft.incrementBy === 'property' ? `+ value of ${draft.incrementField}` : '+1 per activity',
  };
}

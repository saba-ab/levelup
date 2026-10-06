// Editor model for segment conditions: a mutable tree with stable keys that
// converts to and from the API's {"all"|"any": [...]} grammar.
import type {
  SegmentCondition,
  SegmentConditionGroup,
  SegmentConditionItem,
  SegmentMatch,
  SegmentOperator,
} from '@/services/api/models/segments';

export const MAX_DEPTH = 3;
export const MAX_CONDITIONS = 50;

export type FieldKind =
  | 'attributes'
  | 'is_active'
  | 'created_at'
  | 'level'
  | 'balance'
  | 'lifetime_earned'
  | 'last_seen_days'
  | 'badges_earned';

export const FIELD_OPTIONS: { value: FieldKind; label: string }[] = [
  { value: 'level', label: 'Level' },
  { value: 'balance', label: 'Points balance' },
  { value: 'lifetime_earned', label: 'Lifetime points earned' },
  { value: 'last_seen_days', label: 'Days since last seen' },
  { value: 'badges_earned', label: 'Badges earned' },
  { value: 'is_active', label: 'Is active' },
  { value: 'created_at', label: 'Created' },
  { value: 'attributes', label: 'Attribute' },
];

export const OPERATOR_LABELS: Record<SegmentOperator, string> = {
  eq: 'equals',
  neq: 'does not equal',
  gt: 'greater than',
  gte: 'at least',
  lt: 'less than',
  lte: 'at most',
  in: 'is one of',
  not_in: 'is not one of',
  contains: 'contains',
  exists: 'is set',
  not_exists: 'is not set',
  before: 'before',
  after: 'after',
  has: 'has badge',
  not_has: 'does not have badge',
};

const NUMERIC_OPS: SegmentOperator[] = ['eq', 'neq', 'gt', 'gte', 'lt', 'lte'];

export function operatorsFor(field: FieldKind): SegmentOperator[] {
  switch (field) {
    case 'attributes':
      return ['eq', 'neq', 'gt', 'gte', 'lt', 'lte', 'in', 'not_in', 'contains', 'exists', 'not_exists'];
    case 'is_active':
      return ['eq', 'neq'];
    case 'created_at':
      return ['before', 'after'];
    case 'badges_earned':
      return [...NUMERIC_OPS, 'has', 'not_has'];
    default:
      return NUMERIC_OPS;
  }
}

/** The kind of value input a field/operator pair needs. */
export type ValueKind = 'none' | 'number' | 'boolean' | 'date' | 'badge' | 'list' | 'text';

export function valueKindFor(field: FieldKind, op: SegmentOperator): ValueKind {
  if (field === 'is_active') return 'boolean';
  if (field === 'created_at') return 'date';
  if (field === 'badges_earned') return op === 'has' || op === 'not_has' ? 'badge' : 'number';
  if (field === 'attributes') {
    if (op === 'exists' || op === 'not_exists') return 'none';
    if (op === 'in' || op === 'not_in') return 'list';
    return 'text';
  }
  return 'number';
}

export interface EditorCondition {
  kind: 'condition';
  key: string;
  field: FieldKind;
  /** Dot path after "attributes." (attributes only). */
  attrPath: string;
  op: SegmentOperator;
  /** Raw input; for lists, comma-separated. Booleans are "true"/"false". */
  value: string;
}

export interface EditorGroup {
  kind: 'group';
  key: string;
  match: SegmentMatch;
  items: EditorNode[];
}

export type EditorNode = EditorCondition | EditorGroup;

let keySeq = 0;
const nextKey = () => `n${++keySeq}`;

export function newCondition(field: FieldKind = 'level'): EditorCondition {
  const op = operatorsFor(field)[0];
  return { kind: 'condition', key: nextKey(), field, attrPath: '', op, value: defaultValue(field, op) };
}

export function newGroup(match: SegmentMatch = 'all', items: EditorNode[] = [newCondition()]): EditorGroup {
  return { kind: 'group', key: nextKey(), match, items };
}

export function defaultValue(field: FieldKind, op: SegmentOperator): string {
  return valueKindFor(field, op) === 'boolean' ? 'true' : '';
}

export function countConditions(g: EditorGroup): number {
  return g.items.reduce((n, item) => n + (item.kind === 'group' ? countConditions(item) : 1), 0);
}

// ---- API -> editor --------------------------------------------------------

function isGroup(item: SegmentConditionItem): item is SegmentConditionGroup {
  return typeof item === 'object' && item !== null && ('all' in item || 'any' in item);
}

function valueToString(v: unknown): string {
  if (v === undefined || v === null) return '';
  if (Array.isArray(v)) return v.map(valueToString).join(', ');
  return String(v);
}

function fromCondition(c: SegmentCondition): EditorCondition {
  const isAttr = c.field.startsWith('attributes.');
  const field = (isAttr ? 'attributes' : c.field) as FieldKind;
  let value = valueToString(c.value);
  if (field === 'created_at' && value.length > 10) value = value.slice(0, 10);
  return {
    kind: 'condition',
    key: nextKey(),
    field,
    attrPath: isAttr ? c.field.slice('attributes.'.length) : '',
    op: c.op,
    value,
  };
}

export function fromConditions(group: SegmentConditionGroup | null | undefined): EditorGroup {
  if (!group) return newGroup();
  const match: SegmentMatch = 'any' in group && group.any ? 'any' : 'all';
  const items = (match === 'any' ? group.any : group.all) ?? [];
  return {
    kind: 'group',
    key: nextKey(),
    match,
    items: items.map((item) => (isGroup(item) ? fromConditions(item) : fromCondition(item as SegmentCondition))),
  };
}

// ---- editor -> API --------------------------------------------------------

const ATTR_PATH_RE = /^[A-Za-z0-9_-]{1,64}(\.[A-Za-z0-9_-]{1,64}){0,4}$/;

/** Attribute scalars: numbers and booleans when they look like one, else text. */
function parseScalar(raw: string): string | number | boolean {
  const s = raw.trim();
  if (s === 'true') return true;
  if (s === 'false') return false;
  if (s !== '' && !Number.isNaN(Number(s))) return Number(s);
  return s;
}

function toCondition(c: EditorCondition, path: string, errors: string[]): SegmentCondition | null {
  const label = FIELD_OPTIONS.find((f) => f.value === c.field)?.label ?? c.field;
  const fail = (msg: string) => {
    errors.push(`${path}: ${msg}`);
    return null;
  };
  let field: string = c.field;
  if (c.field === 'attributes') {
    if (!ATTR_PATH_RE.test(c.attrPath.trim())) {
      return fail('enter an attribute path such as "plan" or "address.country"');
    }
    field = `attributes.${c.attrPath.trim()}`;
  }
  const kind = valueKindFor(c.field, c.op);
  const raw = c.value.trim();
  switch (kind) {
    case 'none':
      return { field, op: c.op };
    case 'boolean':
      return { field, op: c.op, value: raw === 'true' };
    case 'date':
      if (!/^\d{4}-\d{2}-\d{2}$/.test(raw)) return fail(`${label} needs a date`);
      return { field, op: c.op, value: raw };
    case 'badge':
      if (!raw) return fail('choose a badge');
      return { field, op: c.op, value: raw };
    case 'number':
      if (raw === '' || Number.isNaN(Number(raw))) return fail(`${label} needs a number`);
      return { field, op: c.op, value: Number(raw) };
    case 'list': {
      const values = raw.split(',').map((v) => v.trim()).filter(Boolean);
      if (values.length === 0 || values.length > 100) return fail('enter 1 to 100 comma-separated values');
      return { field, op: c.op, value: values.map(parseScalar) };
    }
    case 'text':
      if (raw === '') return fail('enter a value');
      return { field, op: c.op, value: c.op === 'contains' ? raw : parseScalar(raw) };
  }
}

function toGroup(g: EditorGroup, path: string, errors: string[]): SegmentConditionGroup {
  const items: SegmentConditionItem[] = [];
  if (g.items.length === 0) errors.push(`${path || 'root'}: a group needs at least one condition`);
  g.items.forEach((item, i) => {
    const p = `${path ? `${path}.` : ''}${g.match}[${i}]`;
    if (item.kind === 'group') items.push(toGroup(item, p, errors));
    else {
      const c = toCondition(item, p, errors);
      if (c) items.push(c);
    }
  });
  return g.match === 'all' ? { all: items } : { any: items };
}

/** The API document, or the list of problems that keep it from being valid. */
export function toConditions(g: EditorGroup): { conditions: SegmentConditionGroup | null; errors: string[] } {
  const errors: string[] = [];
  const n = countConditions(g);
  if (n > MAX_CONDITIONS) errors.push(`at most ${MAX_CONDITIONS} conditions (you have ${n})`);
  const conditions = toGroup(g, '', errors);
  return { conditions: errors.length ? null : conditions, errors };
}

// ---- tree edits -------------------------------------------------------------

/** Returns a copy of the tree with the node at `key` replaced (or removed when null). */
export function replaceNode(g: EditorGroup, key: string, next: EditorNode | null): EditorGroup {
  return {
    ...g,
    items: g.items.flatMap((item) => {
      if (item.key === key) return next ? [next] : [];
      return item.kind === 'group' ? [replaceNode(item, key, next)] : [item];
    }),
  };
}

// ---- human-readable summary ---------------------------------------------------

export function describeCondition(c: SegmentCondition, badgeName?: (id: string) => string | undefined): string {
  const isAttr = c.field.startsWith('attributes.');
  const label = isAttr
    ? `attribute "${c.field.slice('attributes.'.length)}"`
    : (FIELD_OPTIONS.find((f) => f.value === c.field)?.label ?? c.field);
  const op = OPERATOR_LABELS[c.op] ?? c.op;
  if (c.op === 'exists' || c.op === 'not_exists') return `${label} ${op}`;
  if (c.op === 'has' || c.op === 'not_has') {
    const id = String(c.value);
    return `${op} ${badgeName?.(id) ?? id}`;
  }
  return `${label} ${op} ${valueToString(c.value)}`;
}

import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams, useSearchParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowLeft,
  Plus,
  Trash2,
  Zap,
  GitBranch,
  Award,
  Target,
  Save,
  ExternalLink,
  Loader2,
  AlertCircle,
  Gauge,
  History,
  Braces,
  CalendarClock,
  OctagonX,
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';
import { useApi } from '@/hooks/useApi';
import { cn } from '@/lib/utils';
import { BADGE_ENDPOINTS, MISSION_ENDPOINTS, PROGRAM_ENDPOINTS, REWARD_ENDPOINTS, STREAK_ENDPOINTS, toQuery } from '@/lib/api-routes';
import { fetchAllPages } from '@/services/api/pagination';
import { useEventsQuery } from '@/services/queries/events';
import {
  ApiRequestError,
  unwrap,
  useRuleQuery,
  useRuleVersionsQuery,
  useCreateRuleMutation,
  useUpdateRuleMutation,
  useCreateRuleVersionMutation,
  usePublishRuleMutation,
} from '@/services/queries/rules';
import type {
  ConditionLeaf,
  ConditionNode,
  ConditionOperator,
  ConditionSource,
  CursorPage,
  RuleAction,
  RuleActionType,
  RuleConditions,
  RuleLimits,
} from '@/services/api/types';
import type {
  DraftRuleDefinition,
  HistoryLeaf,
  HistoryWindow,
  RuleSchedule,
  RuleV2,
  RuleWeekday,
  UpdateRuleDataV2,
} from '@/services/api/models/rules';
import RuleSimulator from '@/components/RuleSimulator';

// ==================== builder model ====================

type ValueKind = 'text' | 'number' | 'boolean' | 'list';

/** Grammar v2 adds "history" (aggregates over the player's past activity). */
type BuilderSource = ConditionSource | 'history';

interface ConditionRow {
  id: string;
  source: BuilderSource;
  /** For history: first_time | count. */
  field: string;
  operator: ConditionOperator;
  value: string;
  kind: ValueKind;
  negate: boolean;
  /** history.count only: blank = the trigger event. */
  eventType: string;
  /** history.count only. */
  window: HistoryWindow;
}

interface ActionRow {
  id: string;
  type: RuleActionType;
  amount: string;
  description: string;
  badge_id: string;
  streak_id: string;
  activity_key: string;
  mission_id: string;
  increment: string;
  reward_id: string;
}

type LimitKey = keyof RuleLimits;

const newId = () => (typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Math.random()}`);

const operatorLabels: Record<ConditionOperator, string> = {
  eq: '=',
  neq: '≠',
  gt: '>',
  gte: '≥',
  lt: '<',
  lte: '≤',
  in: 'in',
  not_in: 'not in',
  contains: 'contains',
  exists: 'exists',
  not_exists: 'does not exist',
};
const OPERATORS = Object.keys(operatorLabels) as ConditionOperator[];
const NO_VALUE: ConditionOperator[] = ['exists', 'not_exists'];
const LIST_OPS: ConditionOperator[] = ['in', 'not_in'];

const sourceLabels: Record<BuilderSource, string> = {
  trigger: 'Event property',
  player: 'Player',
  activity: 'Activity',
  history: 'Player history',
};

const HISTORY_OPS: ConditionOperator[] = ['eq', 'neq', 'gt', 'gte', 'lt', 'lte', 'in', 'not_in'];
const windowLabels: Record<HistoryWindow, string> = {
  '1d': 'today (UTC day)',
  '7d': 'last 7 days',
  '30d': 'last 30 days',
  '90d': 'last 90 days',
  all: 'all time',
};
const HISTORY_WINDOWS = Object.keys(windowLabels) as HistoryWindow[];
const EVENT_SLUG_RE = /^[a-z0-9_.:-]+$/;

const WEEKDAYS: { day: RuleWeekday; label: string }[] = [
  { day: 1, label: 'Mon' },
  { day: 2, label: 'Tue' },
  { day: 3, label: 'Wed' },
  { day: 4, label: 'Thu' },
  { day: 5, label: 'Fri' },
  { day: 6, label: 'Sat' },
  { day: 0, label: 'Sun' },
];

const PLAYER_FIELDS = ['level', 'xp', 'points', 'is_active', 'external_id', 'display_name', 'email', 'id', 'attributes.'];
const ACTIVITY_FIELDS = ['event_type', 'event_id', 'causation_depth', 'properties.', 'context.'];

const actionLabels: Record<RuleActionType, string> = {
  credit_points: 'Credit Points',
  grant_xp: 'Grant XP',
  award_badge: 'Award Badge',
  record_streak: 'Record Streak',
  progress_mission: 'Progress Mission',
  grant_reward: 'Grant Reward',
};
const ACTION_TYPES = Object.keys(actionLabels) as RuleActionType[];

const limitLabels: Record<LimitKey, string> = {
  max_per_player: 'Max per player (lifetime)',
  max_per_player_per_day: 'Max per player per day',
  max_per_player_per_week: 'Max per player per week',
  cooldown_seconds: 'Cooldown (seconds)',
};
const LIMIT_KEYS = Object.keys(limitLabels) as LimitKey[];

function emptyAction(type: RuleActionType): ActionRow {
  return {
    id: newId(),
    type,
    amount: '',
    description: '',
    badge_id: '',
    streak_id: '',
    activity_key: '',
    mission_id: '',
    increment: '1',
    reward_id: '',
  };
}

function kindOf(value: unknown): ValueKind {
  if (Array.isArray(value)) return 'list';
  if (typeof value === 'number') return 'number';
  if (typeof value === 'boolean') return 'boolean';
  return 'text';
}

function valueToString(value: unknown): string {
  if (value === undefined || value === null) return '';
  if (Array.isArray(value)) return value.map(v => String(v)).join(', ');
  return String(value);
}

function parseScalar(raw: string, kind: ValueKind): unknown {
  const s = raw.trim();
  if (kind === 'number') return Number(s);
  if (kind === 'boolean') return s === 'true';
  return s;
}

/** Builder rows -> grammar value; list items that look numeric become numbers. */
function rowValue(row: ConditionRow): unknown {
  if (NO_VALUE.includes(row.operator)) return undefined;
  if (row.kind === 'list' || LIST_OPS.includes(row.operator)) {
    return row.value
      .split(',')
      .map(v => v.trim())
      .filter(Boolean)
      .map(v => (v !== '' && !Number.isNaN(Number(v)) ? Number(v) : v));
  }
  return parseScalar(row.value, row.kind);
}

/** The v1 ConditionNode type predates history leaves; the server grammar accepts both. */
function historyNode(leaf: HistoryLeaf): ConditionNode {
  return leaf as unknown as ConditionNode;
}

function rowToLeaf(row: ConditionRow): ConditionNode {
  if (row.source === 'history') {
    const leaf: HistoryLeaf =
      row.field === 'first_time'
        ? { source: 'history', field: 'first_time', operator: row.operator === 'neq' ? 'neq' : 'eq', value: row.value !== 'false' }
        : {
            source: 'history',
            field: 'count',
            window: row.window,
            operator: row.operator as Exclude<HistoryLeaf['operator'], undefined>,
            value: rowValue(row) as number | number[],
            ...(row.eventType.trim() ? { event_type: row.eventType.trim() } : {}),
          };
    return row.negate ? { not: historyNode(leaf) } : historyNode(leaf);
  }
  const leaf: ConditionLeaf = { source: row.source, field: row.field.trim(), operator: row.operator };
  const value = rowValue(row);
  if (value !== undefined) leaf.value = value;
  return row.negate ? { not: leaf } : leaf;
}

/** "all" is the flat list (implicit AND); "any" wraps the rows in {any: [...]}. */
function buildConditions(rows: ConditionRow[], mode: 'all' | 'any'): RuleConditions {
  const nodes = rows.map(rowToLeaf);
  if (nodes.length === 0) return [];
  return mode === 'all' ? nodes : [{ any: nodes }];
}

/** Server error path prefix for a row, matching how the compiler reports it. */
function rowPath(index: number, mode: 'all' | 'any', negate: boolean): string {
  const base = mode === 'all' ? `conditions[${index}]` : `conditions[0].any[${index}]`;
  return negate ? `${base}.not` : base;
}

function isLeaf(n: unknown): n is ConditionLeaf {
  return typeof n === 'object' && n !== null && 'field' in n && ('source' in n || 'type' in n);
}

function leafToRow(n: unknown): ConditionRow | null {
  let negate = false;
  let node = n;
  if (typeof node === 'object' && node !== null && 'not' in node) {
    negate = true;
    node = (node as { not: unknown }).not;
  }
  if (!isLeaf(node)) return null;
  const leaf = node as Omit<ConditionLeaf, 'source'> & {
    source?: string;
    type?: string;
    event_type?: string;
    window?: string;
  };
  const source = leaf.source ?? leaf.type ?? 'trigger';
  if (!(source in sourceLabels)) return null;
  if (source === 'history') {
    if (leaf.field === 'first_time') {
      return {
        ...emptyCondition('history'),
        operator: leaf.operator === 'neq' ? 'neq' : 'eq',
        value: leaf.value === false ? 'false' : 'true',
        negate,
      };
    }
    if (leaf.field !== 'count' || !HISTORY_OPS.includes(leaf.operator)) return null;
    return {
      ...emptyCondition('history'),
      field: 'count',
      operator: leaf.operator,
      value: valueToString(leaf.value),
      kind: Array.isArray(leaf.value) ? 'list' : 'number',
      eventType: leaf.event_type ?? '',
      window: (HISTORY_WINDOWS as string[]).includes(leaf.window ?? '') ? (leaf.window as HistoryWindow) : '7d',
      negate,
    };
  }
  const operator = (OPERATORS.includes(leaf.operator) ? leaf.operator : 'eq') as ConditionOperator;
  return {
    ...emptyCondition(source as ConditionSource),
    field: leaf.field,
    operator,
    value: valueToString(leaf.value),
    kind: kindOf(leaf.value),
    negate,
  };
}

/** A fresh row for a source; history starts as "first time". */
function emptyCondition(source: BuilderSource): ConditionRow {
  const base = { id: newId(), source, negate: false, eventType: '', window: '7d' as HistoryWindow };
  if (source === 'history') return { ...base, field: 'first_time', operator: 'eq', value: 'true', kind: 'boolean' };
  return { ...base, field: '', operator: 'eq', value: '', kind: 'text' };
}

/** Stored conditions -> builder rows, or null when the tree is too complex for the form. */
function parseConditions(c: RuleConditions | undefined): { rows: ConditionRow[]; mode: 'all' | 'any' } | null {
  if (c === null || c === undefined) return { rows: [], mode: 'all' };
  const list: unknown[] = Array.isArray(c) ? c : [c];
  if (list.length === 1 && typeof list[0] === 'object' && list[0] !== null && ('any' in list[0] || 'all' in list[0])) {
    const group = list[0] as { any?: unknown[]; all?: unknown[] };
    const children = group.any ?? group.all ?? [];
    const rows = children.map(leafToRow);
    if (rows.some(r => r === null)) return null;
    return { rows: rows as ConditionRow[], mode: group.any ? 'any' : 'all' };
  }
  const rows = list.map(leafToRow);
  if (rows.some(r => r === null)) return null;
  return { rows: rows as ConditionRow[], mode: 'all' };
}

function actionToRow(a: RuleAction): ActionRow {
  const row = emptyAction(a.type);
  const p = a as Record<string, unknown>;
  for (const key of ['amount', 'description', 'badge_id', 'streak_id', 'activity_key', 'mission_id', 'increment', 'reward_id'] as const) {
    if (p[key] !== undefined && p[key] !== null) row[key] = String(p[key]);
  }
  return row;
}

function rowToAction(r: ActionRow): RuleAction {
  switch (r.type) {
    case 'credit_points':
    case 'grant_xp':
      return r.description.trim()
        ? { type: r.type, amount: Number(r.amount), description: r.description.trim() }
        : { type: r.type, amount: Number(r.amount) };
    case 'award_badge':
      return { type: 'award_badge', badge_id: r.badge_id };
    case 'record_streak':
      return r.streak_id
        ? { type: 'record_streak', streak_id: r.streak_id }
        : { type: 'record_streak', activity_key: r.activity_key.trim() };
    case 'progress_mission':
      return { type: 'progress_mission', mission_id: r.mission_id, increment: Number(r.increment || 1) };
    case 'grant_reward':
      return { type: 'grant_reward', reward_id: r.reward_id };
  }
}

/** Client-side checks; the server compiler stays the source of truth. */
function actionProblem(r: ActionRow): string | null {
  const positiveInt = (s: string) => /^\d+$/.test(s.trim()) && Number(s) > 0;
  switch (r.type) {
    case 'credit_points':
    case 'grant_xp':
      return positiveInt(r.amount) ? null : 'Amount must be a whole number greater than 0.';
    case 'award_badge':
      return r.badge_id ? null : 'Select a badge.';
    case 'record_streak':
      return r.streak_id || r.activity_key.trim() ? null : 'Select a streak or enter an activity key.';
    case 'progress_mission':
      if (!r.mission_id) return 'Select a mission.';
      return positiveInt(r.increment || '1') ? null : 'Increment must be a whole number greater than 0.';
    case 'grant_reward':
      return r.reward_id ? null : 'Select a reward.';
  }
}

function conditionProblem(r: ConditionRow): string | null {
  if (r.source === 'history') {
    if (r.field === 'first_time') return null;
    if (r.eventType.trim() && !EVENT_SLUG_RE.test(r.eventType.trim())) return 'Event type must be an event slug.';
    const items = LIST_OPS.includes(r.operator) ? r.value.split(',').map(v => v.trim()).filter(Boolean) : [r.value.trim()];
    if (items.length === 0 || items.some(v => !v)) return 'Value is required.';
    if (items.some(v => Number.isNaN(Number(v)))) return 'Count must be a number.';
    return null;
  }
  if (!r.field.trim()) return 'Field is required.';
  if (NO_VALUE.includes(r.operator)) return null;
  if (!r.value.trim()) return 'Value is required.';
  if (r.kind === 'number' && !LIST_OPS.includes(r.operator) && Number.isNaN(Number(r.value))) return 'Value must be a number.';
  return null;
}

/** JSON with sorted object keys: the API may return keys in a different order. */
function canonical(value: unknown): string {
  const sort = (v: unknown): unknown => {
    if (Array.isArray(v)) return v.map(sort);
    if (v && typeof v === 'object') {
      return Object.fromEntries(
        Object.entries(v as Record<string, unknown>)
          .filter(([, x]) => x !== undefined)
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([k, x]) => [k, sort(x)]),
      );
    }
    return v;
  };
  return JSON.stringify(sort(value));
}

// ==================== schedule ====================

interface ScheduleForm {
  enabled: boolean;
  /** datetime-local values, in the browser's time zone. */
  startsAt: string;
  endsAt: string;
  days: RuleWeekday[];
  hoursEnabled: boolean;
  from: string;
  to: string;
  timezone: string;
}

const browserTimeZone = (() => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
})();

const TIME_ZONES: string[] = (() => {
  const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] };
  try {
    const zones = intl.supportedValuesOf?.('timeZone') ?? [];
    return zones.includes('UTC') ? zones : ['UTC', ...zones];
  } catch {
    return ['UTC'];
  }
})();

const emptySchedule = (): ScheduleForm => ({
  enabled: false,
  startsAt: '',
  endsAt: '',
  days: [],
  hoursEnabled: false,
  from: '09:00',
  to: '17:00',
  timezone: browserTimeZone,
});

/** RFC 3339 -> "YYYY-MM-DDTHH:mm" in local time (datetime-local). */
function isoToLocalInput(iso: string | null | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function scheduleToForm(s: RuleSchedule | null | undefined): ScheduleForm {
  if (!s) return emptySchedule();
  return {
    enabled: true,
    startsAt: isoToLocalInput(s.starts_at),
    endsAt: isoToLocalInput(s.ends_at),
    days: s.days_of_week ?? [],
    hoursEnabled: !!s.hours,
    from: s.hours?.from ?? '09:00',
    to: s.hours?.to ?? '17:00',
    timezone: s.timezone || 'UTC',
  };
}

/** The form -> the grammar's schedule (null = always), or an error message. */
function formToSchedule(f: ScheduleForm): { schedule: RuleSchedule | null } | { error: string } {
  if (!f.enabled) return { schedule: null };
  const s: RuleSchedule = { timezone: f.timezone.trim() || 'UTC' };
  const starts = f.startsAt ? new Date(f.startsAt) : null;
  const ends = f.endsAt ? new Date(f.endsAt) : null;
  if (starts && Number.isNaN(starts.getTime())) return { error: 'Schedule start is not a valid date.' };
  if (ends && Number.isNaN(ends.getTime())) return { error: 'Schedule end is not a valid date.' };
  if (starts && ends && ends <= starts) return { error: 'Schedule end must be after its start.' };
  if (starts) s.starts_at = starts.toISOString();
  if (ends) s.ends_at = ends.toISOString();
  if (f.days.length > 0 && f.days.length < 7) s.days_of_week = [...f.days].sort((a, b) => a - b);
  if (f.hoursEnabled) {
    if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(f.from) || !/^([01]\d|2[0-3]):[0-5]\d$/.test(f.to)) {
      return { error: 'Schedule hours must be times HH:MM.' };
    }
    if (f.from === f.to) return { error: 'Schedule hours: "from" and "to" must differ.' };
    s.hours = { from: f.from, to: f.to };
  }
  return { schedule: s };
}

// ==================== catalogue selects ====================

interface Named {
  id: string;
  name: string;
}

function useCatalogue(key: string, path: string) {
  const api = useApi();
  return useQuery({
    queryKey: ['rule-builder', 'catalogue', key],
    queryFn: async () =>
      unwrap(
        await fetchAllPages(cursor => api.get<CursorPage<Named>>(`${path}${toQuery({ limit: 100, cursor })}`)),
        `Failed to load ${key}`,
      ).data,
    staleTime: 60_000,
  });
}

function CatalogueSelect({
  items,
  value,
  onChange,
  placeholder,
  invalid,
}: {
  items: Named[] | undefined;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  invalid?: boolean;
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className={cn(invalid && 'border-destructive')}>
        <SelectValue placeholder={items ? placeholder : 'Loading…'} />
      </SelectTrigger>
      <SelectContent>
        {(items ?? []).length === 0 ? (
          <div className="py-3 px-2 text-sm text-muted-foreground">Nothing to select yet.</div>
        ) : (
          items!.map(i => (
            <SelectItem key={i.id} value={i.id}>
              {i.name}
            </SelectItem>
          ))
        )}
      </SelectContent>
    </Select>
  );
}

// ==================== history condition fields ====================

function HistoryConditionFields({
  row,
  eventSlugs,
  onChange,
}: {
  row: ConditionRow;
  eventSlugs: string[];
  onChange: (patch: Partial<ConditionRow>) => void;
}) {
  const listId = `history-events-${row.id}`;
  return (
    <>
      <Select
        value={row.field}
        onValueChange={v =>
          onChange(
            v === 'first_time'
              ? { field: 'first_time', operator: 'eq', value: 'true', kind: 'boolean' }
              : { field: 'count', operator: 'gte', value: '', kind: 'number' },
          )
        }
      >
        <SelectTrigger className="w-36">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="first_time">First time</SelectItem>
          <SelectItem value="count">Activity count</SelectItem>
        </SelectContent>
      </Select>
      {row.field === 'first_time' ? (
        <Select value={row.value === 'false' ? 'false' : 'true'} onValueChange={v => onChange({ value: v, operator: 'eq' })}>
          <SelectTrigger className="w-56">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="true">is the player's first of this event</SelectItem>
            <SelectItem value="false">is not the player's first</SelectItem>
          </SelectContent>
        </Select>
      ) : (
        <>
          <Input
            list={listId}
            placeholder="event (blank = trigger)"
            value={row.eventType}
            onChange={e => onChange({ eventType: e.target.value })}
            className="w-44 font-mono text-xs"
          />
          <datalist id={listId}>
            {eventSlugs.map(slug => (
              <option key={slug} value={slug} />
            ))}
          </datalist>
          <Select value={row.window} onValueChange={v => onChange({ window: v as HistoryWindow })}>
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {HISTORY_WINDOWS.map(w => (
                <SelectItem key={w} value={w}>
                  {windowLabels[w]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={row.operator}
            onValueChange={v =>
              onChange({ operator: v as ConditionOperator, kind: LIST_OPS.includes(v as ConditionOperator) ? 'list' : 'number' })
            }
          >
            <SelectTrigger className="w-24">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {HISTORY_OPS.map(op => (
                <SelectItem key={op} value={op}>
                  {operatorLabels[op]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            placeholder={row.kind === 'list' ? '1, 2, 3' : 'count'}
            type={row.kind === 'list' ? 'text' : 'number'}
            min={0}
            step={1}
            value={row.value}
            onChange={e => onChange({ value: e.target.value })}
            className="w-28"
          />
        </>
      )}
    </>
  );
}

// ==================== schedule card ====================

function ScheduleCard({
  value,
  onChange,
  errors,
}: {
  value: ScheduleForm;
  onChange: (next: ScheduleForm) => void;
  errors: string[];
}) {
  const set = (patch: Partial<ScheduleForm>) => onChange({ ...value, ...patch });
  const toggleDay = (day: RuleWeekday) =>
    set({ days: value.days.includes(day) ? value.days.filter(d => d !== day) : [...value.days, day] });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-2">
            <CalendarClock className="w-5 h-5 text-sky-500" />
            Schedule
          </span>
          <label className="flex items-center gap-2 text-sm font-normal text-muted-foreground">
            {value.enabled ? 'Only at these times' : 'Always'}
            <Switch checked={value.enabled} onCheckedChange={enabled => set({ enabled })} />
          </label>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {!value.enabled ? (
          <p className="text-sm text-muted-foreground">
            The rule can fire at any time. Turn the schedule on to limit it to a date range, days of the week or hours.
          </p>
        ) : (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1">
                <label className="text-sm font-medium">Starts (optional)</label>
                <Input type="datetime-local" value={value.startsAt} onChange={e => set({ startsAt: e.target.value })} />
              </div>
              <div className="space-y-1">
                <label className="text-sm font-medium">Ends (optional, exclusive)</label>
                <Input type="datetime-local" value={value.endsAt} onChange={e => set({ endsAt: e.target.value })} />
              </div>
            </div>
            <p className="text-xs text-muted-foreground -mt-2">Start and end are entered in your browser's time zone.</p>

            <div className="space-y-2">
              <label className="text-sm font-medium">Days of the week</label>
              <div className="flex flex-wrap gap-2">
                {WEEKDAYS.map(({ day, label }) => {
                  const on = value.days.includes(day);
                  return (
                    <Button
                      key={day}
                      type="button"
                      size="sm"
                      variant={on ? 'default' : 'outline'}
                      aria-pressed={on}
                      onClick={() => toggleDay(day)}
                    >
                      {label}
                    </Button>
                  );
                })}
              </div>
              <p className="text-xs text-muted-foreground">None selected = every day.</p>
            </div>

            <div className="space-y-2">
              <label className="flex items-center gap-2 text-sm font-medium">
                <Checkbox checked={value.hoursEnabled} onCheckedChange={c => set({ hoursEnabled: c === true })} />
                Only between these hours
              </label>
              {value.hoursEnabled && (
                <div className="flex flex-wrap items-center gap-2">
                  <Input type="time" value={value.from} onChange={e => set({ from: e.target.value })} className="w-32" />
                  <span className="text-sm text-muted-foreground">to</span>
                  <Input type="time" value={value.to} onChange={e => set({ to: e.target.value })} className="w-32" />
                  <span className="text-xs text-muted-foreground">
                    "to" is exclusive; a window like 22:00 to 02:00 wraps midnight.
                  </span>
                </div>
              )}
            </div>

            <div className="space-y-1">
              <label className="text-sm font-medium">Time zone</label>
              <Input
                list="rule-schedule-timezones"
                value={value.timezone}
                onChange={e => set({ timezone: e.target.value })}
                placeholder="UTC"
                className="font-mono text-xs"
              />
              <datalist id="rule-schedule-timezones">
                {TIME_ZONES.map(tz => (
                  <option key={tz} value={tz} />
                ))}
              </datalist>
              <p className="text-xs text-muted-foreground">Days and hours are judged in this IANA time zone.</p>
            </div>
          </>
        )}
        {errors.map(e => (
          <p key={e} className="text-xs text-destructive">
            {e}
          </p>
        ))}
      </CardContent>
    </Card>
  );
}

// ==================== page ====================

const NO_PROGRAM = 'none';

export default function RuleBuilder() {
  const navigate = useNavigate();
  const { toast } = useToast();
  const params = useParams<{ ruleId?: string }>();
  const [searchParams] = useSearchParams();
  const editId = params.ruleId ?? searchParams.get('edit') ?? undefined;
  const isEditing = !!editId;

  const [ruleName, setRuleName] = useState('');
  const [description, setDescription] = useState('');
  const [triggerEvent, setTriggerEvent] = useState('');
  const [priority, setPriority] = useState('0');
  const [programId, setProgramId] = useState(NO_PROGRAM);
  const [matchMode, setMatchMode] = useState<'all' | 'any'>('all');
  const [conditions, setConditions] = useState<ConditionRow[]>([]);
  const [jsonMode, setJsonMode] = useState(false);
  const [conditionsJson, setConditionsJson] = useState('[]');
  const [actions, setActions] = useState<ActionRow[]>([]);
  const [limits, setLimits] = useState<Record<LimitKey, string>>({
    max_per_player: '',
    max_per_player_per_day: '',
    max_per_player_per_week: '',
    cooldown_seconds: '',
  });
  const [schedule, setSchedule] = useState<ScheduleForm>(emptySchedule);
  const [stopProcessing, setStopProcessing] = useState(false);
  const [serverErrors, setServerErrors] = useState<Record<string, string[]>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [showValidation, setShowValidation] = useState(false);
  const [loadedRuleId, setLoadedRuleId] = useState<string | null>(null);

  const { data: events = [], isLoading: eventsLoading } = useEventsQuery({ active: true });
  const { data: badges } = useCatalogue('badges', BADGE_ENDPOINTS.LIST);
  const { data: missions } = useCatalogue('missions', MISSION_ENDPOINTS.LIST);
  const { data: rewards } = useCatalogue('rewards', REWARD_ENDPOINTS.LIST);
  const { data: streaks } = useCatalogue('streaks', STREAK_ENDPOINTS.LIST);
  const { data: programs } = useCatalogue('programs', PROGRAM_ENDPOINTS.LIST);

  const { data: rule, isLoading: ruleLoading, error: ruleError } = useRuleQuery(editId);
  const { data: versionsPage } = useRuleVersionsQuery(editId, { limit: 20 });
  const createRule = useCreateRuleMutation();
  const updateRule = useUpdateRuleMutation();
  const createVersion = useCreateRuleVersionMutation();
  const publishRule = usePublishRuleMutation();
  const saving = createRule.isPending || updateRule.isPending || createVersion.isPending || publishRule.isPending;

  // Load the rule being edited once (from its latest version, draft or not).
  useEffect(() => {
    if (!rule || loadedRuleId === rule.id) return;
    setLoadedRuleId(rule.id);
    setRuleName(rule.name);
    setDescription(rule.description ?? '');
    setTriggerEvent(rule.trigger_event);
    setPriority(String(rule.priority));
    setProgramId(rule.program_id ?? NO_PROGRAM);
    const version = rule.latest_version ?? rule.current_version;
    const parsed = parseConditions(version?.conditions);
    if (parsed) {
      setConditions(parsed.rows);
      setMatchMode(parsed.mode);
      setJsonMode(false);
    } else {
      setConditionsJson(JSON.stringify(version?.conditions ?? [], null, 2));
      setJsonMode(true);
    }
    setActions((version?.actions ?? []).map(actionToRow));
    const l = version?.limits ?? {};
    setLimits({
      max_per_player: l.max_per_player ? String(l.max_per_player) : '',
      max_per_player_per_day: l.max_per_player_per_day ? String(l.max_per_player_per_day) : '',
      max_per_player_per_week: l.max_per_player_per_week ? String(l.max_per_player_per_week) : '',
      cooldown_seconds: l.cooldown_seconds ? String(l.cooldown_seconds) : '',
    });
    setSchedule(scheduleToForm(version?.schedule));
    setStopProcessing(!!version?.stop_processing);
  }, [rule, loadedRuleId]);

  const selectedEvent = useMemo(() => events.find(e => e.slug === triggerEvent), [events, triggerEvent]);
  const eventProperties = useMemo(
    () => Object.entries(selectedEvent?.property_schema?.properties ?? {}).map(([name, def]) => ({ name, type: def?.type ?? 'any' })),
    [selectedEvent],
  );

  const fieldSuggestions = (source: BuilderSource) =>
    source === 'trigger'
      ? eventProperties.map(p => p.name)
      : source === 'player'
        ? PLAYER_FIELDS
        : source === 'activity'
          ? ACTIVITY_FIELDS
          : [];

  // ----- conditions -----
  /** A new row with sensible defaults for its source. */
  const conditionFor = (source: BuilderSource): ConditionRow => {
    if (source === 'history') return emptyCondition('history');
    const firstProp = eventProperties[0];
    return {
      ...emptyCondition(source),
      field: source === 'trigger' ? (firstProp?.name ?? '') : source === 'player' ? 'level' : 'event_type',
      operator: source === 'trigger' && firstProp?.type === 'number' ? 'gte' : 'eq',
      kind: source === 'player' || firstProp?.type === 'number' || firstProp?.type === 'integer' ? 'number' : 'text',
    };
  };
  const addCondition = (source: BuilderSource) => setConditions(rows => [...rows, conditionFor(source)]);
  const changeSource = (id: string, source: BuilderSource) =>
    setConditions(rows =>
      rows.map(r =>
        r.id !== id
          ? r
          : source === 'history' || r.source === 'history'
            ? { ...conditionFor(source), id: r.id, negate: r.negate }
            : { ...r, source, field: '' },
      ),
    );
  const updateCondition = (id: string, patch: Partial<ConditionRow>) =>
    setConditions(rows => rows.map(r => (r.id === id ? { ...r, ...patch } : r)));
  const removeCondition = (id: string) => setConditions(rows => rows.filter(r => r.id !== id));

  const toggleJsonMode = (on: boolean) => {
    if (on) {
      setConditionsJson(JSON.stringify(buildConditions(conditions, matchMode), null, 2));
      setJsonMode(true);
      return;
    }
    try {
      const parsed = parseConditions(JSON.parse(conditionsJson));
      if (!parsed) {
        toast({
          title: 'Too complex for the form',
          description: 'These conditions use nested groups; keep editing them as JSON.',
          variant: 'destructive',
        });
        return;
      }
      setConditions(parsed.rows);
      setMatchMode(parsed.mode);
      setJsonMode(false);
    } catch {
      toast({ title: 'Invalid JSON', description: 'Fix the conditions JSON first.', variant: 'destructive' });
    }
  };

  // ----- actions -----
  const addAction = (type: RuleActionType) => setActions(rows => [...rows, emptyAction(type)]);
  const updateAction = (id: string, patch: Partial<ActionRow>) =>
    setActions(rows => rows.map(r => (r.id === id ? { ...r, ...patch } : r)));
  const removeAction = (id: string) => setActions(rows => rows.filter(r => r.id !== id));

  const errorsAt = (prefix: string) =>
    Object.entries(serverErrors)
      .filter(([k]) => k === prefix || k.startsWith(`${prefix}.`) || k.startsWith(`${prefix}[`))
      .flatMap(([k, msgs]) => msgs.map(m => `${k.slice(prefix.length).replace(/^\./, '') || prefix}: ${m}`));

  // ----- build + save -----
  const buildDefinition = (): {
    conditions: RuleConditions;
    actions: RuleAction[];
    limits: RuleLimits;
    schedule: RuleSchedule | null;
    stop_processing: boolean;
  } | null => {
    let conds: RuleConditions;
    if (jsonMode) {
      try {
        conds = conditionsJson.trim() ? JSON.parse(conditionsJson) : [];
      } catch {
        setFormError('Conditions JSON is not valid JSON.');
        return null;
      }
    } else {
      if (conditions.some(c => conditionProblem(c))) {
        setFormError('Some conditions are incomplete.');
        return null;
      }
      conds = buildConditions(conditions, matchMode);
    }
    if (actions.length === 0) {
      setFormError('Add at least one action.');
      return null;
    }
    if (actions.some(a => actionProblem(a))) {
      setFormError('Some actions are incomplete.');
      return null;
    }
    const lim: RuleLimits = {};
    for (const key of LIMIT_KEYS) {
      const v = limits[key].trim();
      if (!v) continue;
      if (!/^\d+$/.test(v) || Number(v) <= 0) {
        setFormError(`${limitLabels[key]} must be a whole number greater than 0.`);
        return null;
      }
      lim[key] = Number(v);
    }
    const sched = formToSchedule(schedule);
    if ('error' in sched) {
      setFormError(sched.error);
      return null;
    }
    return {
      conditions: conds,
      actions: actions.map(rowToAction),
      limits: lim,
      schedule: sched.schedule,
      stop_processing: stopProcessing,
    };
  };

  /** The unsaved form as a draft for POST /rules/simulate (null + a form error when incomplete). */
  const buildDraft = (): DraftRuleDefinition | null => {
    setFormError(null);
    setServerErrors({});
    setShowValidation(true);
    if (!triggerEvent) {
      setFormError('Select a trigger event to simulate the rule.');
      return null;
    }
    const def = buildDefinition();
    if (!def) return null;
    return {
      trigger_event: triggerEvent,
      conditions: def.conditions,
      actions: def.actions,
      limits: Object.keys(def.limits).length ? def.limits : undefined,
      schedule: def.schedule,
      stop_processing: def.stop_processing,
    };
  };

  const handleError = (err: unknown) => {
    if (err instanceof ApiRequestError) {
      setServerErrors(err.validationErrors ?? {});
      if (err.code === 'invalid_rule_definition') {
        setFormError('The rule definition is invalid. Fix the highlighted fields.');
      } else if (err.code === 'no_draft_version') {
        setFormError('The latest version is published and cannot be edited; save again to create a new draft version.');
      } else {
        setFormError(err.message);
      }
    } else {
      setFormError(err instanceof Error ? err.message : 'Failed to save the rule');
    }
  };

  const handleSave = async (publish: boolean) => {
    setShowValidation(true);
    setFormError(null);
    setServerErrors({});
    if (!ruleName.trim() || !triggerEvent) {
      setFormError('Please provide a rule name and a trigger event.');
      return;
    }
    const prio = Number(priority || 0);
    if (!Number.isInteger(prio) || prio < 0) {
      setFormError('Priority must be a whole number of 0 or more.');
      return;
    }
    const def = buildDefinition();
    if (!def) return;
    const limitsOrUndefined = Object.keys(def.limits).length ? def.limits : undefined;

    try {
      let saved: RuleV2;
      if (!isEditing || !rule) {
        saved = await createRule.mutateAsync({
          name: ruleName.trim(),
          description: description.trim() || undefined,
          trigger_event: triggerEvent,
          program_id: programId === NO_PROGRAM ? undefined : programId,
          priority: prio,
          conditions: def.conditions,
          actions: def.actions,
          limits: limitsOrUndefined,
          schedule: def.schedule ?? undefined,
          stop_processing: def.stop_processing || undefined,
        });
      } else {
        const patch: UpdateRuleDataV2 = {};
        if (ruleName.trim() !== rule.name) patch.name = ruleName.trim();
        if (description.trim() !== (rule.description ?? '')) patch.description = description.trim() || null;
        if (triggerEvent !== rule.trigger_event) patch.trigger_event = triggerEvent;
        const nextProgram = programId === NO_PROGRAM ? null : programId;
        if (nextProgram !== rule.program_id) patch.program_id = nextProgram;
        if (prio !== rule.priority) patch.priority = prio;

        const latest = rule.latest_version;
        const definitionChanged =
          !latest ||
          canonical(def.conditions ?? []) !== canonical(latest.conditions ?? []) ||
          canonical(def.actions) !== canonical(latest.actions) ||
          canonical(def.limits) !== canonical(latest.limits ?? {}) ||
          canonical(def.schedule) !== canonical(latest.schedule ?? null) ||
          def.stop_processing !== !!latest.stop_processing;

        if (definitionChanged && latest && !latest.published) {
          // The latest version is still a draft: edit it in place.
          patch.conditions = def.conditions;
          patch.actions = def.actions;
          patch.limits = limitsOrUndefined ?? null;
          patch.schedule = def.schedule;
          patch.stop_processing = def.stop_processing;
        }
        saved = rule;
        if (Object.keys(patch).length > 0) {
          saved = await updateRule.mutateAsync({ ruleId: rule.id, data: patch });
        }
        if (definitionChanged && (!latest || latest.published)) {
          // Published versions are immutable: create a new draft version.
          const version = await createVersion.mutateAsync({
            ruleId: rule.id,
            data: {
              conditions: def.conditions,
              actions: def.actions,
              // Omitted parts are copied from the latest version: send null to clear.
              limits: limitsOrUndefined ?? null,
              schedule: def.schedule,
              stop_processing: def.stop_processing,
            },
          });
          saved = { ...saved, latest_version: version };
        }
      }

      if (publish) {
        const version = saved.latest_version?.version;
        if (!version) throw new Error('The rule has no version to publish.');
        await publishRule.mutateAsync({ ruleId: saved.id, version });
      }
      toast({
        title: publish ? 'Rule published' : 'Rule saved as draft',
        description: `"${ruleName.trim()}" has been ${publish ? 'published and is live' : 'saved'}.`,
      });
      navigate('/rules');
    } catch (err) {
      handleError(err);
    }
  };

  const publishVersion = async (version: number) => {
    if (!rule) return;
    try {
      await publishRule.mutateAsync({ ruleId: rule.id, version });
      toast({ title: `Version ${version} published` });
    } catch (err) {
      handleError(err);
    }
  };

  if (isEditing && ruleLoading) {
    return (
      <div className="flex items-center justify-center py-24 text-muted-foreground">
        <Loader2 className="w-6 h-6 animate-spin mr-2" /> Loading rule…
      </div>
    );
  }
  if (isEditing && ruleError) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" onClick={() => navigate('/rules')}>
          <ArrowLeft className="w-4 h-4" /> Back to rules
        </Button>
        <p className="text-destructive">{ruleError.message}</p>
      </div>
    );
  }

  const generalErrors = Object.entries(serverErrors).filter(
    ([k]) =>
      !k.startsWith('conditions') && !k.startsWith('actions') && !k.startsWith('limits') && !k.startsWith('schedule'),
  );

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/rules')}>
          <ArrowLeft className="w-5 h-5" />
        </Button>
        <div className="flex-1">
          <h1 className="text-3xl font-bold">{isEditing ? `Edit "${rule?.name ?? ''}"` : 'Create New Rule'}</h1>
          <p className="text-muted-foreground mt-1">
            When the trigger event arrives and the conditions match, the actions run for the player.
          </p>
        </div>
        {rule && (
          <Badge variant="outline" className="capitalize">
            {rule.status}
          </Badge>
        )}
      </div>

      {(formError || generalErrors.length > 0) && (
        <div className="flex items-start gap-2 p-4 rounded-lg border border-destructive/40 bg-destructive/10 text-destructive">
          <AlertCircle className="w-5 h-5 shrink-0 mt-0.5" />
          <div className="text-sm space-y-1">
            {formError && <p className="font-medium">{formError}</p>}
            {generalErrors.map(([k, msgs]) => (
              <p key={k}>
                {k}: {msgs.join(', ')}
              </p>
            ))}
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 space-y-6">
          {/* Trigger */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Zap className="w-5 h-5 text-primary" />
                Trigger Event
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <label className="text-sm font-medium">When this event occurs:</label>
                  <Link to="/events" className="text-xs text-primary hover:underline flex items-center gap-1">
                    Manage Events <ExternalLink className="w-3 h-3" />
                  </Link>
                </div>
                <Select value={triggerEvent} onValueChange={setTriggerEvent}>
                  <SelectTrigger className={cn(showValidation && !triggerEvent && 'border-destructive')}>
                    <SelectValue placeholder={eventsLoading ? 'Loading events...' : 'Select an event type'} />
                  </SelectTrigger>
                  <SelectContent>
                    {events.length === 0 ? (
                      <div className="py-4 px-2 text-center text-sm text-muted-foreground">No active event types</div>
                    ) : (
                      events.map(event => (
                        <SelectItem key={event.id} value={event.slug}>
                          <div className="flex items-center gap-2">
                            <span>{event.name}</span>
                            <code className="text-[10px] text-muted-foreground">{event.slug}</code>
                          </div>
                        </SelectItem>
                      ))
                    )}
                    {triggerEvent && !events.some(e => e.slug === triggerEvent) && (
                      <SelectItem value={triggerEvent}>{triggerEvent} (inactive or unknown)</SelectItem>
                    )}
                  </SelectContent>
                </Select>
              </div>

              {selectedEvent && (
                <div className="p-4 bg-primary/5 border border-primary/20 rounded-lg space-y-2">
                  <div className="flex items-center gap-2">
                    <h4 className="font-medium">{selectedEvent.name}</h4>
                    <code className="text-xs bg-secondary px-1.5 py-0.5 rounded">{selectedEvent.slug}</code>
                  </div>
                  {selectedEvent.description && <p className="text-sm text-muted-foreground">{selectedEvent.description}</p>}
                  {eventProperties.length > 0 ? (
                    <div className="flex flex-wrap gap-2 pt-1">
                      {eventProperties.map(p => (
                        <Badge key={p.name} variant="outline" className="font-mono text-xs">
                          {p.name}: {p.type}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      No property schema declared; you can still reference any property by name.
                    </p>
                  )}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Conditions */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center justify-between gap-2">
                <span className="flex items-center gap-2">
                  <GitBranch className="w-5 h-5 text-blue-500" />
                  Conditions
                </span>
                <label className="flex items-center gap-2 text-sm font-normal text-muted-foreground">
                  <Braces className="w-4 h-4" />
                  JSON
                  <Switch checked={jsonMode} onCheckedChange={toggleJsonMode} />
                </label>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {jsonMode ? (
                <div className="space-y-2">
                  <Textarea
                    value={conditionsJson}
                    onChange={e => setConditionsJson(e.target.value)}
                    rows={10}
                    className="font-mono text-xs"
                  />
                  <p className="text-xs text-muted-foreground">
                    Grammar: a list (AND) of nodes; a node is {'{"all": [...]}'}, {'{"any": [...]}'}, {'{"not": node}'} or{' '}
                    {'{"source", "field", "operator", "value"}'}. History leaves:{' '}
                    {'{"source": "history", "field": "first_time"}'} or{' '}
                    {'{"source": "history", "field": "count", "event_type"?, "window": "1d|7d|30d|90d|all", "operator", "value"}'}.
                  </p>
                  {errorsAt('conditions').map(e => (
                    <p key={e} className="text-xs text-destructive">
                      {e}
                    </p>
                  ))}
                </div>
              ) : (
                <>
                  {conditions.length > 1 && (
                    <div className="flex items-center gap-2 text-sm">
                      <span>Match</span>
                      <Select value={matchMode} onValueChange={v => setMatchMode(v as 'all' | 'any')}>
                        <SelectTrigger className="w-28 h-8">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="all">all</SelectItem>
                          <SelectItem value="any">any</SelectItem>
                        </SelectContent>
                      </Select>
                      <span>of these conditions</span>
                    </div>
                  )}
                  {conditions.length === 0 && (
                    <p className="text-sm text-muted-foreground text-center py-4">
                      No conditions: the rule runs on every activity of the trigger event.
                    </p>
                  )}
                  {conditions.map((row, index) => {
                    const problems = errorsAt(rowPath(index, matchMode, row.negate));
                    const localProblem = showValidation ? conditionProblem(row) : null;
                    const listId = `fields-${row.id}`;
                    return (
                      <div
                        key={row.id}
                        className={cn(
                          'p-3 bg-secondary/50 rounded-lg space-y-2',
                          (problems.length > 0 || localProblem) && 'ring-1 ring-destructive',
                        )}
                      >
                        <div className="flex flex-wrap items-center gap-2">
                          <Select value={row.source} onValueChange={v => changeSource(row.id, v as BuilderSource)}>
                            <SelectTrigger className="w-36">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {(Object.keys(sourceLabels) as BuilderSource[]).map(s => (
                                <SelectItem key={s} value={s}>
                                  {sourceLabels[s]}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                          {row.source === 'history' ? (
                            <HistoryConditionFields
                              row={row}
                              eventSlugs={events.map(e => e.slug)}
                              onChange={patch => updateCondition(row.id, patch)}
                            />
                          ) : (
                            <>
                              <Input
                                list={listId}
                                placeholder={row.source === 'trigger' ? 'property (dot.path)' : 'field'}
                                value={row.field}
                                onChange={e => updateCondition(row.id, { field: e.target.value })}
                                className="w-40 font-mono text-xs"
                              />
                              <datalist id={listId}>
                                {fieldSuggestions(row.source).map(f => (
                                  <option key={f} value={f} />
                                ))}
                              </datalist>
                              <Select
                                value={row.operator}
                                onValueChange={v =>
                                  updateCondition(row.id, {
                                    operator: v as ConditionOperator,
                                    kind: LIST_OPS.includes(v as ConditionOperator)
                                      ? 'list'
                                      : row.kind === 'list'
                                        ? 'text'
                                        : row.kind,
                                  })
                                }
                              >
                                <SelectTrigger className="w-32">
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                  {OPERATORS.map(op => (
                                    <SelectItem key={op} value={op}>
                                      {operatorLabels[op]}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                              {!NO_VALUE.includes(row.operator) && (
                                <>
                                  {row.kind === 'boolean' ? (
                                    <Select value={row.value || 'true'} onValueChange={v => updateCondition(row.id, { value: v })}>
                                      <SelectTrigger className="w-28">
                                        <SelectValue />
                                      </SelectTrigger>
                                      <SelectContent>
                                        <SelectItem value="true">true</SelectItem>
                                        <SelectItem value="false">false</SelectItem>
                                      </SelectContent>
                                    </Select>
                                  ) : (
                                    <Input
                                      placeholder={row.kind === 'list' ? 'a, b, c' : 'value'}
                                      type={row.kind === 'number' ? 'number' : 'text'}
                                      value={row.value}
                                      onChange={e => updateCondition(row.id, { value: e.target.value })}
                                      className="flex-1 min-w-[100px]"
                                    />
                                  )}
                                  {!LIST_OPS.includes(row.operator) && (
                                    <Select
                                      value={row.kind}
                                      onValueChange={v =>
                                        updateCondition(row.id, {
                                          kind: v as ValueKind,
                                          value: v === 'boolean' ? 'true' : row.value,
                                        })
                                      }
                                    >
                                      <SelectTrigger className="w-28">
                                        <SelectValue />
                                      </SelectTrigger>
                                      <SelectContent>
                                        <SelectItem value="text">text</SelectItem>
                                        <SelectItem value="number">number</SelectItem>
                                        <SelectItem value="boolean">boolean</SelectItem>
                                      </SelectContent>
                                    </Select>
                                  )}
                                </>
                              )}
                            </>
                          )}
                          <label className="flex items-center gap-1 text-xs text-muted-foreground">
                            <Checkbox
                              checked={row.negate}
                              onCheckedChange={c => updateCondition(row.id, { negate: c === true })}
                            />
                            NOT
                          </label>
                          <Button variant="ghost" size="icon" className="h-8 w-8 ml-auto" onClick={() => removeCondition(row.id)}>
                            <Trash2 className="w-4 h-4 text-destructive" />
                          </Button>
                        </div>
                        {localProblem && <p className="text-xs text-destructive">{localProblem}</p>}
                        {problems.map(p => (
                          <p key={p} className="text-xs text-destructive">
                            {p}
                          </p>
                        ))}
                      </div>
                    );
                  })}
                  <div className="flex flex-wrap gap-2 pt-2">
                    <Button variant="outline" size="sm" onClick={() => addCondition('trigger')}>
                      <Plus className="w-4 h-4 mr-1" />
                      Event property
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => addCondition('player')}>
                      <Plus className="w-4 h-4 mr-1" />
                      Player attribute
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => addCondition('activity')}>
                      <Plus className="w-4 h-4 mr-1" />
                      Activity field
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => addCondition('history')}>
                      <Plus className="w-4 h-4 mr-1" />
                      Player history
                    </Button>
                  </div>
                </>
              )}
            </CardContent>
          </Card>

          {/* Actions */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Award className="w-5 h-5 text-green-500" />
                Actions
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {actions.length === 0 && (
                <p
                  className={cn(
                    'text-sm text-center py-4',
                    showValidation ? 'text-destructive' : 'text-muted-foreground',
                  )}
                >
                  Add at least one action to run when the conditions match.
                </p>
              )}
              {actions.map((action, index) => {
                const problems = errorsAt(`actions[${index}]`);
                const localProblem = showValidation ? actionProblem(action) : null;
                return (
                  <div
                    key={action.id}
                    className={cn(
                      'p-4 bg-secondary/50 rounded-lg flex items-start justify-between gap-4',
                      (problems.length > 0 || localProblem) && 'ring-1 ring-destructive',
                    )}
                  >
                    <div className="flex-1 space-y-2">
                      <Badge variant="outline">{actionLabels[action.type]}</Badge>
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                        {(action.type === 'credit_points' || action.type === 'grant_xp') && (
                          <>
                            <Input
                              placeholder="Amount"
                              type="number"
                              min={1}
                              step={1}
                              value={action.amount}
                              onChange={e => updateAction(action.id, { amount: e.target.value })}
                            />
                            <Input
                              placeholder="Description (optional)"
                              value={action.description}
                              maxLength={255}
                              onChange={e => updateAction(action.id, { description: e.target.value })}
                            />
                          </>
                        )}
                        {action.type === 'award_badge' && (
                          <CatalogueSelect
                            items={badges}
                            value={action.badge_id}
                            onChange={v => updateAction(action.id, { badge_id: v })}
                            placeholder="Select badge"
                          />
                        )}
                        {action.type === 'grant_reward' && (
                          <CatalogueSelect
                            items={rewards}
                            value={action.reward_id}
                            onChange={v => updateAction(action.id, { reward_id: v })}
                            placeholder="Select reward"
                          />
                        )}
                        {action.type === 'progress_mission' && (
                          <>
                            <CatalogueSelect
                              items={missions}
                              value={action.mission_id}
                              onChange={v => updateAction(action.id, { mission_id: v })}
                              placeholder="Select mission"
                            />
                            <Input
                              placeholder="Increment"
                              type="number"
                              min={1}
                              step={1}
                              value={action.increment}
                              onChange={e => updateAction(action.id, { increment: e.target.value })}
                            />
                          </>
                        )}
                        {action.type === 'record_streak' && (
                          <>
                            <CatalogueSelect
                              items={streaks}
                              value={action.streak_id}
                              onChange={v => updateAction(action.id, { streak_id: v, activity_key: '' })}
                              placeholder="Select streak"
                            />
                            <Input
                              placeholder="…or activity key"
                              value={action.activity_key}
                              disabled={!!action.streak_id}
                              onChange={e => updateAction(action.id, { activity_key: e.target.value })}
                            />
                          </>
                        )}
                      </div>
                      {localProblem && <p className="text-xs text-destructive">{localProblem}</p>}
                      {problems.map(p => (
                        <p key={p} className="text-xs text-destructive">
                          {p}
                        </p>
                      ))}
                    </div>
                    <Button variant="ghost" size="icon" onClick={() => removeAction(action.id)}>
                      <Trash2 className="w-4 h-4 text-destructive" />
                    </Button>
                  </div>
                );
              })}
              <div className="flex flex-wrap gap-2 pt-2">
                {ACTION_TYPES.map(type => (
                  <Button key={type} variant="outline" size="sm" onClick={() => addAction(type)}>
                    <Plus className="w-4 h-4 mr-1" />
                    {actionLabels[type]}
                  </Button>
                ))}
              </div>
            </CardContent>
          </Card>

          {/* Limits */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Gauge className="w-5 h-5 text-amber-500" />
                Limits
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {LIMIT_KEYS.map(key => (
                  <div key={key} className="space-y-1">
                    <label className="text-sm font-medium">{limitLabels[key]}</label>
                    <Input
                      type="number"
                      min={1}
                      step={1}
                      placeholder="Unlimited"
                      value={limits[key]}
                      onChange={e => setLimits(l => ({ ...l, [key]: e.target.value }))}
                    />
                    {errorsAt(`limits.${key}`).map(p => (
                      <p key={p} className="text-xs text-destructive">
                        {p}
                      </p>
                    ))}
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          {/* Schedule */}
          <ScheduleCard value={schedule} onChange={setSchedule} errors={errorsAt('schedule')} />
        </div>

        {/* Sidebar */}
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Rule Settings</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Rule Name *</label>
                <Input
                  placeholder="e.g., First Purchase Bonus"
                  value={ruleName}
                  maxLength={255}
                  onChange={e => setRuleName(e.target.value)}
                  className={cn(showValidation && !ruleName.trim() && 'border-destructive')}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Description</label>
                <Textarea
                  placeholder="Describe what this rule does..."
                  value={description}
                  maxLength={1000}
                  onChange={e => setDescription(e.target.value)}
                  rows={3}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Program</label>
                <Select value={programId} onValueChange={setProgramId}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_PROGRAM}>All players (no program)</SelectItem>
                    {(programs ?? []).map(p => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">A program-scoped rule only runs for enrolled players.</p>
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Priority</label>
                <Input type="number" min={0} step={1} value={priority} onChange={e => setPriority(e.target.value)} />
                <p className="text-xs text-muted-foreground">Higher priority rules are evaluated first.</p>
              </div>
              <div className="flex items-start justify-between gap-3 rounded-lg border p-3">
                <div className="space-y-1">
                  <label htmlFor="stop-processing" className="text-sm font-medium flex items-center gap-1.5">
                    <OctagonX className="w-4 h-4 text-destructive" />
                    Stop processing
                  </label>
                  <p className="text-xs text-muted-foreground">
                    When this rule fires, lower-priority rules are skipped for the same activity. A rule refused by a
                    limit does not stop anything.
                  </p>
                </div>
                <Switch id="stop-processing" checked={stopProcessing} onCheckedChange={setStopProcessing} />
              </div>
            </CardContent>
          </Card>

          {isEditing && (versionsPage?.data.length ?? 0) > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <History className="w-5 h-5" />
                  Versions
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                {versionsPage!.data.map(v => {
                  const isCurrent = v.id === rule?.current_version_id;
                  return (
                    <div key={v.id} className="flex items-center justify-between gap-2 text-sm">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">v{v.version}</span>
                        {isCurrent ? (
                          <Badge className="text-[10px]">live</Badge>
                        ) : v.published ? (
                          <Badge variant="secondary" className="text-[10px]">
                            published
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="text-[10px]">
                            draft
                          </Badge>
                        )}
                      </div>
                      {!isCurrent && (
                        <Button variant="ghost" size="sm" disabled={saving} onClick={() => publishVersion(v.version)}>
                          Publish
                        </Button>
                      )}
                    </div>
                  );
                })}
                <p className="text-xs text-muted-foreground pt-2">
                  Saving changes to a published version creates a new draft version.
                </p>
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Target className="w-5 h-5" />
                Simulate
              </CardTitle>
            </CardHeader>
            <CardContent>
              <RuleSimulator
                compact
                defaultEventType={triggerEvent || undefined}
                focusRuleId={editId}
                canSendActivity
                getDraft={buildDraft}
                onDraftErrors={setServerErrors}
              />
            </CardContent>
          </Card>

          <div className="flex flex-col gap-2">
            <Button variant="glow" className="w-full" disabled={saving} onClick={() => handleSave(true)}>
              {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Zap className="w-4 h-4" />}
              {isEditing ? 'Save & Publish' : 'Publish Rule'}
            </Button>
            <Button variant="outline" className="w-full" disabled={saving} onClick={() => handleSave(false)}>
              <Save className="w-4 h-4" />
              Save as Draft
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

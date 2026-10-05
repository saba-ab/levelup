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
  Rule,
  RuleAction,
  RuleActionType,
  RuleConditions,
  RuleLimits,
  UpdateRuleData,
} from '@/services/api/types';
import RuleSimulator from '@/components/RuleSimulator';

// ==================== builder model ====================

type ValueKind = 'text' | 'number' | 'boolean' | 'list';

interface ConditionRow {
  id: string;
  source: ConditionSource;
  field: string;
  operator: ConditionOperator;
  value: string;
  kind: ValueKind;
  negate: boolean;
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

const sourceLabels: Record<ConditionSource, string> = {
  trigger: 'Event property',
  player: 'Player',
  activity: 'Activity',
};

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

function rowToLeaf(row: ConditionRow): ConditionNode {
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
  const leaf = node as ConditionLeaf & { type?: ConditionSource };
  const operator = (OPERATORS.includes(leaf.operator) ? leaf.operator : 'eq') as ConditionOperator;
  return {
    id: newId(),
    source: leaf.source ?? leaf.type ?? 'trigger',
    field: leaf.field,
    operator,
    value: valueToString(leaf.value),
    kind: kindOf(leaf.value),
    negate,
  };
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
  }, [rule, loadedRuleId]);

  const selectedEvent = useMemo(() => events.find(e => e.slug === triggerEvent), [events, triggerEvent]);
  const eventProperties = useMemo(
    () => Object.entries(selectedEvent?.property_schema?.properties ?? {}).map(([name, def]) => ({ name, type: def?.type ?? 'any' })),
    [selectedEvent],
  );

  const fieldSuggestions = (source: ConditionSource) =>
    source === 'trigger' ? eventProperties.map(p => p.name) : source === 'player' ? PLAYER_FIELDS : ACTIVITY_FIELDS;

  // ----- conditions -----
  const addCondition = (source: ConditionSource) => {
    const firstProp = eventProperties[0];
    setConditions(rows => [
      ...rows,
      {
        id: newId(),
        source,
        field: source === 'trigger' ? (firstProp?.name ?? '') : source === 'player' ? 'level' : 'event_type',
        operator: source === 'trigger' && firstProp?.type === 'number' ? 'gte' : 'eq',
        value: '',
        kind: source === 'player' || firstProp?.type === 'number' || firstProp?.type === 'integer' ? 'number' : 'text',
        negate: false,
      },
    ]);
  };
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
  const buildDefinition = (): { conditions: RuleConditions; actions: RuleAction[]; limits: RuleLimits } | null => {
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
    return { conditions: conds, actions: actions.map(rowToAction), limits: lim };
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
      let saved: Rule;
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
        });
      } else {
        const patch: UpdateRuleData = {};
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
          canonical(def.limits) !== canonical(latest.limits ?? {});

        if (definitionChanged && latest && !latest.published) {
          // The latest version is still a draft: edit it in place.
          patch.conditions = def.conditions;
          patch.actions = def.actions;
          patch.limits = limitsOrUndefined ?? null;
        }
        saved = rule;
        if (Object.keys(patch).length > 0) {
          saved = await updateRule.mutateAsync({ ruleId: rule.id, data: patch });
        }
        if (definitionChanged && (!latest || latest.published)) {
          // Published versions are immutable: create a new draft version.
          const version = await createVersion.mutateAsync({
            ruleId: rule.id,
            data: { conditions: def.conditions, actions: def.actions, limits: limitsOrUndefined },
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
    ([k]) => !k.startsWith('conditions') && !k.startsWith('actions') && !k.startsWith('limits'),
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
                    {'{"source", "field", "operator", "value"}'}.
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
                          <Select
                            value={row.source}
                            onValueChange={v => updateCondition(row.id, { source: v as ConditionSource, field: '' })}
                          >
                            <SelectTrigger className="w-36">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {(Object.keys(sourceLabels) as ConditionSource[]).map(s => (
                                <SelectItem key={s} value={s}>
                                  {sourceLabels[s]}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
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
              />
              <p className="text-xs text-muted-foreground mt-2">
                Unsaved and draft changes are not simulated: publish first to see this rule's effects.
              </p>
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

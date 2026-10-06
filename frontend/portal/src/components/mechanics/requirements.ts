// Badge requirements grammar helpers (badges/internal/domain/requirements.go):
// {all?: [cond], any?: [cond]}, cond = {metric, event_type? (activity_count only), gte >= 1}.
import type {
  BadgeRequirementCondition,
  BadgeRequirementMetric,
  BadgeRequirements,
} from '@/services/api/types';

/** Metrics of the requirements grammar (badges/internal/domain/requirements.go). */
export const requirementMetricLabels: Record<BadgeRequirementMetric, { label: string; unit: string }> = {
  lifetime_points: { label: 'Lifetime points earned', unit: 'points' },
  missions_completed: { label: 'Missions completed', unit: 'missions' },
  streak_days: { label: 'Streak length (best current streak)', unit: 'periods' },
  level: { label: 'Level reached', unit: '' },
  badges_earned: { label: 'Badges earned (any badge)', unit: 'badges' },
  activity_count: { label: 'Activities recorded', unit: 'activities' },
};

export const REQUIREMENT_METRICS = Object.keys(requirementMetricLabels) as BadgeRequirementMetric[];
/** Backend cap per list. */
export const MAX_REQUIREMENT_CONDITIONS = 20;

export interface RequirementsDraft {
  all: BadgeRequirementCondition[];
  any: BadgeRequirementCondition[];
}

export const emptyRequirementsDraft: RequirementsDraft = { all: [], any: [] };

function isCondition(item: unknown): item is BadgeRequirementCondition {
  if (!item || typeof item !== 'object') return false;
  const c = item as Record<string, unknown>;
  return (
    typeof c.metric === 'string' &&
    (REQUIREMENT_METRICS as string[]).includes(c.metric) &&
    typeof c.gte === 'number' &&
    (c.event_type === undefined || typeof c.event_type === 'string')
  );
}

/**
 * Reads a badge's stored requirements into the editor. `unsupported` is
 * true when the stored JSON is not in the all/any grammar (it would be
 * replaced on save).
 */
export function requirementsToDraft(raw: Record<string, unknown> | null | undefined): {
  draft: RequirementsDraft;
  unsupported: boolean;
} {
  if (!raw || Object.keys(raw).length === 0) return { draft: emptyRequirementsDraft, unsupported: false };
  const unknownKeys = Object.keys(raw).some((k) => k !== 'all' && k !== 'any');
  const list = (key: 'all' | 'any') => {
    const v = raw[key];
    if (v === undefined || v === null) return { items: [] as BadgeRequirementCondition[], ok: true };
    if (!Array.isArray(v)) return { items: [] as BadgeRequirementCondition[], ok: false };
    return { items: v.filter(isCondition).map((c) => ({ ...c })), ok: v.every(isCondition) };
  };
  const all = list('all');
  const any = list('any');
  return { draft: { all: all.items, any: any.items }, unsupported: unknownKeys || !all.ok || !any.ok };
}

/** The requirements payload, or null when there are no conditions (explicit awards only). */
export function draftToRequirements(draft: RequirementsDraft): BadgeRequirements | null {
  const clean = (list: BadgeRequirementCondition[]) =>
    list.map((c) => {
      const out: BadgeRequirementCondition = { metric: c.metric, gte: c.gte };
      if (c.metric === 'activity_count' && c.event_type) out.event_type = c.event_type;
      return out;
    });
  if (draft.all.length === 0 && draft.any.length === 0) return null;
  const out: BadgeRequirements = {};
  if (draft.all.length > 0) out.all = clean(draft.all);
  if (draft.any.length > 0) out.any = clean(draft.any);
  return out;
}

/** Client-side check mirroring the backend grammar; null when valid. */
export function validateRequirementsDraft(draft: RequirementsDraft): string | null {
  for (const [key, list] of [['all', draft.all], ['any', draft.any]] as const) {
    if (list.length > MAX_REQUIREMENT_CONDITIONS) return `"${key}" holds at most ${MAX_REQUIREMENT_CONDITIONS} conditions.`;
    for (const c of list) {
      if (!Number.isInteger(c.gte) || c.gte < 1) return 'Every threshold must be a whole number of at least 1.';
    }
  }
  return null;
}

/** One-line description of a condition, e.g. "Missions completed ≥ 5". */
export function describeCondition(c: BadgeRequirementCondition): string {
  const meta = requirementMetricLabels[c.metric];
  const scope = c.metric === 'activity_count' && c.event_type ? ` (${c.event_type})` : '';
  return `${meta?.label ?? c.metric}${scope} ≥ ${c.gte.toLocaleString()}`;
}

/** Human summary of stored requirements for cards; [] when none or unreadable. */
export function summarizeRequirements(raw: Record<string, unknown> | null | undefined): { all: string[]; any: string[] } {
  const { draft } = requirementsToDraft(raw);
  return { all: draft.all.map(describeCondition), any: draft.any.map(describeCondition) };
}

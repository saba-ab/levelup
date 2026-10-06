import { Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { EventTypeSelect } from './EventTypeSelect';
import {
  MAX_REQUIREMENT_CONDITIONS,
  REQUIREMENT_METRICS,
  requirementMetricLabels,
  type RequirementsDraft,
} from './requirements';
import type { BadgeRequirementCondition, BadgeRequirementMetric } from '@/services/api/types';

interface ConditionListProps {
  title: string;
  hint: string;
  conditions: BadgeRequirementCondition[];
  onChange: (next: BadgeRequirementCondition[]) => void;
  idPrefix: string;
  disabled?: boolean;
}

function ConditionList({ title, hint, conditions, onChange, idPrefix, disabled }: ConditionListProps) {
  const update = (index: number, patch: Partial<BadgeRequirementCondition>) =>
    onChange(conditions.map((c, i) => (i === index ? { ...c, ...patch } : c)));

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div>
          <p className="text-sm font-medium">{title}</p>
          <p className="text-xs text-muted-foreground">{hint}</p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled || conditions.length >= MAX_REQUIREMENT_CONDITIONS}
          onClick={() => onChange([...conditions, { metric: 'lifetime_points', gte: 1 }])}
        >
          <Plus className="w-3 h-3 mr-1" /> Add condition
        </Button>
      </div>
      {conditions.length === 0 ? (
        <p className="text-xs text-muted-foreground italic">No conditions.</p>
      ) : (
        <div className="space-y-2">
          {conditions.map((c, index) => {
            const rowId = `${idPrefix}-${index}`;
            return (
              <div key={rowId} className="grid grid-cols-1 sm:grid-cols-[1fr_1fr_7rem_auto] gap-2 items-end rounded-md border border-border p-2">
                <div className="space-y-1">
                  <Label htmlFor={`${rowId}-metric`} className="text-xs">Metric</Label>
                  <Select
                    value={c.metric}
                    onValueChange={(v) =>
                      update(index, {
                        metric: v as BadgeRequirementMetric,
                        event_type: v === 'activity_count' ? c.event_type : undefined,
                      })
                    }
                    disabled={disabled}
                  >
                    <SelectTrigger id={`${rowId}-metric`}><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {REQUIREMENT_METRICS.map((m) => (
                        <SelectItem key={m} value={m}>{requirementMetricLabels[m].label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1">
                  <Label htmlFor={`${rowId}-event`} className="text-xs">Event type</Label>
                  {c.metric === 'activity_count' ? (
                    <EventTypeSelect
                      id={`${rowId}-event`}
                      value={c.event_type ?? ''}
                      onChange={(slug) => update(index, { event_type: slug || undefined })}
                      emptyLabel="Any event type"
                      disabled={disabled}
                    />
                  ) : (
                    <p className="text-xs text-muted-foreground h-10 flex items-center">Only for activity counts</p>
                  )}
                </div>
                <div className="space-y-1">
                  <Label htmlFor={`${rowId}-gte`} className="text-xs">At least</Label>
                  <Input
                    id={`${rowId}-gte`}
                    type="number"
                    min={1}
                    step={1}
                    value={c.gte}
                    disabled={disabled}
                    onChange={(e) => update(index, { gte: Math.max(1, Math.floor(Number(e.target.value) || 1)) })}
                  />
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="Remove condition"
                  disabled={disabled}
                  onClick={() => onChange(conditions.filter((_, i) => i !== index))}
                >
                  <Trash2 className="w-4 h-4" />
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

interface BadgeRequirementsEditorProps {
  value: RequirementsDraft;
  onChange: (next: RequirementsDraft) => void;
  /** Stored requirements were not in the all/any grammar. */
  unsupported?: boolean;
  /** Server message for 422 invalid_badge_requirements. */
  error?: string | null;
  disabled?: boolean;
}

/**
 * Editor of a badge's automatic-award requirements:
 * `{all?: [cond], any?: [cond]}` with cond `{metric, event_type?, gte}`.
 */
export function BadgeRequirementsEditor({ value, onChange, unsupported, error, disabled }: BadgeRequirementsEditorProps) {
  const none = value.all.length === 0 && value.any.length === 0;
  return (
    <div className="space-y-4 rounded-lg border border-border p-4">
      <div>
        <h4 className="font-semibold text-sm">Automatic award requirements</h4>
        <p className="text-xs text-muted-foreground">
          {none
            ? 'No requirements: the badge is only awarded explicitly (API, rules or "Award to player").'
            : 'The badge is awarded automatically once a player meets every "All" condition and, if any are listed, at least one "Any" condition.'}
        </p>
      </div>
      {unsupported && (
        <p className="text-xs text-amber-500">
          The saved requirements use an older format and are not shown here. Saving with conditions replaces them.
        </p>
      )}
      <ConditionList
        title="All of"
        hint="Every condition must hold."
        conditions={value.all}
        onChange={(all) => onChange({ ...value, all })}
        idPrefix="req-all"
        disabled={disabled}
      />
      <ConditionList
        title="Any of"
        hint="At least one condition must hold."
        conditions={value.any}
        onChange={(any) => onChange({ ...value, any })}
        idPrefix="req-any"
        disabled={disabled}
      />
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  );
}

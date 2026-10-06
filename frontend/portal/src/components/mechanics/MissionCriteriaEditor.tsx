import { Plus, Trash2, Zap, Hand } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useEventsQuery } from '@/services/queries/events';
import { EventTypeSelect } from './EventTypeSelect';
import { eventPropertyPaths } from './event-utils';
import {
  CRITERIA_OPERATORS,
  MAX_CRITERIA_CONDITIONS,
  NUMERIC_OPERATORS,
  criteriaOperatorLabels,
  type CriteriaConditionDraft,
  type CriteriaDraft,
  type CriteriaValueType,
} from './criteria';
import type { MissionCriteriaOperator } from '@/services/api/types';

interface MissionCriteriaEditorProps {
  value: CriteriaDraft;
  onChange: (next: CriteriaDraft) => void;
  /** Field errors keyed "criteria.*" (client checks or the 422 invalid_mission_criteria response). */
  errors?: Record<string, string>;
  /** Stored criteria held keys outside the grammar. */
  unsupported?: boolean;
  disabled?: boolean;
}

function FieldError({ message }: { message?: string }) {
  return message ? <p className="text-xs text-destructive">{message}</p> : null;
}

/**
 * Editor of a mission's criteria: which event type progresses it, optional
 * property conditions, and how much each matching activity adds.
 */
export function MissionCriteriaEditor({ value, onChange, errors = {}, unsupported, disabled }: MissionCriteriaEditorProps) {
  const { data: events = [] } = useEventsQuery();
  const automatic = !!value.event_type;
  const suggestions = automatic ? eventPropertyPaths(events, value.event_type) : [];
  const datalistId = 'mission-criteria-properties';

  const updateCondition = (index: number, patch: Partial<CriteriaConditionDraft>) =>
    onChange({ ...value, where: value.where.map((c, i) => (i === index ? { ...c, ...patch } : c)) });

  const setOperator = (index: number, operator: MissionCriteriaOperator) => {
    const current = value.where[index];
    const patch: Partial<CriteriaConditionDraft> = { operator };
    if (operator === 'exists') patch.value = 'true';
    else if (current.operator === 'exists') patch.value = '';
    if (NUMERIC_OPERATORS.includes(operator)) patch.valueType = 'number';
    updateCondition(index, patch);
  };

  return (
    <div className="space-y-4 rounded-lg border border-border p-4">
      <div className="flex items-start gap-3">
        {automatic ? <Zap className="w-5 h-5 text-primary mt-0.5" /> : <Hand className="w-5 h-5 text-muted-foreground mt-0.5" />}
        <div>
          <h4 className="font-semibold text-sm">{automatic ? 'Automatic progress' : 'Manual progress'}</h4>
          <p className="text-xs text-muted-foreground">
            {automatic
              ? 'Every matching activity of this event type progresses the player\'s attempt (starting one if needed).'
              : 'No event type: progress comes only from rules or the API ("Add progress").'}
          </p>
        </div>
      </div>

      {unsupported && (
        <p className="text-xs text-amber-500">
          The saved criteria contain keys this editor does not support; they are dropped when you save.
        </p>
      )}

      <div className="space-y-2">
        <Label htmlFor="criteria-event-type">Event type</Label>
        <EventTypeSelect
          id="criteria-event-type"
          value={value.event_type}
          onChange={(slug) => onChange(slug ? { ...value, event_type: slug } : { ...value, event_type: '', where: [], incrementBy: 'count', incrementField: '' })}
          emptyLabel="None (manual / rule-driven)"
          disabled={disabled}
        />
        <FieldError message={errors['criteria.event_type']} />
      </div>

      {automatic && (
        <>
          <datalist id={datalistId}>
            {suggestions.map((p) => <option key={p} value={p} />)}
          </datalist>

          <div className="space-y-2">
            <div className="flex items-center justify-between gap-2">
              <div>
                <p className="text-sm font-medium">Conditions</p>
                <p className="text-xs text-muted-foreground">All must hold on the activity's properties (dot paths, e.g. cart.total).</p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={disabled || value.where.length >= MAX_CRITERIA_CONDITIONS}
                onClick={() =>
                  onChange({ ...value, where: [...value.where, { field: '', operator: 'eq', valueType: 'string', value: '' }] })
                }
              >
                <Plus className="w-3 h-3 mr-1" /> Add condition
              </Button>
            </div>
            <FieldError message={errors['criteria.where']} />
            {value.where.length === 0 ? (
              <p className="text-xs text-muted-foreground italic">No conditions: every activity of this type counts.</p>
            ) : (
              value.where.map((c, index) => {
                const at = `criteria.where[${index}]`;
                const rowId = `criteria-where-${index}`;
                const numeric = NUMERIC_OPERATORS.includes(c.operator);
                const rowErrors = Object.entries(errors).filter(([k]) => k === at || k.startsWith(`${at}.`));
                return (
                  <div key={rowId} className="rounded-md border border-border p-2 space-y-2">
                    <div className="grid grid-cols-1 sm:grid-cols-[1fr_9rem_7rem_1fr_auto] gap-2 items-end">
                      <div className="space-y-1">
                        <Label htmlFor={`${rowId}-field`} className="text-xs">Property</Label>
                        <Input
                          id={`${rowId}-field`}
                          list={datalistId}
                          maxLength={200}
                          placeholder="e.g. cart.total"
                          value={c.field}
                          disabled={disabled}
                          onChange={(e) => updateCondition(index, { field: e.target.value.trim() })}
                        />
                      </div>
                      <div className="space-y-1">
                        <Label htmlFor={`${rowId}-op`} className="text-xs">Operator</Label>
                        <Select value={c.operator} onValueChange={(v) => setOperator(index, v as MissionCriteriaOperator)} disabled={disabled}>
                          <SelectTrigger id={`${rowId}-op`}><SelectValue /></SelectTrigger>
                          <SelectContent>
                            {CRITERIA_OPERATORS.map((op) => (
                              <SelectItem key={op} value={op}>{criteriaOperatorLabels[op]}</SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-1">
                        <Label htmlFor={`${rowId}-type`} className="text-xs">Type</Label>
                        <Select
                          value={c.operator === 'exists' ? 'boolean' : numeric ? 'number' : c.valueType}
                          onValueChange={(v) => updateCondition(index, { valueType: v as CriteriaValueType })}
                          disabled={disabled || numeric || c.operator === 'exists'}
                        >
                          <SelectTrigger id={`${rowId}-type`}><SelectValue /></SelectTrigger>
                          <SelectContent>
                            <SelectItem value="string">Text</SelectItem>
                            <SelectItem value="number">Number</SelectItem>
                            <SelectItem value="boolean">Boolean</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-1">
                        <Label htmlFor={`${rowId}-value`} className="text-xs">Value</Label>
                        {c.operator === 'exists' ? (
                          <Select value={c.value === 'false' ? 'false' : 'true'} onValueChange={(v) => updateCondition(index, { value: v })} disabled={disabled}>
                            <SelectTrigger id={`${rowId}-value`}><SelectValue /></SelectTrigger>
                            <SelectContent>
                              <SelectItem value="true">is present</SelectItem>
                              <SelectItem value="false">is missing</SelectItem>
                            </SelectContent>
                          </Select>
                        ) : c.valueType === 'boolean' && !numeric && c.operator !== 'in' ? (
                          <Select value={c.value || undefined} onValueChange={(v) => updateCondition(index, { value: v })} disabled={disabled}>
                            <SelectTrigger id={`${rowId}-value`}><SelectValue placeholder="true / false" /></SelectTrigger>
                            <SelectContent>
                              <SelectItem value="true">true</SelectItem>
                              <SelectItem value="false">false</SelectItem>
                            </SelectContent>
                          </Select>
                        ) : (
                          <Input
                            id={`${rowId}-value`}
                            inputMode={numeric || c.valueType === 'number' ? 'decimal' : undefined}
                            placeholder={c.operator === 'in' ? 'a, b, c' : numeric ? '100' : 'value'}
                            value={c.value}
                            disabled={disabled}
                            onChange={(e) => updateCondition(index, { value: e.target.value })}
                          />
                        )}
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label="Remove condition"
                        disabled={disabled}
                        onClick={() => onChange({ ...value, where: value.where.filter((_, i) => i !== index) })}
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>
                    {rowErrors.map(([k, msg]) => (
                      <p key={k} className="text-xs text-destructive">
                        {k.slice(at.length + 1) || 'condition'}: {msg}
                      </p>
                    ))}
                  </div>
                );
              })
            )}
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="criteria-increment">Each matching activity adds</Label>
              <Select
                value={value.incrementBy}
                onValueChange={(v) => onChange({ ...value, incrementBy: v as 'count' | 'property' })}
                disabled={disabled}
              >
                <SelectTrigger id="criteria-increment"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="count">1 (count activities)</SelectItem>
                  <SelectItem value="property">The value of a property</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {value.incrementBy === 'property' && (
              <div className="space-y-2">
                <Label htmlFor="criteria-increment-field">Property</Label>
                <Input
                  id="criteria-increment-field"
                  list={datalistId}
                  maxLength={200}
                  placeholder="e.g. quantity"
                  value={value.incrementField}
                  disabled={disabled}
                  onChange={(e) => onChange({ ...value, incrementField: e.target.value.trim() })}
                />
                <p className="text-xs text-muted-foreground">Floored to a whole number; values below 1 do not progress.</p>
              </div>
            )}
          </div>
          <FieldError message={errors['criteria.increment'] ?? errors['criteria.increment.field'] ?? errors['criteria.increment.by']} />
        </>
      )}

      {Object.entries(errors)
        .filter(([k]) => !k.startsWith('criteria.where') && !k.startsWith('criteria.increment') && k !== 'criteria.event_type' && k.startsWith('criteria'))
        .map(([k, msg]) => (
          <p key={k} className="text-xs text-destructive">{k}: {msg}</p>
        ))}
    </div>
  );
}

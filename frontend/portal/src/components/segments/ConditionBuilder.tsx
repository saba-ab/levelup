import { FolderPlus, Plus, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { cn } from '@/lib/utils';
import type { SegmentMatch, SegmentOperator } from '@/services/api/models/segments';
import {
  FIELD_OPTIONS,
  MAX_CONDITIONS,
  MAX_DEPTH,
  OPERATOR_LABELS,
  countConditions,
  defaultValue,
  newCondition,
  newGroup,
  operatorsFor,
  replaceNode,
  valueKindFor,
  type EditorCondition,
  type EditorGroup,
  type FieldKind,
} from './conditionModel';

export interface BadgeOption {
  id: string;
  name: string;
}

interface ConditionBuilderProps {
  value: EditorGroup;
  onChange: (next: EditorGroup) => void;
  badges: BadgeOption[];
  badgesLoading?: boolean;
  disabled?: boolean;
}

/** Visual editor for {"all"|"any": [...]} conditions with nested groups (max depth 3). */
export function ConditionBuilder({ value, onChange, badges, badgesLoading, disabled }: ConditionBuilderProps) {
  const total = countConditions(value);
  const atLimit = total >= MAX_CONDITIONS;

  const update = (key: string, next: EditorCondition | EditorGroup | null) => {
    if (key === value.key) {
      if (next && next.kind === 'group') onChange(next);
      return;
    }
    onChange(replaceNode(value, key, next));
  };

  return (
    <div className="flex flex-col gap-2">
      <GroupEditor
        group={value}
        depth={1}
        update={update}
        badges={badges}
        badgesLoading={badgesLoading}
        disabled={disabled}
        atLimit={atLimit}
      />
      <p className="text-xs text-muted-foreground">
        {total} of {MAX_CONDITIONS} conditions · groups nest up to {MAX_DEPTH} levels
      </p>
    </div>
  );
}

interface GroupEditorProps {
  group: EditorGroup;
  depth: number;
  update: (key: string, next: EditorCondition | EditorGroup | null) => void;
  badges: BadgeOption[];
  badgesLoading?: boolean;
  disabled?: boolean;
  atLimit: boolean;
  onRemove?: () => void;
}

function GroupEditor({ group, depth, update, badges, badgesLoading, disabled, atLimit, onRemove }: GroupEditorProps) {
  const setGroup = (patch: Partial<EditorGroup>) => update(group.key, { ...group, ...patch });

  return (
    <div
      className={cn(
        'flex flex-col gap-3 rounded-lg border p-3',
        depth === 1 ? 'border-border bg-muted/20' : 'border-dashed border-primary/40 bg-primary/5',
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-muted-foreground">Players matching</span>
        <Select value={group.match} onValueChange={(v) => setGroup({ match: v as SegmentMatch })} disabled={disabled}>
          <SelectTrigger className="h-8 w-[90px]" aria-label="Group match mode">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">all</SelectItem>
            <SelectItem value="any">any</SelectItem>
          </SelectContent>
        </Select>
        <span className="text-sm text-muted-foreground">of these</span>
        {onRemove && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="ml-auto h-8 w-8"
            onClick={onRemove}
            disabled={disabled}
            aria-label="Remove group"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
      </div>

      {group.items.length === 0 && (
        <p className="text-sm text-muted-foreground">This group is empty. Add a condition.</p>
      )}

      {group.items.map((item, index) => (
        <div key={item.key} className="flex flex-col gap-2">
          {index > 0 && (
            <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {group.match === 'all' ? 'and' : 'or'}
            </span>
          )}
          {item.kind === 'group' ? (
            <GroupEditor
              group={item}
              depth={depth + 1}
              update={update}
              badges={badges}
              badgesLoading={badgesLoading}
              disabled={disabled}
              atLimit={atLimit}
              onRemove={() => update(item.key, null)}
            />
          ) : (
            <ConditionRow
              condition={item}
              onChange={(next) => update(item.key, next)}
              onRemove={() => update(item.key, null)}
              badges={badges}
              badgesLoading={badgesLoading}
              disabled={disabled}
            />
          )}
        </div>
      ))}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setGroup({ items: [...group.items, newCondition()] })}
          disabled={disabled || atLimit}
        >
          <Plus className="mr-1 h-4 w-4" />
          Condition
        </Button>
        {depth < MAX_DEPTH && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setGroup({ items: [...group.items, newGroup(group.match === 'all' ? 'any' : 'all')] })}
            disabled={disabled || atLimit}
          >
            <FolderPlus className="mr-1 h-4 w-4" />
            Group
          </Button>
        )}
      </div>
    </div>
  );
}

interface ConditionRowProps {
  condition: EditorCondition;
  onChange: (next: EditorCondition) => void;
  onRemove: () => void;
  badges: BadgeOption[];
  badgesLoading?: boolean;
  disabled?: boolean;
}

function ConditionRow({ condition, onChange, onRemove, badges, badgesLoading, disabled }: ConditionRowProps) {
  const ops = operatorsFor(condition.field);
  const kind = valueKindFor(condition.field, condition.op);

  const setField = (field: FieldKind) => {
    const op = operatorsFor(field)[0];
    onChange({ ...condition, field, op, value: defaultValue(field, op) });
  };

  const setOp = (op: SegmentOperator) => {
    const sameKind = valueKindFor(condition.field, op) === kind;
    onChange({ ...condition, op, value: sameKind ? condition.value : defaultValue(condition.field, op) });
  };

  const setValue = (value: string) => onChange({ ...condition, value });

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-background p-2">
      <Select value={condition.field} onValueChange={(v) => setField(v as FieldKind)} disabled={disabled}>
        <SelectTrigger className="h-9 w-[190px]" aria-label="Field">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {FIELD_OPTIONS.map((f) => (
            <SelectItem key={f.value} value={f.value}>
              {f.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {condition.field === 'attributes' && (
        <Input
          className="h-9 w-[160px]"
          placeholder="path, e.g. plan"
          value={condition.attrPath}
          onChange={(e) => onChange({ ...condition, attrPath: e.target.value })}
          disabled={disabled}
          aria-label="Attribute path"
        />
      )}

      <Select value={condition.op} onValueChange={(v) => setOp(v as SegmentOperator)} disabled={disabled}>
        <SelectTrigger className="h-9 w-[170px]" aria-label="Operator">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {ops.map((op) => (
            <SelectItem key={op} value={op}>
              {OPERATOR_LABELS[op]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {kind === 'boolean' && (
        <Select value={condition.value || 'true'} onValueChange={setValue} disabled={disabled}>
          <SelectTrigger className="h-9 w-[110px]" aria-label="Value">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="true">true</SelectItem>
            <SelectItem value="false">false</SelectItem>
          </SelectContent>
        </Select>
      )}

      {kind === 'badge' && (
        <Select value={condition.value || undefined} onValueChange={setValue} disabled={disabled || badgesLoading}>
          <SelectTrigger className="h-9 w-[220px]" aria-label="Badge">
            <SelectValue placeholder={badgesLoading ? 'Loading badges…' : 'Choose a badge'} />
          </SelectTrigger>
          <SelectContent>
            {badges.length === 0 && !badgesLoading && (
              <div className="px-2 py-1.5 text-sm text-muted-foreground">No badges yet</div>
            )}
            {badges.map((b) => (
              <SelectItem key={b.id} value={b.id}>
                {b.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      {kind === 'date' && (
        <Input
          type="date"
          className="h-9 w-[170px]"
          value={condition.value}
          onChange={(e) => setValue(e.target.value)}
          disabled={disabled}
          aria-label="Date"
        />
      )}

      {kind === 'number' && (
        <Input
          type="number"
          className="h-9 w-[130px]"
          value={condition.value}
          onChange={(e) => setValue(e.target.value)}
          disabled={disabled}
          aria-label="Value"
        />
      )}

      {(kind === 'text' || kind === 'list') && (
        <Input
          className="h-9 min-w-[160px] flex-1"
          placeholder={kind === 'list' ? 'value1, value2, …' : 'value'}
          value={condition.value}
          onChange={(e) => setValue(e.target.value)}
          disabled={disabled}
          aria-label="Value"
        />
      )}

      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="ml-auto h-8 w-8"
        onClick={onRemove}
        disabled={disabled}
        aria-label="Remove condition"
      >
        <Trash2 className="h-4 w-4" />
      </Button>
    </div>
  );
}

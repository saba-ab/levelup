import { useEffect, useState } from 'react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { lastDays, rangeDays, rangeProblem, todayUTC, type DayRange } from './dateRange';

const PRESETS = [7, 30, 90, 180, 365] as const;
const CUSTOM = 'custom';

interface DateRangeControlProps {
  value: DayRange;
  onChange: (next: DayRange) => void;
}

function presetOf(r: DayRange): string {
  if (r.to !== todayUTC()) return CUSTOM;
  const days = rangeDays(r);
  return PRESETS.some((p) => p === days) ? String(days) : CUSTOM;
}

/** Preset or custom UTC day range; only valid ranges reach onChange. */
export function DateRangeControl({ value, onChange }: DateRangeControlProps) {
  const [draft, setDraft] = useState<DayRange>(value);
  const [mode, setMode] = useState(() => presetOf(value));
  useEffect(() => setDraft(value), [value]);

  const problem = rangeProblem(draft);

  const setDraftDay = (key: keyof DayRange, day: string) => {
    const next = { ...draft, [key]: day };
    setDraft(next);
    if (!rangeProblem(next)) onChange(next);
  };

  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-end gap-2">
        <Select
          value={mode}
          onValueChange={(v) => {
            setMode(v);
            if (v !== CUSTOM) onChange(lastDays(Number(v)));
          }}
        >
          <SelectTrigger className="w-[160px]" aria-label="Date range">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PRESETS.map((d) => (
              <SelectItem key={d} value={String(d)}>
                Last {d} days
              </SelectItem>
            ))}
            <SelectItem value={CUSTOM}>Custom range</SelectItem>
          </SelectContent>
        </Select>
        {mode === CUSTOM && (
          <>
            <div className="flex flex-col gap-1">
              <Label htmlFor="analytics-from" className="text-xs text-muted-foreground">
                From
              </Label>
              <Input
                id="analytics-from"
                type="date"
                className="w-[150px]"
                value={draft.from}
                max={draft.to}
                onChange={(e) => setDraftDay('from', e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor="analytics-to" className="text-xs text-muted-foreground">
                To
              </Label>
              <Input
                id="analytics-to"
                type="date"
                className="w-[150px]"
                value={draft.to}
                min={draft.from}
                max={todayUTC()}
                onChange={(e) => setDraftDay('to', e.target.value)}
              />
            </div>
          </>
        )}
      </div>
      {mode === CUSTOM && problem && <p className="text-xs text-destructive">{problem}</p>}
      <p className="text-xs text-muted-foreground">Days are UTC.</p>
    </div>
  );
}

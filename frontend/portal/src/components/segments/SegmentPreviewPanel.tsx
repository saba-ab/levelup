import { useMemo } from 'react';
import { AlertCircle, Loader2, Users } from 'lucide-react';
import { useDebouncedValue } from '@/components/players/useDebouncedValue';
import { describeSegmentError, useSegmentPreviewQuery } from '@/services/queries/segments';
import type { SegmentConditionGroup } from '@/services/api/models/segments';

interface SegmentPreviewPanelProps {
  /** Valid conditions, or null while the builder has problems. */
  conditions: SegmentConditionGroup | null;
  /** Builder problems to show instead of an estimate. */
  problems: string[];
}

/** Debounced POST /segments/preview: estimate plus a sample of matching players. */
export function SegmentPreviewPanel({ conditions, problems }: SegmentPreviewPanelProps) {
  const serialized = conditions ? JSON.stringify(conditions) : null;
  const debounced = useDebouncedValue(serialized, 500);
  const debouncedConditions = useMemo(
    () => (debounced ? (JSON.parse(debounced) as SegmentConditionGroup) : null),
    [debounced],
  );
  const { data, error, isFetching } = useSegmentPreviewQuery(debouncedConditions);
  const settling = serialized !== debounced || isFetching;

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border p-4">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium">Live preview</h3>
        {settling && conditions && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
      </div>

      {problems.length > 0 ? (
        <div className="flex flex-col gap-1 text-sm text-muted-foreground">
          <p>Complete the conditions to see an estimate:</p>
          <ul className="list-disc pl-5">
            {problems.slice(0, 5).map((p) => (
              <li key={p}>{p}</li>
            ))}
            {problems.length > 5 && <li>and {problems.length - 5} more</li>}
          </ul>
        </div>
      ) : error ? (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <span className="whitespace-pre-line">{describeSegmentError(error, 'Preview failed')}</span>
        </div>
      ) : !data ? (
        <p className="text-sm text-muted-foreground">Calculating…</p>
      ) : (
        <>
          <div className="flex items-baseline gap-2">
            <Users className="h-4 w-4 self-center text-primary" />
            <span className="text-2xl font-bold">
              {data.complete ? '' : '≥ '}
              {data.count_estimate.toLocaleString()}
            </span>
            <span className="text-sm text-muted-foreground">
              matching players
              {data.complete
                ? ` of ${data.scanned.toLocaleString()}`
                : ` in the first ${data.scanned.toLocaleString()} scanned`}
            </span>
          </div>
          {!data.complete && (
            <p className="text-xs text-muted-foreground">
              The preview scans a sample of players. The exact count is computed when the segment is saved and refreshed.
            </p>
          )}
          {data.sample.length > 0 ? (
            <div className="flex flex-col gap-1">
              <p className="text-xs font-medium text-muted-foreground">Sample</p>
              <ul className="flex flex-col divide-y divide-border rounded-md border border-border">
                {data.sample.map((p) => (
                  <li key={p.id} className="flex items-center justify-between gap-2 px-3 py-1.5 text-sm">
                    <span className="truncate">{p.display_name || p.external_id}</span>
                    <span className="truncate font-mono text-xs text-muted-foreground">{p.external_id}</span>
                  </li>
                ))}
              </ul>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">No players match yet.</p>
          )}
        </>
      )}
    </div>
  );
}

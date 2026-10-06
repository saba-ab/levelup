import type { SegmentCondition, SegmentConditionGroup } from '@/services/api/models/segments';
import { cn } from '@/lib/utils';
import { describeCondition } from './conditionModel';

interface ConditionSummaryProps {
  conditions: SegmentConditionGroup;
  badgeName?: (id: string) => string | undefined;
  className?: string;
}

/** Read-only rendering of a segment's condition tree. */
export function ConditionSummary({ conditions, badgeName, className }: ConditionSummaryProps) {
  const match = 'any' in conditions && conditions.any ? 'any' : 'all';
  const items = (match === 'any' ? conditions.any : conditions.all) ?? [];
  return (
    <div className={cn('flex flex-col gap-1.5 text-sm', className)}>
      <span className="text-muted-foreground">
        Players matching <span className="font-medium text-foreground">{match}</span> of:
      </span>
      <ul className="flex flex-col gap-1.5 border-l-2 border-primary/30 pl-3">
        {items.map((item, i) =>
          'all' in item || 'any' in item ? (
            <li key={i}>
              <ConditionSummary conditions={item as SegmentConditionGroup} badgeName={badgeName} />
            </li>
          ) : (
            <li key={i}>{describeCondition(item as SegmentCondition, badgeName)}</li>
          ),
        )}
      </ul>
    </div>
  );
}

import { formatDateTime } from '@/lib/player-utils';
import { cn } from '@/lib/utils';
import { formatRelativeTime } from './relativeTime';

interface LastSeenProps {
  /** last_activity_at; null/undefined = no activity recorded. */
  at: string | null | undefined;
  /** last_event_type, shown under the time. */
  eventType?: string;
  className?: string;
}

/** A player's latest activity (GET /activities/last-seen), relative with the exact time on hover. */
export function LastSeen({ at, eventType, className }: LastSeenProps) {
  if (!at) return <span className={cn('text-muted-foreground', className)}>Never</span>;
  return (
    <span className={cn('inline-flex flex-col', className)} title={formatDateTime(at)}>
      <span>{formatRelativeTime(at)}</span>
      {eventType && <code className="text-[11px] text-muted-foreground">{eventType}</code>}
    </span>
  );
}

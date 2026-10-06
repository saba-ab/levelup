import { formatDistanceToNow } from 'date-fns';
import { Loader2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { isSegmentRefreshPending } from '@/services/queries/segments';
import type { Segment } from '@/services/api/models/segments';

/** "Refreshing…" while a refresh runs or is queued, else when it last completed. */
export function RefreshStatus({ segment }: { segment: Segment }) {
  if (isSegmentRefreshPending(segment)) {
    return (
      <Badge variant="secondary" className="gap-1">
        <Loader2 className="h-3 w-3 animate-spin" />
        {segment.refreshing ? 'Refreshing' : 'Refresh queued'}
      </Badge>
    );
  }
  if (!segment.last_refreshed_at) {
    return <span className="text-sm text-muted-foreground">Never refreshed</span>;
  }
  return (
    <span className="text-sm text-muted-foreground" title={new Date(segment.last_refreshed_at).toLocaleString()}>
      {formatDistanceToNow(new Date(segment.last_refreshed_at), { addSuffix: true })}
    </span>
  );
}

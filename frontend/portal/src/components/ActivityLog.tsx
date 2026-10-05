import { useState } from 'react';
import { Activity as ActivityIcon, RefreshCw, Send } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useActivitiesQuery } from '@/services/queries/activities';
import type { Activity, ActivityStatus } from '@/services/api/types';
import CursorPager from './CursorPager';
import SendActivityDialog from './SendActivityDialog';
import { DecisionDetail } from './RuleTrace';

const statusClass: Record<ActivityStatus, string> = {
  pending: 'border-amber-500/50 text-amber-500 bg-amber-500/10',
  decided: 'border-green-500/50 text-green-500 bg-green-500/10',
  rejected: 'border-destructive/50 text-destructive bg-destructive/10',
};

/** Ingested activities (GET /activities) with their rules decision. */
export default function ActivityLog({ canSend }: { canSend: boolean }) {
  const pager = useCursorPagination(25);
  const [status, setStatus] = useState<'all' | ActivityStatus>('all');
  const [eventType, setEventType] = useState('');
  const [playerExternalId, setPlayerExternalId] = useState('');
  const [selected, setSelected] = useState<Activity | null>(null);
  const [sendOpen, setSendOpen] = useState(false);

  const { data, isLoading, isFetching, error, refetch } = useActivitiesQuery({
    limit: pager.limit,
    cursor: pager.cursor,
    status: status === 'all' ? undefined : status,
    event_type: eventType.trim() || undefined,
    player_external_id: playerExternalId.trim() || undefined,
  });
  const activities = data?.data ?? [];

  const onFilter = <T,>(set: (v: T) => void) => (v: T) => {
    set(v);
    pager.reset();
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="p-4">
          <div className="flex flex-col lg:flex-row gap-3">
            <Input
              placeholder="Event type slug"
              value={eventType}
              onChange={e => onFilter(setEventType)(e.target.value)}
              className="lg:max-w-[220px]"
            />
            <Input
              placeholder="Player external id"
              value={playerExternalId}
              onChange={e => onFilter(setPlayerExternalId)(e.target.value)}
              className="lg:max-w-[220px]"
            />
            <Select value={status} onValueChange={v => onFilter(setStatus)(v as typeof status)}>
              <SelectTrigger className="lg:w-[160px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All statuses</SelectItem>
                <SelectItem value="pending">Pending</SelectItem>
                <SelectItem value="decided">Decided</SelectItem>
                <SelectItem value="rejected">Rejected</SelectItem>
              </SelectContent>
            </Select>
            <div className="flex gap-2 lg:ml-auto">
              <Button variant="outline" onClick={() => refetch()} disabled={isFetching}>
                <RefreshCw className={cn('w-4 h-4', isFetching && 'animate-spin')} />
                Refresh
              </Button>
              {canSend && (
                <Button onClick={() => setSendOpen(true)}>
                  <Send className="w-4 h-4" />
                  Send test activity
                </Button>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Event type</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Player</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Outcome</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Occurred</th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  [...Array(5)].map((_, i) => (
                    <tr key={i} className="border-b border-border/50">
                      <td colSpan={5} className="p-4">
                        <Skeleton className="h-5 w-full" />
                      </td>
                    </tr>
                  ))
                ) : error ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center text-destructive">{error.message}</td>
                  </tr>
                ) : activities.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center">
                      <ActivityIcon className="w-8 h-8 text-muted-foreground mx-auto mb-2" />
                      <p className="font-medium">No activities</p>
                      <p className="text-sm text-muted-foreground">
                        Activities appear here when your systems call POST /api/v1/activities.
                      </p>
                    </td>
                  </tr>
                ) : (
                  activities.map(a => (
                    <tr
                      key={a.id}
                      className="border-b border-border/50 hover:bg-secondary/30 transition-colors cursor-pointer"
                      onClick={() => setSelected(a)}
                    >
                      <td className="p-4">
                        <code className="text-sm bg-secondary px-2 py-1 rounded">{a.event_type}</code>
                      </td>
                      <td className="p-4 text-sm">{a.player_external_id ?? '—'}</td>
                      <td className="p-4">
                        <Badge variant="outline" className={cn('capitalize', statusClass[a.status])}>
                          {a.status}
                        </Badge>
                      </td>
                      <td className="p-4 text-sm text-muted-foreground">{a.outcome ?? '—'}</td>
                      <td className="p-4 text-sm text-muted-foreground">{new Date(a.occurred_at).toLocaleString()}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
          <CursorPager
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            nextCursor={data?.next_cursor}
            onPrevious={pager.previous}
            onNext={pager.next}
            isFetching={isFetching}
          />
        </CardContent>
      </Card>

      <Dialog open={!!selected} onOpenChange={o => !o && setSelected(null)}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          {selected && (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <code>{selected.event_type}</code>
                  <Badge variant="outline" className={cn('capitalize', statusClass[selected.status])}>
                    {selected.status}
                  </Badge>
                </DialogTitle>
                <DialogDescription>
                  event_id <code>{selected.event_id}</code> · player {selected.player_external_id ?? '—'}
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 text-sm">
                <div className="grid grid-cols-2 gap-2 text-muted-foreground">
                  <span>Occurred: {new Date(selected.occurred_at).toLocaleString()}</span>
                  <span>Received: {new Date(selected.received_at).toLocaleString()}</span>
                  {selected.decided_at && <span>Decided: {new Date(selected.decided_at).toLocaleString()}</span>}
                  <span>Causation depth: {selected.causation_depth}</span>
                </div>
                {selected.reason && <p className="text-muted-foreground">Reason: {selected.reason}</p>}
                <div>
                  <p className="font-medium mb-1">Properties</p>
                  <pre className="bg-secondary/50 rounded p-3 text-xs overflow-x-auto">
                    {JSON.stringify(selected.properties, null, 2)}
                  </pre>
                </div>
                {Object.keys(selected.context ?? {}).length > 0 && (
                  <div>
                    <p className="font-medium mb-1">Context</p>
                    <pre className="bg-secondary/50 rounded p-3 text-xs overflow-x-auto">
                      {JSON.stringify(selected.context, null, 2)}
                    </pre>
                  </div>
                )}
                <div>
                  <p className="font-medium mb-2">Rules decision</p>
                  {selected.decision_id ? (
                    <DecisionDetail decisionId={selected.decision_id} />
                  ) : (
                    <p className="text-muted-foreground">
                      {selected.status === 'pending' ? 'Not evaluated yet.' : 'No decision recorded.'}
                    </p>
                  )}
                </div>
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>

      <SendActivityDialog open={sendOpen} onOpenChange={setSendOpen} />
    </div>
  );
}

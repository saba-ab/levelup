import { useState } from 'react';
import { GitBranch, RefreshCw, X } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import CursorPager from '@/components/CursorPager';
import { DecisionDetail } from '@/components/RuleTrace';
import { PlayerPicker } from '@/components/players/PlayerPicker';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useRuleDecisionsQuery } from '@/services/queries/rules';
import type { Player, RuleDecision } from '@/services/api/types';

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const outcomeClass = (outcome: string) =>
  outcome === 'matched'
    ? 'border-green-500/50 text-green-600 dark:text-green-400 bg-green-500/10'
    : 'text-muted-foreground';

/** Recorded rules decisions (GET /rules/decisions), newest first, with their trace. */
export function RuleDecisionsLog() {
  const pager = useCursorPagination(25);
  const [player, setPlayer] = useState<Player | null>(null);
  const [activityId, setActivityId] = useState('');
  const [selected, setSelected] = useState<RuleDecision | null>(null);

  const trimmedActivityId = activityId.trim();
  const activityIdInvalid = trimmedActivityId !== '' && !UUID_RE.test(trimmedActivityId);

  const { data, isLoading, isFetching, error, refetch } = useRuleDecisionsQuery(
    {
      limit: pager.limit,
      cursor: pager.cursor,
      player_id: player?.id,
      activity_id: trimmedActivityId && !activityIdInvalid ? trimmedActivityId : undefined,
    },
    !activityIdInvalid,
  );
  const decisions = data?.data ?? [];

  const onPlayer = (p: Player | null) => {
    setPlayer(p);
    pager.reset();
  };
  const onActivityId = (v: string) => {
    setActivityId(v);
    pager.reset();
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="p-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-start">
            <div className="lg:w-[320px]">
              <PlayerPicker value={player} onChange={onPlayer} placeholder="Filter by player..." />
            </div>
            <div className="space-y-1 lg:w-[320px]">
              <div className="relative">
                <Input
                  placeholder="Activity id (UUID)"
                  value={activityId}
                  onChange={e => onActivityId(e.target.value)}
                  aria-invalid={activityIdInvalid}
                  className={cn('font-mono text-xs pr-8', activityIdInvalid && 'border-destructive')}
                />
                {activityId && (
                  <button
                    type="button"
                    aria-label="Clear activity id"
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    onClick={() => onActivityId('')}
                  >
                    <X className="h-4 w-4" />
                  </button>
                )}
              </div>
              {activityIdInvalid && <p className="text-xs text-destructive">Enter a full activity UUID.</p>}
            </div>
            <Button variant="outline" className="lg:ml-auto" onClick={() => refetch()} disabled={isFetching}>
              <RefreshCw className={cn('h-4 w-4', isFetching && 'animate-spin')} />
              Refresh
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Event type</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Player</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Outcome</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Duration</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Evaluated</th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  [...Array(5)].map((_, i) => (
                    <tr key={i} className="border-b border-border/50">
                      <td colSpan={5} className="p-4"><Skeleton className="h-5 w-full" /></td>
                    </tr>
                  ))
                ) : error ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center text-destructive">{error.message}</td>
                  </tr>
                ) : decisions.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center">
                      <GitBranch className="mx-auto mb-2 h-8 w-8 text-muted-foreground" />
                      <p className="font-medium">No rule decisions</p>
                      <p className="text-sm text-muted-foreground">
                        The rules engine records a decision for every activity it evaluates.
                      </p>
                    </td>
                  </tr>
                ) : (
                  decisions.map(d => (
                    <tr
                      key={d.id}
                      className="cursor-pointer border-b border-border/50 transition-colors hover:bg-secondary/30"
                      onClick={() => setSelected(d)}
                    >
                      <td className="p-4"><code className="rounded bg-secondary px-2 py-1 text-sm">{d.event_type}</code></td>
                      <td className="p-4 text-sm">{d.player_external_id ?? '—'}</td>
                      <td className="p-4">
                        <div className="flex flex-col gap-1">
                          <Badge variant="outline" className={cn('w-fit', outcomeClass(d.outcome))}>{d.outcome}</Badge>
                          {d.reason && <span className="text-xs text-muted-foreground">{d.reason}</span>}
                        </div>
                      </td>
                      <td className="p-4 text-sm text-muted-foreground">{d.duration_us} µs</td>
                      <td className="p-4 text-sm text-muted-foreground">{new Date(d.evaluated_at).toLocaleString()}</td>
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
        <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
          {selected && (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <code>{selected.event_type}</code>
                </DialogTitle>
                <DialogDescription className="break-all">
                  activity <code>{selected.activity_id}</code>
                  {selected.event_id && <> · event_id <code>{selected.event_id}</code></>} · player{' '}
                  {selected.player_external_id ?? '—'} · ruleset generation {selected.ruleset_generation}
                </DialogDescription>
              </DialogHeader>
              <DecisionDetail decisionId={selected.id} />
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}

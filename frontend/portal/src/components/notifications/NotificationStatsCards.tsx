import { useMemo, useState } from 'react';
import { CheckCircle2, Clock, Eye, MinusCircle, Send, XCircle } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { StatCard } from '@/components/players/StatCard';
import { useNotificationStatsQuery } from '@/services/queries/notifications';
import { CHANNEL_LABELS } from './templateVariables';

const RANGES = [
  { value: '1', label: 'Last 24 hours' },
  { value: '7', label: 'Last 7 days' },
  { value: '30', label: 'Last 30 days' },
  { value: '90', label: 'Last 90 days' },
  { value: 'all', label: 'All time' },
] as const;

/** Sent / delivered / failed / pending / skipped / open rate over a date range. */
export function NotificationStatsCards() {
  const [range, setRange] = useState<string>('30');
  /** `from` is fixed when the range is picked so the query key stays stable. */
  const params = useMemo(() => {
    if (range === 'all') return undefined;
    return { from: new Date(Date.now() - Number(range) * 24 * 3600 * 1000).toISOString() };
  }, [range]);
  const { data, isLoading, isError, error, refetch } = useNotificationStatsQuery(params);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          {data && (
            <>
              {(['in_app', 'email'] as const).map((ch) => (
                <span key={ch} className="mr-4">
                  {CHANNEL_LABELS[ch]}: {(data.by_channel?.[ch]?.sent ?? 0).toLocaleString()} sent
                </span>
              ))}
            </>
          )}
        </p>
        <Select value={range} onValueChange={setRange}>
          <SelectTrigger className="w-[160px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RANGES.map((r) => (
              <SelectItem key={r.value} value={r.value}>{r.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-6">
          {Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className="h-[108px]" />)}
        </div>
      ) : isError || !data ? (
        <div className="flex items-center justify-between rounded-lg border border-destructive/40 bg-destructive/5 p-4 text-sm">
          <span className="text-destructive">Could not load stats: {error?.message}</span>
          <Button variant="outline" size="sm" onClick={() => refetch()}>Retry</Button>
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-6">
          <StatCard icon={<Send className="h-6 w-6" />} iconClassName="text-primary" value={data.sent} label="Sent" />
          <StatCard icon={<CheckCircle2 className="h-6 w-6" />} iconClassName="text-green-500" value={data.delivered} label="Delivered" />
          <StatCard icon={<XCircle className="h-6 w-6" />} iconClassName="text-destructive" value={data.failed} label="Failed" />
          <StatCard icon={<Clock className="h-6 w-6" />} iconClassName="text-amber-500" value={data.pending} label="Pending" />
          <StatCard icon={<MinusCircle className="h-6 w-6" />} iconClassName="text-muted-foreground" value={data.skipped} label="Skipped" />
          <StatCard
            icon={<Eye className="h-6 w-6" />}
            iconClassName="text-purple-500"
            value={`${(data.open_rate * 100).toFixed(1)}%`}
            label={`Open rate (in-app, ${data.read.toLocaleString()} read)`}
          />
        </div>
      )}
    </div>
  );
}

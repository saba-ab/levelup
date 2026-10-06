import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Bell, Inbox, Mail, RefreshCw } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { PlayerPicker } from '@/components/players/PlayerPicker';
import CursorPager from '@/components/CursorPager';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { formatDateTime } from '@/lib/player-utils';
import { useNotificationHistoryQuery, useNotificationTemplatesQuery } from '@/services/queries/notifications';
import {
  NOTIFICATION_CHANNELS,
  NOTIFICATION_STATUSES,
  type NotificationChannel,
  type NotificationStatus,
} from '@/services/api/models/notifications';
import type { Player } from '@/services/api/types';
import { CHANNEL_LABELS, STATUS_STYLES, TRIGGER_LABELS } from './templateVariables';

const ALL = 'all';

/** Delivery history (newest first) with template / status / channel / player filters. */
export function HistoryTab() {
  const pager = useCursorPagination(25);
  const [templateId, setTemplateId] = useState<string>(ALL);
  const [status, setStatus] = useState<string>(ALL);
  const [channel, setChannel] = useState<string>(ALL);
  const [player, setPlayer] = useState<Player | null>(null);

  const templates = useNotificationTemplatesQuery({ limit: 100 });
  const { data, isLoading, isError, error, refetch, isFetching } = useNotificationHistoryQuery({
    template_id: templateId === ALL ? undefined : templateId,
    status: status === ALL ? undefined : (status as NotificationStatus),
    channel: channel === ALL ? undefined : (channel as NotificationChannel),
    player_id: player?.id,
    limit: pager.limit,
    cursor: pager.cursor,
  });
  const rows = data?.data ?? [];
  const hasFilters = templateId !== ALL || status !== ALL || channel !== ALL || !!player;

  const onFilter = <T,>(setter: (v: T) => void) => (v: T) => {
    setter(v);
    pager.reset();
  };

  return (
    <Card>
      <CardHeader className="flex flex-col gap-2 space-y-0 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <CardTitle>History</CardTitle>
          <CardDescription>Every notification rendered for a player, newest first.</CardDescription>
        </div>
        <Button variant="outline" size="sm" className="gap-2" onClick={() => refetch()} disabled={isFetching}>
          <RefreshCw className={isFetching ? 'h-4 w-4 animate-spin' : 'h-4 w-4'} />
          Refresh
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <Select value={templateId} onValueChange={onFilter(setTemplateId)}>
            <SelectTrigger>
              <SelectValue placeholder="Template" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All templates</SelectItem>
              {(templates.data?.data ?? []).map((t) => (
                <SelectItem key={t.id} value={t.id}>{t.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={status} onValueChange={onFilter(setStatus)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All statuses</SelectItem>
              {NOTIFICATION_STATUSES.map((s) => (
                <SelectItem key={s} value={s} className="capitalize">{s}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={channel} onValueChange={onFilter(setChannel)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All channels</SelectItem>
              {NOTIFICATION_CHANNELS.map((c) => (
                <SelectItem key={c} value={c}>{CHANNEL_LABELS[c]}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <PlayerPicker value={player} onChange={onFilter(setPlayer)} placeholder="Filter by player..." />
        </div>

        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }, (_, i) => <Skeleton key={i} className="h-12" />)}
          </div>
        ) : isError ? (
          <div className="flex flex-col items-center gap-3 py-10 text-center">
            <p className="text-sm text-muted-foreground">Could not load history: {error.message}</p>
            <Button variant="outline" size="sm" onClick={() => refetch()}>Retry</Button>
          </div>
        ) : rows.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-12 text-center">
            <Inbox className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{hasFilters ? 'No notifications match these filters' : 'No notifications sent yet'}</p>
            <p className="text-sm text-muted-foreground">
              {hasFilters ? 'Try other filters.' : 'They appear here once an active template fires.'}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Created</TableHead>
                  <TableHead>Template</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Player</TableHead>
                  <TableHead>Channel</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Delivered / read</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((n) => (
                  <TableRow key={n.id}>
                    <TableCell className="whitespace-nowrap text-sm text-muted-foreground">{formatDateTime(n.created_at)}</TableCell>
                    <TableCell>
                      <p className="font-medium">{n.template_name || '—'}</p>
                      <p className="text-xs text-muted-foreground">{TRIGGER_LABELS[n.trigger] ?? n.trigger}</p>
                    </TableCell>
                    <TableCell className="max-w-[320px]">
                      <p className="truncate text-sm font-medium" title={n.title}>{n.title}</p>
                      {n.body && <p className="line-clamp-2 text-xs text-muted-foreground" title={n.body}>{n.body}</p>}
                    </TableCell>
                    <TableCell>
                      <Link to={`/players/${n.player_id}`} className="font-mono text-xs text-primary hover:underline">
                        {n.player_id.slice(0, 8)}…
                      </Link>
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-1.5 whitespace-nowrap text-sm">
                        {n.channel === 'email' ? <Mail className="h-3.5 w-3.5 text-muted-foreground" /> : <Bell className="h-3.5 w-3.5 text-muted-foreground" />}
                        {CHANNEL_LABELS[n.channel] ?? n.channel}
                      </span>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline" className={`capitalize ${STATUS_STYLES[n.status] ?? ''}`}>{n.status}</Badge>
                      {(n.last_error || n.reason) && (
                        <p className="mt-1 max-w-[200px] truncate text-xs text-muted-foreground" title={n.last_error || n.reason}>
                          {n.last_error || n.reason}
                        </p>
                      )}
                      {n.attempts > 1 && <p className="text-xs text-muted-foreground">{n.attempts} attempts</p>}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      <p>{n.delivered_at ? formatDateTime(n.delivered_at) : '—'}</p>
                      {n.read_at && <p className="text-green-600 dark:text-green-400">Read {formatDateTime(n.read_at)}</p>}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

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
  );
}

import { useMemo, useState } from 'react';
import { Loader2, RefreshCw, RotateCcw, Webhook } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import CursorPager from '@/components/CursorPager';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useToast } from '@/hooks/use-toast';
import { DeliveryStatusBadge, WebhookDeliveryDetail } from '@/components/integrations/WebhookDeliveryDetail';
import {
  describeWebhookError,
  useAllWebhookEndpointsQuery,
  useRedeliverWebhookMutation,
  useWebhookDeliveriesQuery,
  useWebhookDeliveryQuery,
  useWebhookEventTypesQuery,
} from '@/services/queries/webhooks';
import { WEBHOOK_TEST_EVENT, type WebhookDeliveryStatus } from '@/services/api/models/webhooks';

const ALL = 'all';

function DeliveryDialog({
  deliveryId,
  urlById,
  canManage,
  onClose,
}: {
  deliveryId: string | null;
  urlById: Map<string, string>;
  canManage: boolean;
  onClose: () => void;
}) {
  const { toast } = useToast();
  const { data, isLoading, error } = useWebhookDeliveryQuery(deliveryId ?? undefined);
  const redeliver = useRedeliverWebhookMutation();

  const onRedeliver = async () => {
    if (!deliveryId) return;
    try {
      await redeliver.mutateAsync(deliveryId);
      toast({ title: 'Redelivery queued', description: 'The same payload is sent again, signed with the current secret.' });
    } catch (err) {
      toast({ title: 'Could not redeliver', description: describeWebhookError(err, 'Request failed'), variant: 'destructive' });
    }
  };

  return (
    <Dialog open={!!deliveryId} onOpenChange={o => !o && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Webhook delivery</DialogTitle>
          <DialogDescription className="break-all">
            {data ? (urlById.get(data.endpoint_id) ?? `Endpoint ${data.endpoint_id}`) : '\u00a0'}
          </DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> Loading delivery…
          </div>
        ) : error || !data ? (
          <p className="text-sm text-destructive">{error?.message ?? 'Delivery not found'}</p>
        ) : (
          <WebhookDeliveryDetail delivery={data} />
        )}
        {canManage && data && (
          <DialogFooter>
            <Button
              variant="outline"
              className="gap-2"
              onClick={onRedeliver}
              disabled={redeliver.isPending || data.status === 'pending'}
              title={data.status === 'pending' ? 'Still being delivered' : undefined}
            >
              {redeliver.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <RotateCcw className="h-4 w-4" />}
              Redeliver
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}

/** Webhook deliveries (GET /webhooks/deliveries) with payload, response and redeliver. */
export function WebhookDeliveriesLog({ canManage }: { canManage: boolean }) {
  const pager = useCursorPagination(25);
  const [status, setStatus] = useState<typeof ALL | WebhookDeliveryStatus>(ALL);
  const [event, setEvent] = useState<string>(ALL);
  const [endpointId, setEndpointId] = useState<string>(ALL);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const endpoints = useAllWebhookEndpointsQuery();
  const eventTypes = useWebhookEventTypesQuery();
  const urlById = useMemo(() => new Map((endpoints.data ?? []).map(e => [e.id, e.url])), [endpoints.data]);

  const { data, isLoading, isFetching, error, refetch } = useWebhookDeliveriesQuery({
    limit: pager.limit,
    cursor: pager.cursor,
    status: status === ALL ? undefined : status,
    event: event === ALL ? undefined : event,
    endpoint_id: endpointId === ALL ? undefined : endpointId,
  });
  const deliveries = data?.data ?? [];

  const onFilter = <T,>(set: (v: T) => void) => (v: T) => {
    set(v);
    pager.reset();
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="p-4">
          <div className="flex flex-col gap-3 lg:flex-row">
            <Select value={endpointId} onValueChange={onFilter(setEndpointId)}>
              <SelectTrigger className="lg:w-[280px]"><SelectValue placeholder="All endpoints" /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All endpoints</SelectItem>
                {(endpoints.data ?? []).map(e => (
                  <SelectItem key={e.id} value={e.id}><span className="font-mono text-xs">{e.url}</span></SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={event} onValueChange={onFilter(setEvent)}>
              <SelectTrigger className="lg:w-[220px]"><SelectValue placeholder="All events" /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All events</SelectItem>
                {(eventTypes.data ?? []).map(t => (
                  <SelectItem key={t.event} value={t.event}>{t.event}</SelectItem>
                ))}
                <SelectItem value={WEBHOOK_TEST_EVENT}>{WEBHOOK_TEST_EVENT}</SelectItem>
              </SelectContent>
            </Select>
            <Select value={status} onValueChange={v => onFilter(setStatus)(v as typeof status)}>
              <SelectTrigger className="lg:w-[160px]"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All statuses</SelectItem>
                <SelectItem value="pending">Pending</SelectItem>
                <SelectItem value="succeeded">Succeeded</SelectItem>
                <SelectItem value="failed">Failed</SelectItem>
              </SelectContent>
            </Select>
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
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Event</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Endpoint</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Status</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Response</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Attempts</th>
                  <th className="p-4 text-left text-sm font-medium text-muted-foreground">Created</th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  [...Array(5)].map((_, i) => (
                    <tr key={i} className="border-b border-border/50">
                      <td colSpan={6} className="p-4"><Skeleton className="h-5 w-full" /></td>
                    </tr>
                  ))
                ) : error ? (
                  <tr>
                    <td colSpan={6} className="p-8 text-center text-destructive">{error.message}</td>
                  </tr>
                ) : deliveries.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="p-8 text-center">
                      <Webhook className="mx-auto mb-2 h-8 w-8 text-muted-foreground" />
                      <p className="font-medium">No webhook deliveries</p>
                      <p className="text-sm text-muted-foreground">
                        Deliveries appear here once an endpoint is subscribed to events (Integrations → Webhooks).
                      </p>
                    </td>
                  </tr>
                ) : (
                  deliveries.map(d => (
                    <tr
                      key={d.id}
                      className="cursor-pointer border-b border-border/50 transition-colors hover:bg-secondary/30"
                      onClick={() => setSelectedId(d.id)}
                    >
                      <td className="p-4"><code className="rounded bg-secondary px-2 py-1 text-sm">{d.event}</code></td>
                      <td className="max-w-64 truncate p-4 font-mono text-xs" title={urlById.get(d.endpoint_id)}>
                        {urlById.get(d.endpoint_id) ?? <span className="text-muted-foreground">{d.endpoint_id}</span>}
                      </td>
                      <td className="p-4">
                        <div className="flex flex-col gap-1">
                          <DeliveryStatusBadge status={d.status} />
                          {d.last_error && (
                            <span className="max-w-56 truncate text-xs text-destructive" title={d.last_error}>{d.last_error}</span>
                          )}
                        </div>
                      </td>
                      <td className="p-4 text-sm text-muted-foreground">
                        {d.response_status != null ? `HTTP ${d.response_status}` : '—'}
                        {d.latency_ms != null && <span className="block text-xs">{d.latency_ms} ms</span>}
                      </td>
                      <td className="p-4 text-sm">{d.attempts}</td>
                      <td className="p-4 text-sm text-muted-foreground">{new Date(d.created_at).toLocaleString()}</td>
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

      <DeliveryDialog
        deliveryId={selectedId}
        urlById={urlById}
        canManage={canManage}
        onClose={() => setSelectedId(null)}
      />
    </div>
  );
}

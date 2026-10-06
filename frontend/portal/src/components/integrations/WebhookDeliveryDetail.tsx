import type { ReactNode } from 'react';
import { CheckCircle2, Clock, XCircle } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import type { WebhookDelivery, WebhookDeliveryStatus } from '@/services/api/models/webhooks';
import { CodeBlock, JsonBlock } from './CodeBlock';

const STATUS_STYLES: Record<WebhookDeliveryStatus, { label: string; className: string; icon: typeof Clock }> = {
  succeeded: { label: 'Succeeded', className: 'border-green-500/40 text-green-600 dark:text-green-400', icon: CheckCircle2 },
  failed: { label: 'Failed', className: 'border-destructive/40 text-destructive', icon: XCircle },
  pending: { label: 'Pending', className: 'border-amber-500/40 text-amber-600 dark:text-amber-400', icon: Clock },
};

export function DeliveryStatusBadge({ status }: { status: WebhookDeliveryStatus }) {
  const s = STATUS_STYLES[status] ?? STATUS_STYLES.pending;
  const Icon = s.icon;
  return (
    <Badge variant="outline" className={cn('gap-1 font-normal', s.className)}>
      <Icon className="h-3 w-3" /> {s.label}
    </Badge>
  );
}

const fmt = (ts: string | null | undefined) => (ts ? new Date(ts).toLocaleString() : '—');

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm break-all">{children}</dd>
    </div>
  );
}

/** Everything recorded about one delivery: outcome, response and the exact payload. */
export function WebhookDeliveryDetail({ delivery, endpointUrl }: { delivery: WebhookDelivery; endpointUrl?: string }) {
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <DeliveryStatusBadge status={delivery.status} />
        <code className="text-xs">{delivery.event}</code>
        {delivery.response_status != null && (
          <Badge variant={delivery.response_status < 300 ? 'secondary' : 'destructive'} className="font-mono">
            HTTP {delivery.response_status}
          </Badge>
        )}
        {delivery.latency_ms != null && <span className="text-xs text-muted-foreground">{delivery.latency_ms} ms</span>}
      </div>

      <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {endpointUrl && <Field label="Endpoint">{endpointUrl}</Field>}
        <Field label="Delivery id"><code className="text-xs">{delivery.id}</code></Field>
        <Field label="Event id"><code className="text-xs">{delivery.event_id}</code></Field>
        <Field label="Attempts">{delivery.attempts}</Field>
        <Field label="Created">{fmt(delivery.created_at)}</Field>
        <Field label="Last attempt">{fmt(delivery.last_attempt_at)}</Field>
        <Field label="Delivered">{fmt(delivery.delivered_at)}</Field>
      </dl>

      {delivery.last_error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive break-all">
          {delivery.last_error}
        </div>
      )}

      {delivery.response_body ? (
        <CodeBlock title="Response body" code={delivery.response_body} className="[&_pre]:max-h-60" />
      ) : (
        <p className="text-xs text-muted-foreground">No response body recorded.</p>
      )}

      {delivery.payload !== undefined && <JsonBlock title="Payload (request body)" value={delivery.payload} />}
    </div>
  );
}

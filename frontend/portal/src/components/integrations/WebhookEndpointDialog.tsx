import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { Skeleton } from '@/components/ui/skeleton';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { useToast } from '@/hooks/use-toast';
import {
  describeWebhookError,
  useCreateWebhookEndpointMutation,
  useUpdateWebhookEndpointMutation,
  useWebhookEventTypesQuery,
} from '@/services/queries/webhooks';
import { ApiRequestError } from '@/services/queries/rules';
import {
  WEBHOOK_WILDCARD,
  type UpdateWebhookEndpointData,
  type WebhookEndpoint,
  type WebhookEndpointWithSecret,
} from '@/services/api/models/webhooks';

interface WebhookEndpointDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Edit this endpoint; create a new one when absent. */
  endpoint?: WebhookEndpoint | null;
  /** Create only: the response carries the secret, shown once. */
  onCreated?: (endpoint: WebhookEndpointWithSecret) => void;
}

const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every(x => b.includes(x));

/** Register or edit an endpoint: URL, description and the events it receives. */
export function WebhookEndpointDialog({ open, onOpenChange, endpoint, onCreated }: WebhookEndpointDialogProps) {
  const { toast } = useToast();
  const isEdit = !!endpoint;
  const eventTypes = useWebhookEventTypesQuery(open);
  const createEndpoint = useCreateWebhookEndpointMutation();
  const updateEndpoint = useUpdateWebhookEndpointMutation();

  const [url, setUrl] = useState('');
  const [description, setDescription] = useState('');
  const [allEvents, setAllEvents] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [isActive, setIsActive] = useState(true);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string[]>>({});

  useEffect(() => {
    if (!open) return;
    setUrl(endpoint?.url ?? '');
    setDescription(endpoint?.description ?? '');
    const types = endpoint?.event_types ?? [];
    setAllEvents(types.includes(WEBHOOK_WILDCARD));
    setSelected(types.filter(t => t !== WEBHOOK_WILDCARD));
    setIsActive(endpoint?.is_active ?? true);
    setFieldErrors({});
  }, [open, endpoint]);

  const toggle = (event: string, checked: boolean) =>
    setSelected(s => (checked ? [...s, event] : s.filter(e => e !== event)));

  const eventTypesValue = allEvents ? [WEBHOOK_WILDCARD] : selected;
  const isPending = createEndpoint.isPending || updateEndpoint.isPending;
  const canSubmit = url.trim().length > 0 && eventTypesValue.length > 0 && !isPending;

  const submit = async () => {
    setFieldErrors({});
    try {
      if (endpoint) {
        const patch: UpdateWebhookEndpointData = {};
        if (url.trim() !== endpoint.url) patch.url = url.trim();
        if (description.trim() !== endpoint.description) patch.description = description.trim();
        if (!sameSet(eventTypesValue, endpoint.event_types)) patch.event_types = eventTypesValue;
        if (Object.keys(patch).length === 0) {
          onOpenChange(false);
          return;
        }
        await updateEndpoint.mutateAsync({ id: endpoint.id, data: patch });
        toast({ title: 'Endpoint updated' });
        onOpenChange(false);
      } else {
        const created = await createEndpoint.mutateAsync({
          url: url.trim(),
          description: description.trim() || undefined,
          event_types: eventTypesValue,
          is_active: isActive,
        });
        onOpenChange(false);
        onCreated?.(created);
      }
    } catch (err) {
      if (err instanceof ApiRequestError && err.validationErrors) setFieldErrors(err.validationErrors);
      toast({
        title: isEdit ? 'Could not update endpoint' : 'Could not create endpoint',
        description: describeWebhookError(err, 'Request failed'),
        variant: 'destructive',
      });
    }
  };

  const errorFor = (field: string) =>
    Object.entries(fieldErrors)
      .filter(([k]) => k === field || k.startsWith(`${field}[`))
      .flatMap(([, m]) => m)
      .join(', ');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? 'Edit webhook endpoint' : 'Add webhook endpoint'}</DialogTitle>
          <DialogDescription>
            LevelUp POSTs a signed JSON body to this URL for every subscribed event. It must be https.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="webhook-url">Endpoint URL</Label>
            <Input
              id="webhook-url"
              type="url"
              value={url}
              maxLength={2048}
              placeholder="https://example.com/webhooks/levelup"
              onChange={e => setUrl(e.target.value)}
            />
            {errorFor('url') && <p className="text-xs text-destructive">{errorFor('url')}</p>}
          </div>

          <div className="space-y-2">
            <Label htmlFor="webhook-description">Description</Label>
            <Textarea
              id="webhook-description"
              value={description}
              maxLength={1000}
              rows={2}
              placeholder="What consumes these events"
              onChange={e => setDescription(e.target.value)}
            />
            {errorFor('description') && <p className="text-xs text-destructive">{errorFor('description')}</p>}
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Events</Label>
              <label className="flex items-center gap-2 text-sm">
                <Checkbox checked={allEvents} onCheckedChange={v => setAllEvents(v === true)} />
                All events
              </label>
            </div>
            {eventTypes.isLoading ? (
              <Skeleton className="h-40 w-full" />
            ) : eventTypes.error ? (
              <p className="text-sm text-destructive">{eventTypes.error.message}</p>
            ) : (
              <div className="max-h-56 space-y-1 overflow-y-auto rounded-md border p-2">
                {(eventTypes.data ?? []).map(t => (
                  <label
                    key={t.event}
                    className="flex cursor-pointer items-start gap-2 rounded px-1 py-1 hover:bg-muted/50 has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60"
                  >
                    <Checkbox
                      className="mt-0.5"
                      disabled={allEvents}
                      checked={allEvents || selected.includes(t.event)}
                      onCheckedChange={v => toggle(t.event, v === true)}
                    />
                    <span className="min-w-0">
                      <code className="text-xs font-medium">{t.event}</code>
                      <span className="block text-xs text-muted-foreground">{t.description}</span>
                    </span>
                  </label>
                ))}
              </div>
            )}
            {errorFor('event_types') && <p className="text-xs text-destructive">{errorFor('event_types')}</p>}
            {!allEvents && selected.length === 0 && (
              <p className="text-xs text-muted-foreground">Pick at least one event, or All events.</p>
            )}
          </div>

          {!isEdit && (
            <div className="flex items-center justify-between rounded-md border p-3">
              <div>
                <Label htmlFor="webhook-active">Active</Label>
                <p className="text-xs text-muted-foreground">Inactive endpoints receive nothing until enabled.</p>
              </div>
              <Switch id="webhook-active" checked={isActive} onCheckedChange={setIsActive} />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={submit} disabled={!canSubmit}>
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {isEdit ? 'Save changes' : 'Add endpoint'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

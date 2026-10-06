import { useState } from 'react';
import { AlertTriangle, KeyRound, Loader2, MoreHorizontal, Pencil, Plus, RefreshCw, Send, Trash2, Webhook } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import CursorPager from '@/components/CursorPager';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useToast } from '@/hooks/use-toast';
import {
  describeWebhookError,
  useDeleteWebhookEndpointMutation,
  useRotateWebhookSecretMutation,
  useTestWebhookEndpointMutation,
  useUpdateWebhookEndpointMutation,
  useWebhookEndpointsQuery,
} from '@/services/queries/webhooks';
import {
  WEBHOOK_WILDCARD,
  type WebhookDelivery,
  type WebhookEndpoint,
  type WebhookEndpointWithSecret,
} from '@/services/api/models/webhooks';
import { WebhookEndpointDialog } from './WebhookEndpointDialog';
import { WebhookSecretDialog } from './WebhookSecretDialog';
import { WebhookDeliveryDetail } from './WebhookDeliveryDetail';
import { WebhookSignatureDocs } from './WebhookSignatureDocs';

const MAX_EVENT_CHIPS = 3;

function EventChips({ events }: { events: string[] }) {
  if (events.includes(WEBHOOK_WILDCARD)) return <Badge variant="secondary">All events</Badge>;
  const shown = events.slice(0, MAX_EVENT_CHIPS);
  const rest = events.slice(MAX_EVENT_CHIPS);
  return (
    <div className="flex flex-wrap gap-1">
      {shown.map(e => (
        <Badge key={e} variant="outline" className="font-mono text-[11px] font-normal">{e}</Badge>
      ))}
      {rest.length > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="outline" className="cursor-default text-[11px] font-normal">+{rest.length} more</Badge>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs font-mono text-xs">{rest.join(', ')}</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

function EndpointState({ endpoint }: { endpoint: WebhookEndpoint }) {
  if (endpoint.is_active) {
    return (
      <div className="space-y-1">
        <Badge className="bg-green-500/15 text-green-600 hover:bg-green-500/15 dark:text-green-400">Active</Badge>
        {endpoint.consecutive_failures > 0 && (
          <p className="flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
            <AlertTriangle className="h-3 w-3" /> {endpoint.consecutive_failures} failing in a row
          </p>
        )}
      </div>
    );
  }
  return (
    <div className="space-y-1">
      <Badge variant="secondary">Disabled</Badge>
      {endpoint.disabled_reason && <p className="max-w-56 text-xs text-destructive">{endpoint.disabled_reason}</p>}
      {endpoint.consecutive_failures > 0 && (
        <p className="text-xs text-muted-foreground">{endpoint.consecutive_failures} consecutive failures</p>
      )}
      {endpoint.disabled_at && (
        <p className="text-xs text-muted-foreground">since {new Date(endpoint.disabled_at).toLocaleString()}</p>
      )}
    </div>
  );
}

/** Integrations → Webhooks: endpoints, their health, and the signing docs. */
export function WebhooksPanel({ canManage }: { canManage: boolean }) {
  const { toast } = useToast();
  const pager = useCursorPagination(25);
  const { data, isLoading, isFetching, error, refetch } = useWebhookEndpointsQuery({ limit: pager.limit, cursor: pager.cursor });
  const updateEndpoint = useUpdateWebhookEndpointMutation();
  const deleteEndpoint = useDeleteWebhookEndpointMutation();
  const rotateSecret = useRotateWebhookSecretMutation();
  const testEndpoint = useTestWebhookEndpointMutation();

  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<WebhookEndpoint | null>(null);
  const [secret, setSecret] = useState<{ endpoint: WebhookEndpointWithSecret; reason: 'created' | 'rotated' } | null>(null);
  const [toDelete, setToDelete] = useState<WebhookEndpoint | null>(null);
  const [toRotate, setToRotate] = useState<WebhookEndpoint | null>(null);
  const [testing, setTesting] = useState<WebhookEndpoint | null>(null);
  const [testResult, setTestResult] = useState<WebhookDelivery | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);

  const endpoints = data?.data ?? [];

  const openCreate = () => {
    setEditing(null);
    setEditorOpen(true);
  };
  const openEdit = (e: WebhookEndpoint) => {
    setEditing(e);
    setEditorOpen(true);
  };

  const toggleActive = async (e: WebhookEndpoint, next: boolean) => {
    setTogglingId(e.id);
    try {
      await updateEndpoint.mutateAsync({ id: e.id, data: { is_active: next } });
      toast({
        title: next ? 'Endpoint enabled' : 'Endpoint disabled',
        description: next && e.consecutive_failures > 0 ? 'Its failure streak was reset.' : undefined,
      });
    } catch (err) {
      toast({ title: 'Could not update endpoint', description: describeWebhookError(err, 'Request failed'), variant: 'destructive' });
    } finally {
      setTogglingId(null);
    }
  };

  const sendTest = async (e: WebhookEndpoint) => {
    setTesting(e);
    setTestResult(null);
    try {
      setTestResult(await testEndpoint.mutateAsync(e.id));
    } catch (err) {
      setTesting(null);
      toast({ title: 'Could not send test event', description: describeWebhookError(err, 'Request failed'), variant: 'destructive' });
    }
  };

  const rotate = async () => {
    if (!toRotate) return;
    const target = toRotate;
    setToRotate(null);
    try {
      const res = await rotateSecret.mutateAsync(target.id);
      setSecret({ endpoint: res, reason: 'rotated' });
    } catch (err) {
      toast({ title: 'Could not rotate secret', description: describeWebhookError(err, 'Request failed'), variant: 'destructive' });
    }
  };

  const remove = async () => {
    if (!toDelete) return;
    const target = toDelete;
    setToDelete(null);
    try {
      await deleteEndpoint.mutateAsync(target.id);
      toast({ title: 'Endpoint deleted', description: target.url });
    } catch (err) {
      toast({ title: 'Could not delete endpoint', description: describeWebhookError(err, 'Request failed'), variant: 'destructive' });
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Webhook className="h-5 w-5" /> Webhook endpoints
            </CardTitle>
            <CardDescription className="mt-1">
              Get notified when players earn points, badges, levels and rewards. Failed deliveries are retried with
              backoff; an endpoint that keeps failing is disabled automatically.
            </CardDescription>
          </div>
          {canManage && (
            <Button onClick={openCreate} className="gap-2 shrink-0">
              <Plus className="h-4 w-4" /> Add endpoint
            </Button>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          {!canManage && (
            <p className="text-xs text-muted-foreground">You can view endpoints. Only admins can add or change them.</p>
          )}
          {isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : error ? (
            <div className="flex flex-col items-center gap-2 py-8 text-center">
              <p className="text-sm text-destructive">{error.message}</p>
              <Button variant="outline" size="sm" onClick={() => refetch()}>Retry</Button>
            </div>
          ) : endpoints.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-10 text-center">
              <Webhook className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm font-medium">No webhook endpoints yet</p>
              <p className="text-sm text-muted-foreground">
                {canManage ? 'Add an endpoint to start receiving events.' : 'An admin can add one.'}
              </p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Endpoint</TableHead>
                  <TableHead>Events</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="w-20">Enabled</TableHead>
                  <TableHead className="w-12" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {endpoints.map(e => (
                  <TableRow key={e.id}>
                    <TableCell className="max-w-72">
                      <p className="truncate font-mono text-xs" title={e.url}>{e.url}</p>
                      {e.description && <p className="truncate text-xs text-muted-foreground">{e.description}</p>}
                    </TableCell>
                    <TableCell><EventChips events={e.event_types} /></TableCell>
                    <TableCell><EndpointState endpoint={e} /></TableCell>
                    <TableCell>
                      <Switch
                        checked={e.is_active}
                        disabled={!canManage || togglingId === e.id}
                        aria-label={e.is_active ? `Disable ${e.url}` : `Enable ${e.url}`}
                        onCheckedChange={v => toggleActive(e, v)}
                      />
                    </TableCell>
                    <TableCell>
                      {canManage && (
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" aria-label={`Actions for ${e.url}`}>
                              {testing?.id === e.id && testEndpoint.isPending ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <MoreHorizontal className="h-4 w-4" />
                              )}
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem onClick={() => sendTest(e)} disabled={!e.is_active || testEndpoint.isPending}>
                              <Send className="mr-2 h-4 w-4" /> Send test event
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => openEdit(e)}>
                              <Pencil className="mr-2 h-4 w-4" /> Edit
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => setToRotate(e)}>
                              <KeyRound className="mr-2 h-4 w-4" /> Rotate secret
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem onClick={() => setToDelete(e)} className="text-destructive focus:text-destructive">
                              <Trash2 className="mr-2 h-4 w-4" /> Delete
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
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

      <WebhookSignatureDocs />

      <WebhookEndpointDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        endpoint={editing}
        onCreated={endpoint => setSecret({ endpoint, reason: 'created' })}
      />

      <WebhookSecretDialog endpoint={secret?.endpoint ?? null} reason={secret?.reason ?? 'created'} onClose={() => setSecret(null)} />

      <Dialog open={!!testing} onOpenChange={open => !open && setTesting(null)}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Test delivery</DialogTitle>
            <DialogDescription className="break-all">
              A <code>webhook.test</code> event sent to {testing?.url}
            </DialogDescription>
          </DialogHeader>
          {testResult ? (
            <WebhookDeliveryDetail delivery={testResult} />
          ) : (
            <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" /> Sending…
            </div>
          )}
          <DialogFooter>
            {testing && testResult && (
              <Button variant="outline" className="gap-2" onClick={() => sendTest(testing)} disabled={testEndpoint.isPending}>
                <RefreshCw className="h-4 w-4" /> Send again
              </Button>
            )}
            <Button onClick={() => setTesting(null)}>Close</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!toRotate} onOpenChange={open => !open && setToRotate(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Rotate the signing secret?</AlertDialogTitle>
            <AlertDialogDescription className="break-all">
              Deliveries to {toRotate?.url} will be signed with a new secret straight away. Update your receiver
              before it starts rejecting them.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={rotate}>Rotate secret</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={!!toDelete} onOpenChange={open => !open && setToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this endpoint?</AlertDialogTitle>
            <AlertDialogDescription className="break-all">
              {toDelete?.url} stops receiving events, and its pending deliveries fail. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={remove} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              Delete endpoint
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

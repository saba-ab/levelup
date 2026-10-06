import { useState } from 'react';
import { Building2, Power, PowerOff, Copy } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
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
import CursorPager from '@/components/CursorPager';
import { useToast } from '@/hooks/use-toast';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { cn } from '@/lib/utils';
import { usePlatformTenantsQuery, useSetPlatformTenantActiveMutation } from '@/services/queries/platform';
import type { PlatformTenant } from '@/services/api/models/platform';
import { PlatformGuard } from './PlatformAccess';
import { apiErrorMessage, formatDate } from './platform-utils';

const shortId = (id: string) => `${id.slice(0, 8)}…`;

function TenantsContent() {
  const { toast } = useToast();
  const pager = useCursorPagination(25);
  const { data, isLoading, isFetching, error } = usePlatformTenantsQuery({ limit: pager.limit, cursor: pager.cursor });
  const setActiveMutation = useSetPlatformTenantActiveMutation();
  const [pending, setPending] = useState<PlatformTenant | null>(null);

  const tenants = data?.data ?? [];

  const copyId = async (id: string) => {
    try {
      await navigator.clipboard.writeText(id);
      toast({ title: 'Copied to clipboard' });
    } catch {
      toast({ title: 'Could not copy', variant: 'destructive' });
    }
  };

  const handleConfirm = async () => {
    if (!pending) return;
    const active = !pending.active;
    try {
      await setActiveMutation.mutateAsync({ tenantId: pending.id, active });
      toast({ title: active ? `${pending.name} activated` : `${pending.name} deactivated` });
      setPending(null);
    } catch (err) {
      toast({ title: 'Error', description: apiErrorMessage(err, 'Failed to update tenant'), variant: 'destructive' });
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-3xl font-bold">Tenants</h1>
        <p className="text-muted-foreground mt-1">
          Every organisation on the platform. Deactivating a tenant signs out all of its members.
        </p>
      </div>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-4 space-y-3">
              {[...Array(6)].map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : error ? (
            <div className="p-12 text-center text-destructive">{apiErrorMessage(error, 'Failed to fetch tenants')}</div>
          ) : tenants.length === 0 ? (
            <div className="p-12 text-center">
              <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center mx-auto mb-4">
                <Building2 className="w-8 h-8 text-muted-foreground" />
              </div>
              <h3 className="text-lg font-medium mb-2">No tenants yet</h3>
              <p className="text-muted-foreground">Tenants appear here once organisations register.</p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Tenant</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Owner</TableHead>
                  <TableHead>Timezone</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tenants.map(tenant => (
                  <TableRow key={tenant.id} className={cn(!tenant.active && 'opacity-70')}>
                    <TableCell>
                      <div className="flex items-center gap-3">
                        <div className="w-9 h-9 rounded-lg bg-primary/10 flex items-center justify-center shrink-0">
                          <Building2 className="w-4 h-4 text-primary" />
                        </div>
                        <div className="min-w-0">
                          <p className="font-medium truncate">{tenant.name}</p>
                          <code className="text-xs text-muted-foreground">{tenant.slug}</code>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      {tenant.active ? (
                        <Badge className="bg-success/15 text-success border-success/30 hover:bg-success/15">Active</Badge>
                      ) : (
                        <Badge variant="outline" className="text-muted-foreground">
                          Inactive
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      {tenant.owner_user_id ? (
                        <button
                          type="button"
                          className="inline-flex items-center gap-1 font-mono text-xs text-muted-foreground hover:text-foreground"
                          title={`${tenant.owner_user_id} (click to copy)`}
                          onClick={() => copyId(tenant.owner_user_id!)}
                        >
                          {shortId(tenant.owner_user_id)}
                          <Copy className="w-3 h-3" />
                        </button>
                      ) : (
                        <span className="text-xs text-muted-foreground">No owner</span>
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{tenant.timezone}</TableCell>
                    <TableCell className="text-sm text-muted-foreground" title={tenant.created_at}>
                      {formatDate(tenant.created_at)}
                    </TableCell>
                    <TableCell className="text-right">
                      {tenant.active ? (
                        <Button variant="outline" size="sm" onClick={() => setPending(tenant)}>
                          <PowerOff className="w-4 h-4" />
                          Deactivate
                        </Button>
                      ) : (
                        <Button variant="outline" size="sm" onClick={() => setPending(tenant)}>
                          <Power className="w-4 h-4" />
                          Activate
                        </Button>
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

      <AlertDialog open={!!pending} onOpenChange={o => !o && setPending(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{pending?.active ? 'Deactivate tenant' : 'Activate tenant'}</AlertDialogTitle>
            <AlertDialogDescription>
              {pending?.active
                ? `Deactivate "${pending?.name}"? Every member is signed out immediately and cannot sign in until the tenant is activated again.`
                : `Activate "${pending?.name}"? Its members will be able to sign in again.`}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={setActiveMutation.isPending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={e => {
                e.preventDefault();
                void handleConfirm();
              }}
              disabled={setActiveMutation.isPending}
              className={cn(pending?.active && 'bg-destructive text-destructive-foreground hover:bg-destructive/90')}
            >
              {setActiveMutation.isPending ? 'Saving...' : pending?.active ? 'Deactivate' : 'Activate'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

export default function PlatformTenants() {
  return (
    <PlatformGuard>
      <TenantsContent />
    </PlatformGuard>
  );
}

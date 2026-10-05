import { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
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
import { Alert, AlertDescription } from '@/components/ui/alert';
import { KeyRound, Plus, Copy, Check, Ban, AlertTriangle } from 'lucide-react';
import CursorPager from '@/components/CursorPager';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useToast } from '@/hooks/use-toast';
import { useApiKeysQuery, useCreateApiKeyMutation, useRevokeApiKeyMutation } from '@/services/queries/apiKeys';
import { ApiRequestError } from '@/services/queries/rules';
import { ROLE_IDS, API_KEY_ROLE_KEYS, type ApiKey, type CreatedApiKey, type RoleKey } from '@/services/api/types';

const ROLE_LABELS: Record<string, string> = {
  admin: 'Admin',
  program_manager: 'Program manager',
  developer: 'Developer',
};

const EXPIRY_OPTIONS = [
  { value: 'never', label: 'Never' },
  { value: '30', label: '30 days' },
  { value: '90', label: '90 days' },
  { value: '365', label: '1 year' },
];

function fmt(ts: string | null) {
  return ts ? new Date(ts).toLocaleString() : '—';
}

function keyStatus(k: ApiKey): { label: string; variant: 'default' | 'secondary' | 'destructive' | 'outline' } {
  if (k.revoked_at) return { label: 'Revoked', variant: 'destructive' };
  if (k.expires_at && new Date(k.expires_at) <= new Date()) return { label: 'Expired', variant: 'secondary' };
  return { label: 'Active', variant: 'default' };
}

const ERROR_MESSAGES: Record<string, string> = {
  role_escalation: 'You cannot give a key a role above your own.',
  api_key_role_not_allowed: 'Keys can only be Admin, Program manager or Developer.',
  human_principal_required: 'API keys can only be managed by a signed-in admin.',
};

/** Settings → API Keys: credentials for the tenant's own backends (ADR-0017). */
export default function ApiKeysSettings() {
  const { toast } = useToast();
  const pager = useCursorPagination(25);
  const { data, isLoading, isFetching } = useApiKeysQuery({ limit: pager.limit, cursor: pager.cursor });
  const createKey = useCreateApiKeyMutation();
  const revokeKey = useRevokeApiKeyMutation();

  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState('');
  const [role, setRole] = useState<RoleKey>('developer');
  const [expiry, setExpiry] = useState('never');
  const [created, setCreated] = useState<CreatedApiKey | null>(null);
  const [copied, setCopied] = useState(false);
  const [toRevoke, setToRevoke] = useState<ApiKey | null>(null);

  const describe = (err: unknown, fallback: string) =>
    err instanceof ApiRequestError ? (err.code && ERROR_MESSAGES[err.code]) || err.message : fallback;

  const submit = async () => {
    const expires_at = expiry === 'never' ? undefined : new Date(Date.now() + Number(expiry) * 86_400_000).toISOString();
    try {
      const res = await createKey.mutateAsync({ name: name.trim(), role_ids: [ROLE_IDS[role]], expires_at });
      setCreateOpen(false);
      setName('');
      setCreated(res);
    } catch (err) {
      toast({ title: 'Could not create key', description: describe(err, 'Failed to create API key'), variant: 'destructive' });
    }
  };

  const copySecret = async () => {
    if (!created) return;
    await navigator.clipboard.writeText(created.secret);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const revoke = async () => {
    if (!toRevoke) return;
    try {
      await revokeKey.mutateAsync(toRevoke.id);
      toast({ title: 'Key revoked', description: `"${toRevoke.name}" stops working immediately.` });
    } catch (err) {
      toast({ title: 'Could not revoke key', description: describe(err, 'Failed to revoke API key'), variant: 'destructive' });
    }
    setToRevoke(null);
  };

  const keys = data?.data ?? [];

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
        <div>
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5" /> API Keys
          </CardTitle>
          <CardDescription className="mt-1">
            Credentials for your backends to send activities and call the API. Send as{' '}
            <code className="text-xs">Authorization: Bearer lvl_live_…</code>. Keys cannot manage users or other keys.
          </CardDescription>
        </div>
        <Button onClick={() => setCreateOpen(true)} className="gap-2">
          <Plus className="h-4 w-4" /> Create key
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : keys.length === 0 ? (
          <p className="text-sm text-muted-foreground py-6 text-center">No API keys yet.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Key</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Last used</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.map(k => {
                const status = keyStatus(k);
                return (
                  <TableRow key={k.id}>
                    <TableCell className="font-medium">{k.name}</TableCell>
                    <TableCell><code className="text-xs">lvl_live_{k.prefix}_…</code></TableCell>
                    <TableCell>{k.roles.map(r => r.label).join(', ')}</TableCell>
                    <TableCell><Badge variant={status.variant}>{status.label}</Badge></TableCell>
                    <TableCell className="text-sm text-muted-foreground">{fmt(k.last_used_at)}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{fmt(k.expires_at)}</TableCell>
                    <TableCell>
                      {!k.revoked_at && (
                        <Button variant="ghost" size="icon" aria-label={`Revoke ${k.name}`} onClick={() => setToRevoke(k)}>
                          <Ban className="h-4 w-4 text-destructive" />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
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

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create API key</DialogTitle>
            <DialogDescription>Give it a name you will recognise, e.g. the service that uses it.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="api-key-name">Name</Label>
              <Input id="api-key-name" value={name} maxLength={100} placeholder="Checkout backend" onChange={e => setName(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label>Role</Label>
              <Select value={role} onValueChange={v => setRole(v as RoleKey)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {API_KEY_ROLE_KEYS.map(r => (
                    <SelectItem key={r} value={r}>{ROLE_LABELS[r]}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">Developer is enough to send activities and move points.</p>
            </div>
            <div className="space-y-2">
              <Label>Expires</Label>
              <Select value={expiry} onValueChange={setExpiry}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {EXPIRY_OPTIONS.map(o => (
                    <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button onClick={submit} disabled={!name.trim() || createKey.isPending}>Create key</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!created} onOpenChange={open => !open && setCreated(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Copy your API key</DialogTitle>
            <DialogDescription>"{created?.api_key.name}" is ready.</DialogDescription>
          </DialogHeader>
          <Alert>
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>This is the only time the key is shown. Store it in your secret manager now.</AlertDescription>
          </Alert>
          <div className="flex gap-2">
            <Input readOnly value={created?.secret ?? ''} className="font-mono text-xs" onFocus={e => e.target.select()} />
            <Button variant="outline" size="icon" aria-label="Copy key" onClick={copySecret}>
              {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
            </Button>
          </div>
          <DialogFooter>
            <Button onClick={() => setCreated(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!toRevoke} onOpenChange={open => !open && setToRevoke(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke "{toRevoke?.name}"?</AlertDialogTitle>
            <AlertDialogDescription>
              Requests using this key will be rejected immediately. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={revoke} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              Revoke key
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

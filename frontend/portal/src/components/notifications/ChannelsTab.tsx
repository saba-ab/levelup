import { useEffect, useState } from 'react';
import { Bell, Info, Loader2, Mail, RefreshCw } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Skeleton } from '@/components/ui/skeleton';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { useToast } from '@/hooks/use-toast';
import { formatDateTime } from '@/lib/player-utils';
import { ApiRequestError } from '@/services/queries/rules';
import { useNotificationChannelsQuery, useUpdateNotificationChannelsMutation } from '@/services/queries/notifications';

interface ChannelsTabProps {
  canManage: boolean;
}

/** In-app (always on) and email (toggle + sender name) channel settings. */
export function ChannelsTab({ canManage }: ChannelsTabProps) {
  const { toast } = useToast();
  const { data, isLoading, isError, error, refetch } = useNotificationChannelsQuery();
  const updateMutation = useUpdateNotificationChannelsMutation();
  const [emailEnabled, setEmailEnabled] = useState(false);
  const [fromName, setFromName] = useState('');
  const [fromNameError, setFromNameError] = useState<string | null>(null);

  useEffect(() => {
    if (data) {
      setEmailEnabled(data.email.enabled);
      setFromName(data.email.from_name);
    }
  }, [data]);

  const dirty = !!data && (emailEnabled !== data.email.enabled || fromName !== data.email.from_name);

  const save = () => {
    if (/[<>"\r\n]/.test(fromName)) {
      setFromNameError('The sender name cannot contain <, >, quotes or line breaks.');
      return;
    }
    setFromNameError(null);
    updateMutation.mutate(
      { email: { enabled: emailEnabled, from_name: fromName.trim() } },
      {
        onSuccess: () => toast({ title: 'Channel settings saved' }),
        onError: (err) => {
          const fieldMsg = err instanceof ApiRequestError ? err.validationErrors?.['email.from_name']?.join(', ') ?? err.validationErrors?.from_name?.join(', ') : undefined;
          if (fieldMsg) setFromNameError(fieldMsg);
          toast({ title: 'Could not save channel settings', description: err.message, variant: 'destructive' });
        },
      },
    );
  };

  if (isLoading) {
    return (
      <div className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-48" />
        <Skeleton className="h-48" />
      </div>
    );
  }

  if (isError || !data) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-3 py-10 text-center">
          <p className="text-sm text-muted-foreground">Could not load channel settings: {error?.message}</p>
          <Button variant="outline" size="sm" className="gap-2" onClick={() => refetch()}>
            <RefreshCw className="h-4 w-4" />
            Retry
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="flex items-center gap-2 text-lg">
                <Bell className="h-5 w-5 text-primary" />
                In-app
              </CardTitle>
              <Switch checked disabled aria-label="In-app is always enabled" />
            </div>
            <CardDescription>
              Always on. Players' feeds are served by GET /players/{'{id}'}/notifications for your app to display.
            </CardDescription>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="flex items-center gap-2 text-lg">
                <Mail className="h-5 w-5 text-primary" />
                Email
              </CardTitle>
              <Switch
                checked={emailEnabled}
                onCheckedChange={setEmailEnabled}
                disabled={!canManage || updateMutation.isPending}
                aria-label="Enable email"
              />
            </div>
            <CardDescription>Sends templates that list the email channel to players with an email address.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="from-name">Sender name</Label>
              <Input
                id="from-name"
                value={fromName}
                maxLength={100}
                placeholder="e.g. Acme Rewards"
                disabled={!canManage || updateMutation.isPending}
                onChange={(e) => { setFromName(e.target.value); setFromNameError(null); }}
              />
              {fromNameError && <p className="text-xs text-destructive">{fromNameError}</p>}
            </div>
            <Badge variant="outline" className={emailEnabled ? 'border-green-500/30 text-green-600 dark:text-green-400' : 'text-muted-foreground'}>
              {emailEnabled ? 'Enabled' : 'Disabled'}
            </Badge>
          </CardContent>
        </Card>
      </div>

      <Alert>
        <Info className="h-4 w-4" />
        <AlertDescription>
          Email delivery needs a working mail driver configured on the server. Without one, email notifications stay pending or fail; check the History tab.
        </AlertDescription>
      </Alert>

      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {data.updated_at ? `Last updated ${formatDateTime(data.updated_at)}` : 'Using defaults'}
        </p>
        {canManage ? (
          <Button onClick={save} disabled={!dirty || updateMutation.isPending}>
            {updateMutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
            Save changes
          </Button>
        ) : (
          <p className="text-xs text-muted-foreground">Your role cannot change channel settings.</p>
        )}
      </div>
    </div>
  );
}

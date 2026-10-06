import { Code, KeyRound, Lock, Webhook } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import ApiKeysSettings from '@/components/ApiKeysSettings';
import { WebhooksPanel } from '@/components/integrations/WebhooksPanel';
import { SdkPanel } from '@/components/integrations/SdkPanel';
import { useIntegrationsAccess } from '@/components/integrations/access';

function ApiKeysAdminOnly() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Lock className="h-5 w-5" /> API Keys
        </CardTitle>
        <CardDescription>
          Only owners and admins can create, list or revoke API keys. Ask one to issue a key for your service.
        </CardDescription>
      </CardHeader>
      <CardContent className="text-sm text-muted-foreground">
        Your backend sends the key as <code className="text-xs">Authorization: Bearer lvl_live_…</code>; see the SDKs
        tab for examples.
      </CardContent>
    </Card>
  );
}

/** Integrations: API keys, webhooks and the official SDKs. */
export default function Integrations() {
  const { canManageApiKeys, canManageWebhooks } = useIntegrationsAccess();

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Integrations</h1>
        <p className="text-muted-foreground">
          Connect your backend with API keys, receive events with webhooks, and use the official SDKs
        </p>
      </div>

      <Tabs defaultValue="api-keys" className="space-y-6">
        <TabsList>
          <TabsTrigger value="api-keys" className="gap-2">
            <KeyRound className="h-4 w-4" /> API Keys
          </TabsTrigger>
          <TabsTrigger value="webhooks" className="gap-2">
            <Webhook className="h-4 w-4" /> Webhooks
          </TabsTrigger>
          <TabsTrigger value="sdks" className="gap-2">
            <Code className="h-4 w-4" /> SDKs
          </TabsTrigger>
        </TabsList>

        <TabsContent value="api-keys">
          {canManageApiKeys ? <ApiKeysSettings /> : <ApiKeysAdminOnly />}
        </TabsContent>

        <TabsContent value="webhooks">
          <WebhooksPanel canManage={canManageWebhooks} />
        </TabsContent>

        <TabsContent value="sdks">
          <SdkPanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}

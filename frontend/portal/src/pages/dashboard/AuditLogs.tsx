import { Activity, GitBranch, Webhook } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import ActivityLog from '@/components/ActivityLog';
import { RuleDecisionsLog } from '@/components/audit/RuleDecisionsLog';
import { WebhookDeliveriesLog } from '@/components/audit/WebhookDeliveriesLog';
import { useIntegrationsAccess } from '@/components/integrations/access';

/** Audit logs: ingested activities, rules decisions and webhook deliveries. */
export default function AuditLogs() {
  const { canManageWebhooks } = useIntegrationsAccess();

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Audit Logs</h1>
        <p className="text-muted-foreground">
          What your systems sent, what the rules engine decided, and what LevelUp delivered to your webhooks
        </p>
      </div>

      <Tabs defaultValue="activities" className="space-y-6">
        <TabsList>
          <TabsTrigger value="activities" className="gap-2">
            <Activity className="h-4 w-4" /> Activities
          </TabsTrigger>
          <TabsTrigger value="decisions" className="gap-2">
            <GitBranch className="h-4 w-4" /> Rule decisions
          </TabsTrigger>
          <TabsTrigger value="webhooks" className="gap-2">
            <Webhook className="h-4 w-4" /> Webhook deliveries
          </TabsTrigger>
        </TabsList>

        <TabsContent value="activities">
          <ActivityLog canSend={false} />
        </TabsContent>

        <TabsContent value="decisions">
          <RuleDecisionsLog />
        </TabsContent>

        <TabsContent value="webhooks">
          <WebhookDeliveriesLog canManage={canManageWebhooks} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

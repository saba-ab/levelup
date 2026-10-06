import { Bell, FileText, History, Settings2 } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useAuth } from '@/contexts/AuthContext';
import { NotificationStatsCards } from '@/components/notifications/NotificationStatsCards';
import { TemplatesTab } from '@/components/notifications/TemplatesTab';
import { ChannelsTab } from '@/components/notifications/ChannelsTab';
import { HistoryTab } from '@/components/notifications/HistoryTab';

/**
 * Notification templates, channel settings, delivery history and stats.
 * Backend permissions: notifications:view_any / create / update / delete /
 * channels_manage; the portal maps them to view:/manage:notifications.
 */
export default function Notifications() {
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:notifications');

  return (
    <div className="space-y-6 animate-fade-in">
      <div>
        <h1 className="flex items-center gap-3 text-3xl font-bold">
          <Bell className="h-8 w-8 text-primary" />
          Notifications
        </h1>
        <p className="mt-1 text-muted-foreground">
          Notify players in-app and by email when they earn badges, level up, complete missions and more.
        </p>
      </div>

      <NotificationStatsCards />

      <Tabs defaultValue="templates" className="space-y-4">
        <TabsList>
          <TabsTrigger value="templates" className="gap-2">
            <FileText className="h-4 w-4" />
            Templates
          </TabsTrigger>
          <TabsTrigger value="channels" className="gap-2">
            <Settings2 className="h-4 w-4" />
            Channels
          </TabsTrigger>
          <TabsTrigger value="history" className="gap-2">
            <History className="h-4 w-4" />
            History
          </TabsTrigger>
        </TabsList>
        <TabsContent value="templates">
          <TemplatesTab canCreate={canManage} canUpdate={canManage} canDelete={canManage} />
        </TabsContent>
        <TabsContent value="channels">
          <ChannelsTab canManage={canManage} />
        </TabsContent>
        <TabsContent value="history">
          <HistoryTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}

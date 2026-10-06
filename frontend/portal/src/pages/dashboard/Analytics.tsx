import { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { DateRangeControl } from '@/components/analytics/DateRangeControl';
import { OverviewTab } from '@/components/analytics/OverviewTab';
import { EngagementTab } from '@/components/analytics/EngagementTab';
import { RetentionTab } from '@/components/analytics/RetentionTab';
import { FunnelTab } from '@/components/analytics/FunnelTab';
import { DEFAULT_RANGE_DAYS, lastDays, type DayRange } from '@/components/analytics/dateRange';

type AnalyticsTab = 'overview' | 'engagement' | 'retention' | 'funnel';

export default function Analytics() {
  const [tab, setTab] = useState<AnalyticsTab>('overview');
  const [range, setRange] = useState<DayRange>(() => lastDays(DEFAULT_RANGE_DAYS));
  const [funnelSteps, setFunnelSteps] = useState<string[]>([]);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h1 className="text-3xl font-bold text-foreground">Analytics</h1>
          <p className="mt-1 text-muted-foreground">Activity, engagement, retention and conversion across your program</p>
        </div>
        {tab !== 'retention' && <DateRangeControl value={range} onChange={setRange} />}
      </div>

      <Tabs value={tab} onValueChange={(v) => setTab(v as AnalyticsTab)} className="flex flex-col gap-6">
        <TabsList className="w-fit">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="engagement">Engagement</TabsTrigger>
          <TabsTrigger value="retention">Retention</TabsTrigger>
          <TabsTrigger value="funnel">Funnel</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="mt-0">
          <OverviewTab range={range} />
        </TabsContent>
        <TabsContent value="engagement" className="mt-0">
          <EngagementTab range={range} />
        </TabsContent>
        <TabsContent value="retention" className="mt-0">
          <RetentionTab />
        </TabsContent>
        <TabsContent value="funnel" className="mt-0">
          <FunnelTab range={range} steps={funnelSteps} onStepsChange={setFunnelSteps} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

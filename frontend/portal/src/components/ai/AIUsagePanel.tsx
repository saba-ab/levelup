import { useMemo } from 'react';
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Activity, Cpu, Gauge, Hash } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { StatCard } from '@/components/players/StatCard';
import type { AIUsageReport } from '@/services/api/models/ai';
import { timeUntilQuotaReset } from '@/services/queries/ai';

interface AIUsagePanelProps {
  usage: AIUsageReport | undefined;
  isLoading: boolean;
  days: number;
}

/** Fills the days the API omits (no usage) so the chart has a continuous axis, oldest first. */
function dailySeries(usage: AIUsageReport, days: number) {
  const byDay = new Map(usage.data.map((d) => [d.day, d]));
  const today = new Date();
  return Array.from({ length: days }, (_, i) => {
    const d = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() - (days - 1 - i)));
    const key = d.toISOString().slice(0, 10);
    const row = byDay.get(key);
    return { day: key.slice(5), requests: row?.requests ?? 0, tokens: (row?.input_tokens ?? 0) + (row?.output_tokens ?? 0) };
  });
}

/** Quota stat cards plus requests per day. */
export function AIUsagePanel({ usage, isLoading, days }: AIUsagePanelProps) {
  const series = useMemo(() => (usage ? dailySeries(usage, days) : []), [usage, days]);

  if (isLoading || !usage) {
    return (
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-[108px]" />
        ))}
      </div>
    );
  }

  const unlimited = usage.daily_limit === 0;
  const todayTokens = usage.today.input_tokens + usage.today.output_tokens;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard icon={<Activity className="h-6 w-6" />} iconClassName="text-primary" value={usage.today.requests} label="Requests today" />
        <StatCard
          icon={<Gauge className="h-6 w-6" />}
          iconClassName="text-green-500"
          value={unlimited ? 'Unlimited' : `${Math.max(usage.remaining, 0).toLocaleString()} / ${usage.daily_limit.toLocaleString()}`}
          label={unlimited ? 'Daily quota' : `Remaining today · resets in ${timeUntilQuotaReset()}`}
        />
        <StatCard icon={<Hash className="h-6 w-6" />} iconClassName="text-amber-500" value={todayTokens} label="Tokens today" />
        <StatCard icon={<Cpu className="h-6 w-6" />} iconClassName="text-purple-500" value={usage.model || '—'} label="Model" className="[&_p:first-of-type]:text-base [&_p:first-of-type]:truncate" />
      </div>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Requests per day</CardTitle>
          <CardDescription>Last {days} days (UTC)</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="h-[180px]">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={series}>
                <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
                <XAxis dataKey="day" stroke="hsl(var(--muted-foreground))" fontSize={11} interval="preserveStartEnd" />
                <YAxis stroke="hsl(var(--muted-foreground))" fontSize={11} allowDecimals={false} width={32} />
                <Tooltip
                  contentStyle={{
                    backgroundColor: 'hsl(var(--card))',
                    border: '1px solid hsl(var(--border))',
                    borderRadius: '8px',
                  }}
                />
                <Bar dataKey="requests" name="Requests" fill="hsl(var(--primary))" radius={[3, 3, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

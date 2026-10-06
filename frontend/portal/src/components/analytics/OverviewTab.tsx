import { useMemo } from 'react';
import { Activity, Award, Coins, Gift, Target, TrendingUp, UserPlus, Users } from 'lucide-react';
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { describeAnalyticsError, useAnalyticsOverviewQuery } from '@/services/queries/analytics';
import { EmptyChart, KpiCard, TabState } from './AnalyticsCards';
import { SERIES_COLORS, axisProps, gridProps, tooltipProps } from './chartTheme';
import { shortDay, type DayRange } from './dateRange';

export function OverviewTab({ range }: { range: DayRange }) {
  const { data, isLoading, error } = useAnalyticsOverviewQuery(range);
  const daily = useMemo(() => (data?.daily ?? []).map((d) => ({ ...d, label: shortDay(d.day) })), [data]);

  if (!data) {
    return <TabState loading={isLoading} error={describeAnalyticsError(error, 'Failed to load the overview.')} />;
  }

  const t = data.totals;
  const hasActivity = daily.some((d) => d.activities > 0 || d.active_players > 0 || d.new_players > 0);
  const hasPoints = daily.some((d) => d.points_credited > 0 || d.points_debited > 0);
  const completionRate = t.missions_started > 0 ? Math.round((t.missions_completed / t.missions_started) * 100) : null;

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 sm:grid-cols-3">
        <KpiCard label="Daily active players" value={data.dau} hint={`On ${shortDay(data.to)}`} icon={<Users className="h-5 w-5" />} />
        <KpiCard label="Weekly active players" value={data.wau} hint="7 days ending on the last day" icon={<Users className="h-5 w-5" />} />
        <KpiCard label="Monthly active players" value={data.mau} hint="30 days ending on the last day" icon={<Users className="h-5 w-5" />} />
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <KpiCard label="Activities" value={t.activities} icon={<Activity className="h-5 w-5" />} />
        <KpiCard label="Active players" value={t.active_players} hint="Distinct in range" icon={<TrendingUp className="h-5 w-5" />} />
        <KpiCard label="New players" value={t.new_players} icon={<UserPlus className="h-5 w-5" />} />
        <KpiCard
          label="Points credited / debited"
          value={`${t.points_credited.toLocaleString()} / ${t.points_debited.toLocaleString()}`}
          icon={<Coins className="h-5 w-5" />}
        />
        <KpiCard label="Badges awarded" value={t.badges_awarded} icon={<Award className="h-5 w-5" />} />
        <KpiCard
          label="Missions completed"
          value={t.missions_completed}
          hint={`${t.missions_started.toLocaleString()} started${completionRate !== null ? ` · ${completionRate}% completed` : ''}`}
          icon={<Target className="h-5 w-5" />}
        />
        <KpiCard label="Levels reached" value={t.levels_reached} icon={<TrendingUp className="h-5 w-5" />} />
        <KpiCard label="Rewards claimed" value={t.rewards_claimed} icon={<Gift className="h-5 w-5" />} />
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Activity</CardTitle>
            <CardDescription>Activities and distinct active players per day</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="h-[300px]">
              {hasActivity ? (
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={daily}>
                    <CartesianGrid {...gridProps} />
                    <XAxis dataKey="label" {...axisProps} minTickGap={16} />
                    <YAxis allowDecimals={false} {...axisProps} />
                    <Tooltip {...tooltipProps} />
                    <Legend />
                    <Line type="monotone" dataKey="activities" name="Activities" stroke={SERIES_COLORS[0]} strokeWidth={2} dot={false} />
                    <Line type="monotone" dataKey="active_players" name="Active players" stroke={SERIES_COLORS[1]} strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              ) : (
                <EmptyChart />
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-lg">New players</CardTitle>
            <CardDescription>Players created per day</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="h-[300px]">
              {hasActivity ? (
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart data={daily}>
                    <CartesianGrid {...gridProps} />
                    <XAxis dataKey="label" {...axisProps} minTickGap={16} />
                    <YAxis allowDecimals={false} {...axisProps} />
                    <Tooltip {...tooltipProps} cursor={{ fill: 'hsl(var(--muted))' }} />
                    <Bar dataKey="new_players" name="New players" fill={SERIES_COLORS[4]} radius={[4, 4, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              ) : (
                <EmptyChart />
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-lg">Points flow</CardTitle>
            <CardDescription>Points credited and debited per day</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="h-[300px]">
              {hasPoints ? (
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={daily}>
                    <CartesianGrid {...gridProps} />
                    <XAxis dataKey="label" {...axisProps} minTickGap={16} />
                    <YAxis allowDecimals={false} {...axisProps} />
                    <Tooltip {...tooltipProps} />
                    <Legend />
                    <Area type="monotone" dataKey="points_credited" name="Credited" stroke={SERIES_COLORS[1]} fill={SERIES_COLORS[1]} fillOpacity={0.2} />
                    <Area type="monotone" dataKey="points_debited" name="Debited" stroke={SERIES_COLORS[3]} fill={SERIES_COLORS[3]} fillOpacity={0.2} />
                  </AreaChart>
                </ResponsiveContainer>
              ) : (
                <EmptyChart message="No points moved in this range." />
              )}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

import { useMemo } from 'react';
import { Award, Gift, Target, TrendingUp } from 'lucide-react';
import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useAllBadgesQuery } from '@/services/queries/mechanics';
import { useEventsQuery } from '@/services/queries/events';
import {
  describeAnalyticsError,
  useAllMissionsForAnalyticsQuery,
  useAnalyticsEngagementQuery,
} from '@/services/queries/analytics';
import { EmptyChart, KpiCard, TabState } from './AnalyticsCards';
import { SERIES_COLORS, axisProps, formatNumber, gridProps, tooltipProps } from './chartTheme';
import { shortDay, type DayRange } from './dateRange';

interface RankedRow {
  key: string;
  name: string;
  count: number;
}

function RankedList({ title, description, rows }: { title: string; description: string; rows: RankedRow[] }) {
  const max = rows[0]?.count ?? 0;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        {rows.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">Nothing in this range.</p>
        ) : (
          <ul className="flex flex-col gap-3">
            {rows.map((r) => (
              <li key={r.key} className="flex flex-col gap-1">
                <div className="flex items-center justify-between gap-2 text-sm">
                  <span className="truncate" title={r.name}>
                    {r.name}
                  </span>
                  <span className="font-medium tabular-nums">{formatNumber(r.count)}</span>
                </div>
                <div className="h-1.5 rounded-full bg-muted">
                  <div
                    className="h-1.5 rounded-full bg-primary"
                    style={{ width: `${max > 0 ? Math.max(2, (r.count / max) * 100) : 0}%` }}
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

export function EngagementTab({ range }: { range: DayRange }) {
  const { data, isLoading, error } = useAnalyticsEngagementQuery(range);
  const badgesQuery = useAllBadgesQuery();
  const missionsQuery = useAllMissionsForAnalyticsQuery();
  const eventsQuery = useEventsQuery();

  const badgeNames = useMemo(() => new Map((badgesQuery.data ?? []).map((b) => [b.id, b.name])), [badgesQuery.data]);
  const missionNames = useMemo(
    () => new Map((missionsQuery.data ?? []).map((m) => [m.id, m.name])),
    [missionsQuery.data],
  );
  const eventNames = useMemo(() => new Map((eventsQuery.data ?? []).map((e) => [e.slug, e.name])), [eventsQuery.data]);
  const daily = useMemo(() => (data?.daily ?? []).map((d) => ({ ...d, label: shortDay(d.day) })), [data]);

  if (!data) {
    return <TabState loading={isLoading} error={describeAnalyticsError(error, 'Failed to load engagement.')} />;
  }

  const badgesAwarded = data.badges_per_day.reduce((n, d) => n + d.count, 0);
  const hasDaily = daily.some(
    (d) => d.badges_awarded || d.missions_started || d.missions_completed || d.levels_reached || d.rewards_claimed,
  );
  const levels = [...data.levels_by_number].sort((a, b) => Number(a.level_number) - Number(b.level_number));

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <KpiCard label="Badges awarded" value={badgesAwarded} icon={<Award className="h-5 w-5" />} />
        <KpiCard
          label="Missions completed"
          value={data.missions.completed}
          hint={`${formatNumber(data.missions.started)} started`}
          icon={<Target className="h-5 w-5" />}
        />
        <KpiCard label="Levels reached" value={data.levels_reached} icon={<TrendingUp className="h-5 w-5" />} />
        <KpiCard label="Rewards claimed" value={data.rewards_claimed} icon={<Gift className="h-5 w-5" />} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Daily engagement</CardTitle>
          <CardDescription>Badges, missions, levels and rewards per day</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="h-[320px]">
            {hasDaily ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={daily}>
                  <CartesianGrid {...gridProps} />
                  <XAxis dataKey="label" {...axisProps} minTickGap={16} />
                  <YAxis allowDecimals={false} {...axisProps} />
                  <Tooltip {...tooltipProps} cursor={{ fill: 'hsl(var(--muted))' }} />
                  <Legend />
                  <Bar dataKey="badges_awarded" name="Badges" stackId="e" fill={SERIES_COLORS[0]} />
                  <Bar dataKey="missions_completed" name="Missions completed" stackId="e" fill={SERIES_COLORS[1]} />
                  <Bar dataKey="levels_reached" name="Levels reached" stackId="e" fill={SERIES_COLORS[2]} />
                  <Bar dataKey="rewards_claimed" name="Rewards claimed" stackId="e" fill={SERIES_COLORS[3]} />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <EmptyChart />
            )}
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-6 lg:grid-cols-3">
        <RankedList
          title="Top event types"
          description="Most frequent activities"
          rows={data.top_event_types.map((e) => ({
            key: e.event_type,
            name: eventNames.get(e.event_type) ?? e.event_type,
            count: e.count,
          }))}
        />
        <RankedList
          title="Top badges"
          description="Most awarded badges"
          rows={data.top_badges.map((b) => ({
            key: b.badge_id,
            name: badgeNames.get(b.badge_id) ?? (badgesQuery.isLoading ? '…' : 'Deleted badge'),
            count: b.count,
          }))}
        />
        <RankedList
          title="Top missions"
          description="Most completed missions"
          rows={data.top_missions.map((m) => ({
            key: m.mission_id,
            name: missionNames.get(m.mission_id) ?? (missionsQuery.isLoading ? '…' : 'Deleted mission'),
            count: m.count,
          }))}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Levels reached</CardTitle>
          <CardDescription>Players reaching each level in this range</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="h-[260px]">
            {levels.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={levels}>
                  <CartesianGrid {...gridProps} />
                  <XAxis dataKey="level_number" {...axisProps} tickFormatter={(v) => `L${v}`} />
                  <YAxis allowDecimals={false} {...axisProps} />
                  <Tooltip {...tooltipProps} cursor={{ fill: 'hsl(var(--muted))' }} labelFormatter={(v) => `Level ${v}`} />
                  <Bar dataKey="count" name="Players" fill={SERIES_COLORS[2]} radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <EmptyChart message="No levels reached in this range." />
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

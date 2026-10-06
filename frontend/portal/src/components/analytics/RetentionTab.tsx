import { useState } from 'react';
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { describeAnalyticsError, useAnalyticsRetentionQuery } from '@/services/queries/analytics';
import type { AnalyticsCohort } from '@/services/api/models/analytics';
import { EmptyChart, TabState } from './AnalyticsCards';
import { SERIES_COLORS, axisProps, formatNumber, formatPct, gridProps, tooltipProps } from './chartTheme';
import { shortDay } from './dateRange';

const WEEK_OPTIONS = [4, 8, 12, 26, 52] as const;

function CohortHeatmap({ cohorts }: { cohorts: AnalyticsCohort[] }) {
  const columns = Math.max(0, ...cohorts.map((c) => c.retained.length));
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border">
            <th className="px-3 py-2 text-left font-medium text-muted-foreground">Cohort (week of)</th>
            <th className="px-3 py-2 text-right font-medium text-muted-foreground">Players</th>
            {Array.from({ length: columns }).map((_, k) => (
              <th key={k} className="px-2 py-2 text-center font-medium text-muted-foreground">
                W{k}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {cohorts.map((c) => (
            <tr key={c.cohort_start} className="border-b border-border/50">
              <td className="whitespace-nowrap px-3 py-2 font-medium">{shortDay(c.cohort_start)}</td>
              <td className="px-3 py-2 text-right tabular-nums">{formatNumber(c.size)}</td>
              {Array.from({ length: columns }).map((_, k) => {
                const pct = c.retained[k];
                return (
                  <td key={k} className="px-1 py-1 text-center">
                    {pct === undefined || c.size === 0 ? (
                      <span className="text-muted-foreground">–</span>
                    ) : (
                      <div
                        className="mx-auto min-w-[3rem] rounded px-1 py-1 text-xs font-medium tabular-nums"
                        style={{
                          backgroundColor: `hsl(var(--primary) / ${Math.max(0.06, pct / 100)})`,
                          color: pct > 50 ? 'hsl(var(--primary-foreground))' : 'hsl(var(--foreground))',
                        }}
                        title={`${formatPct(pct)} of ${formatNumber(c.size)} players active in week ${k}`}
                      >
                        {formatPct(pct)}
                      </div>
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function RetentionTab() {
  const [weeks, setWeeks] = useState<number>(8);
  const { data, isLoading, error, isFetching } = useAnalyticsRetentionQuery({ cohort: 'week', weeks });

  const header = (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-sm text-muted-foreground">
        Weekly cohorts (ISO weeks, UTC) by each player's first activity. The date range does not apply here.
      </p>
      <Select value={String(weeks)} onValueChange={(v) => setWeeks(Number(v))}>
        <SelectTrigger className="w-[150px]" aria-label="Number of cohorts">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {WEEK_OPTIONS.map((w) => (
            <SelectItem key={w} value={String(w)}>
              Last {w} weeks
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );

  if (!data) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <TabState loading={isLoading} error={describeAnalyticsError(error, 'Failed to load retention.')} />
      </div>
    );
  }

  const hasCohorts = data.cohorts.some((c) => c.size > 0);
  const curve = data.curve.map((p) => ({ ...p, label: `W${p.week}` }));

  return (
    <div className={`flex flex-col gap-6 ${isFetching ? 'opacity-70' : ''}`}>
      {header}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Retention curve</CardTitle>
          <CardDescription>Size-weighted average share of a cohort active N weeks after its first week</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="h-[280px]">
            {hasCohorts && curve.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={curve}>
                  <CartesianGrid {...gridProps} />
                  <XAxis dataKey="label" {...axisProps} />
                  <YAxis domain={[0, 100]} tickFormatter={(v) => `${v}%`} {...axisProps} />
                  <Tooltip
                    {...tooltipProps}
                    formatter={(value: number, _name, item) => [
                      `${formatPct(value)} (${item.payload.cohorts} cohorts)`,
                      'Retained',
                    ]}
                  />
                  <Line type="monotone" dataKey="pct" stroke={SERIES_COLORS[0]} strokeWidth={3} dot={{ r: 3 }} />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <EmptyChart message="No player activity in these weeks yet." />
            )}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Cohorts</CardTitle>
          <CardDescription>Share of each cohort active in later weeks (W0 is the first week)</CardDescription>
        </CardHeader>
        <CardContent>
          {hasCohorts ? (
            <CohortHeatmap cohorts={data.cohorts} />
          ) : (
            <p className="py-8 text-center text-sm text-muted-foreground">No cohorts yet.</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

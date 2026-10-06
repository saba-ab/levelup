import { useMemo } from 'react';
import { ArrowDown, ArrowUp, Info, Plus, X } from 'lucide-react';
import { Bar, BarChart, CartesianGrid, LabelList, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { useEventsQuery } from '@/services/queries/events';
import { describeAnalyticsError, useAnalyticsFunnelQuery } from '@/services/queries/analytics';
import { SERIES_COLORS, axisProps, formatNumber, formatPct, gridProps, tooltipProps } from './chartTheme';
import type { DayRange } from './dateRange';

const MIN_STEPS = 2;
const MAX_STEPS = 10;

interface FunnelTabProps {
  range: DayRange;
  /** Kept by the page so the selection survives tab switches. */
  steps: string[];
  onStepsChange: (steps: string[]) => void;
}

export function FunnelTab({ range, steps, onStepsChange: setSteps }: FunnelTabProps) {
  const eventsQuery = useEventsQuery();
  const params = useMemo(() => ({ ...range, steps }), [range, steps]);
  const funnelQuery = useAnalyticsFunnelQuery(params);

  const events = useMemo(() => eventsQuery.data ?? [], [eventsQuery.data]);
  const nameOf = useMemo(() => {
    const names = new Map(events.map((e) => [e.slug, e.name]));
    return (slug: string) => names.get(slug) ?? slug;
  }, [events]);
  const available = events.filter((e) => !steps.includes(e.slug));

  const move = (i: number, delta: number) => {
    const next = [...steps];
    [next[i], next[i + delta]] = [next[i + delta], next[i]];
    setSteps(next);
  };

  const chartData = (funnelQuery.data?.steps ?? []).map((s, i) => ({
    ...s,
    label: `${i + 1}. ${nameOf(s.event_type)}`,
  }));

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Steps</CardTitle>
          <CardDescription>
            Choose {MIN_STEPS} to {MAX_STEPS} event types in the order players should do them.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {steps.length === 0 && <p className="text-sm text-muted-foreground">No steps yet.</p>}
          <ol className="flex flex-col gap-2">
            {steps.map((slug, i) => (
              <li key={slug} className="flex items-center gap-2 rounded-md border border-border px-3 py-2">
                <span className="w-6 text-sm font-medium text-muted-foreground">{i + 1}.</span>
                <span className="flex-1 truncate text-sm">
                  {nameOf(slug)} <span className="font-mono text-xs text-muted-foreground">{slug}</span>
                </span>
                <Button variant="ghost" size="icon" className="h-7 w-7" disabled={i === 0} onClick={() => move(i, -1)} aria-label="Move up">
                  <ArrowUp className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  disabled={i === steps.length - 1}
                  onClick={() => move(i, 1)}
                  aria-label="Move down"
                >
                  <ArrowDown className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  onClick={() => setSteps(steps.filter((s) => s !== slug))}
                  aria-label="Remove step"
                >
                  <X className="h-4 w-4" />
                </Button>
              </li>
            ))}
          </ol>
          {steps.length < MAX_STEPS && (
            <div className="flex items-center gap-2">
              <Plus className="h-4 w-4 text-muted-foreground" />
              <Select value="" onValueChange={(slug) => setSteps([...steps, slug])} disabled={eventsQuery.isLoading}>
                <SelectTrigger className="w-[280px]" aria-label="Add step">
                  <SelectValue placeholder={eventsQuery.isLoading ? 'Loading event types…' : 'Add a step'} />
                </SelectTrigger>
                <SelectContent>
                  {available.length === 0 && (
                    <div className="px-2 py-1.5 text-sm text-muted-foreground">No more event types</div>
                  )}
                  {available.map((e) => (
                    <SelectItem key={e.id} value={e.slug}>
                      {e.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
          {eventsQuery.error && <p className="text-sm text-destructive">Event types could not be loaded.</p>}
        </CardContent>
      </Card>

      <Alert>
        <Info className="h-4 w-4" />
        <AlertDescription>
          Step order is approximated per UTC day: steps a player did on the same day count as done in order.
        </AlertDescription>
      </Alert>

      {steps.length < MIN_STEPS ? (
        <Card>
          <CardContent className="py-12 text-center text-sm text-muted-foreground">
            Add at least {MIN_STEPS} steps to see the funnel.
          </CardContent>
        </Card>
      ) : funnelQuery.isLoading ? (
        <Skeleton className="h-[360px]" />
      ) : funnelQuery.error && !funnelQuery.data ? (
        <Card>
          <CardContent className="py-12 text-center text-sm text-destructive">
            {describeAnalyticsError(funnelQuery.error, 'Failed to load the funnel.')}
          </CardContent>
        </Card>
      ) : (
        <Card className={funnelQuery.isFetching ? 'opacity-70' : ''}>
          <CardHeader>
            <CardTitle className="text-lg">Conversion</CardTitle>
            <CardDescription>Players reaching each step</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-6">
            <div style={{ height: Math.max(200, chartData.length * 56) }}>
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={chartData} layout="vertical" margin={{ left: 8, right: 48 }}>
                  <CartesianGrid {...gridProps} horizontal={false} />
                  <XAxis type="number" allowDecimals={false} {...axisProps} />
                  <YAxis type="category" dataKey="label" width={180} {...axisProps} />
                  <Tooltip
                    {...tooltipProps}
                    cursor={{ fill: 'hsl(var(--muted))' }}
                    formatter={(value: number) => [formatNumber(value), 'Players']}
                  />
                  <Bar dataKey="players" fill={SERIES_COLORS[0]} radius={[0, 4, 4, 0]}>
                    <LabelList
                      dataKey="pct_of_first_step"
                      position="right"
                      formatter={(v: number) => formatPct(v)}
                      className="fill-muted-foreground text-xs"
                    />
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Step</TableHead>
                  <TableHead className="text-right">Players</TableHead>
                  <TableHead className="text-right">From previous</TableHead>
                  <TableHead className="text-right">From first step</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {chartData.map((s, i) => (
                  <TableRow key={s.event_type}>
                    <TableCell>{s.label}</TableCell>
                    <TableCell className="text-right tabular-nums">{formatNumber(s.players)}</TableCell>
                    <TableCell className="text-right tabular-nums">{i === 0 ? '—' : formatPct(s.pct_of_previous)}</TableCell>
                    <TableCell className="text-right tabular-nums">{formatPct(s.pct_of_first_step)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

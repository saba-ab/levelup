import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { Zap, Users, GitBranch, Coins, ArrowRight, FileText, Send, Plus, Activity as ActivityIcon } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  LineChart,
  Line,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';
import { useApi } from '@/hooks/useApi';
import { useAuth } from '@/contexts/AuthContext';
import { ACTIVITY_ENDPOINTS, LEADERBOARD_ENDPOINTS, PLAYER_ENDPOINTS, RULE_ENDPOINTS, toQuery } from '@/lib/api-routes';
import { fetchAllPages } from '@/services/api/pagination';
import { ApiRequestError, unwrap, useRuleStatsQuery } from '@/services/queries/rules';
import { useWalletSummaryQuery } from '@/services/queries/players';
import type { Activity, CursorPage, Rule } from '@/services/api/types';
import { cn } from '@/lib/utils';

/** How many recent activities feed the charts (bounded: there is no stats endpoint). */
const ACTIVITY_SAMPLE = 500;
const PIE_COLORS = ['#8B5CF6', '#6366F1', '#10B981', '#F59E0B', '#EC4899', '#64748B'];

interface LeaderboardSummary {
  id: string;
  name: string;
  type: string;
  is_active: boolean;
}

interface LeaderboardEntry {
  rank: number;
  player_id: string;
  external_id: string;
  display_name: string;
  score: number;
}

/** First page of a list (limit 100): the count, and whether there are more. */
function usePageCount(key: string, path: string, params?: object) {
  const api = useApi();
  return useQuery({
    queryKey: ['overview', 'count', key, params],
    queryFn: async () => {
      const page = unwrap(
        await api.get<CursorPage<unknown>>(`${path}${toQuery({ ...params, limit: 100 })}`, { showErrorToast: false }),
        `Failed to load ${key}`,
      );
      return { count: page.data.length, more: !!page.next_cursor };
    },
    staleTime: 30_000,
  });
}

function StatCard({
  label,
  icon: Icon,
  query,
  hint,
}: {
  label: string;
  icon: typeof Zap;
  query: { data?: { count: number; more: boolean }; isLoading: boolean; error: Error | null };
  hint?: string;
}) {
  return (
    <Card className="stat-card">
      <CardContent className="p-4">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-primary/10 mb-3">
          <Icon className="w-5 h-5 text-primary" />
        </div>
        {query.isLoading ? (
          <Skeleton className="h-8 w-16 mb-1" />
        ) : query.error ? (
          <p className="text-sm text-muted-foreground">Unavailable</p>
        ) : (
          <p className="text-2xl font-bold">
            {query.data!.count}
            {query.data!.more && '+'}
          </p>
        )}
        <p className="text-sm text-muted-foreground">{label}</p>
        {hint && <p className="text-xs text-muted-foreground mt-1">{hint}</p>}
      </CardContent>
    </Card>
  );
}

/** A single figure (e.g. points in circulation) with an optional secondary line. */
function ValueCard({
  label,
  icon: Icon,
  value,
  isLoading,
  unavailable,
  hint,
}: {
  label: string;
  icon: typeof Zap;
  value: number | undefined;
  isLoading: boolean;
  unavailable: boolean;
  hint?: string;
}) {
  return (
    <Card className="stat-card">
      <CardContent className="p-4">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-primary/10 mb-3">
          <Icon className="w-5 h-5 text-primary" />
        </div>
        {isLoading ? (
          <Skeleton className="h-8 w-16 mb-1" />
        ) : unavailable || value === undefined ? (
          <p className="text-sm text-muted-foreground">Unavailable</p>
        ) : (
          <p className="text-2xl font-bold tabular-nums">{value.toLocaleString()}</p>
        )}
        <p className="text-sm text-muted-foreground">{label}</p>
        {hint && !isLoading && !unavailable && <p className="text-xs text-muted-foreground mt-1">{hint}</p>}
      </CardContent>
    </Card>
  );
}

const TOP_RULES = 5;

function relativeTime(iso: string): string {
  const s = Math.round((Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return new Date(iso).toLocaleDateString();
}

export default function Overview() {
  const api = useApi();
  const { hasPermission } = useAuth();
  const canManageRules = hasPermission('manage:rules');

  const players = usePageCount('players', PLAYER_ENDPOINTS.LIST);
  const activeRules = usePageCount('rules', RULE_ENDPOINTS.LIST, { status: 'active' });
  const walletSummary = useWalletSummaryQuery();
  /** Last 30 days (the API default window). Needs rules.view_decisions. */
  const ruleStats = useRuleStatsQuery();
  const statsForbidden = ruleStats.error instanceof ApiRequestError && ruleStats.error.status === 403;
  const topRules = (ruleStats.data?.data ?? []).filter(r => r.fired > 0).slice(0, TOP_RULES);

  const activitiesQuery = useQuery({
    queryKey: ['overview', 'activities'],
    queryFn: async () =>
      unwrap(
        await fetchAllPages(
          cursor =>
            api.get<CursorPage<Activity>>(`${ACTIVITY_ENDPOINTS.LIST}${toQuery({ limit: 100, cursor })}`, {
              showErrorToast: false,
            }),
          ACTIVITY_SAMPLE,
        ),
        'Failed to load activities',
      ),
    staleTime: 30_000,
  });
  const activities = useMemo(() => activitiesQuery.data?.data ?? [], [activitiesQuery.data]);
  const sampleTruncated = !!activitiesQuery.data?.next_cursor;

  const leaderboardQuery = useQuery({
    queryKey: ['overview', 'top-leaderboard'],
    queryFn: async () => {
      const boards = unwrap(
        await api.get<CursorPage<LeaderboardSummary>>(`${LEADERBOARD_ENDPOINTS.LIST}${toQuery({ limit: 10 })}`, {
          showErrorToast: false,
        }),
        'Failed to load leaderboards',
      ).data;
      const board = boards.find(b => b.is_active) ?? boards[0];
      if (!board) return null;
      const entries = unwrap(
        await api.get<CursorPage<LeaderboardEntry>>(`${LEADERBOARD_ENDPOINTS.ENTRIES(board.id)}${toQuery({ limit: 5 })}`, {
          showErrorToast: false,
        }),
        'Failed to load leaderboard entries',
      ).data;
      return { board, entries };
    },
    staleTime: 30_000,
  });

  const recentRules = useQuery({
    queryKey: ['overview', 'recent-rules'],
    enabled: statsForbidden,
    queryFn: async () =>
      unwrap(
        await api.get<CursorPage<Rule>>(`${RULE_ENDPOINTS.LIST}${toQuery({ limit: 5 })}`, { showErrorToast: false }),
        'Failed to load rules',
      ).data,
    staleTime: 30_000,
  });

  /** Activities per day over the last 7 days (local time), from the sample. */
  const volume = useMemo(() => {
    const days: { key: string; date: string; activities: number }[] = [];
    for (let i = 6; i >= 0; i--) {
      const d = new Date();
      d.setHours(0, 0, 0, 0);
      d.setDate(d.getDate() - i);
      days.push({ key: d.toDateString(), date: d.toLocaleDateString(undefined, { weekday: 'short' }), activities: 0 });
    }
    const byKey = new Map(days.map(d => [d.key, d]));
    activities.forEach(a => {
      const day = byKey.get(new Date(a.occurred_at).toDateString());
      if (day) day.activities += 1;
    });
    return days;
  }, [activities]);

  const eventMix = useMemo(() => {
    const counts = new Map<string, number>();
    activities.forEach(a => counts.set(a.event_type, (counts.get(a.event_type) ?? 0) + 1));
    const sorted = [...counts.entries()].sort((a, b) => b[1] - a[1]);
    const top = sorted.slice(0, 5).map(([name, value]) => ({ name, value }));
    const rest = sorted.slice(5).reduce((sum, [, v]) => sum + v, 0);
    if (rest > 0) top.push({ name: 'other', value: rest });
    return top;
  }, [activities]);

  const sampleNote = sampleTruncated
    ? `Based on the latest ${activities.length} activities.`
    : `Based on all ${activities.length} activities.`;

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Dashboard Overview</h1>
          <p className="text-muted-foreground mt-1">Live figures from your tenant.</p>
        </div>
        {canManageRules && (
          <Link to="/rules/new">
            <Button variant="glow">
              <Plus className="w-4 h-4" />
              Create Rule
            </Button>
          </Link>
        )}
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard label="Players" icon={Users} query={players} />
        <StatCard label="Active rules" icon={GitBranch} query={activeRules} />
        <ValueCard
          label="Points in circulation"
          icon={Coins}
          value={walletSummary.data?.total_balance}
          isLoading={walletSummary.isLoading}
          unavailable={!!walletSummary.error}
          hint={
            walletSummary.data
              ? `+${walletSummary.data.credited_last_30d.toLocaleString()} / -${walletSummary.data.debited_last_30d.toLocaleString()} in 30 days · ${walletSummary.data.open_wallets.toLocaleString()} wallets`
              : undefined
          }
        />
        <StatCard
          label="Activities (last 7 days)"
          icon={Zap}
          query={{
            data: activitiesQuery.data
              ? { count: volume.reduce((s, d) => s + d.activities, 0), more: sampleTruncated }
              : undefined,
            isLoading: activitiesQuery.isLoading,
            error: activitiesQuery.error,
          }}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-lg">Activity Volume (7 days)</CardTitle>
            {!activitiesQuery.isLoading && <p className="text-xs text-muted-foreground">{sampleNote}</p>}
          </CardHeader>
          <CardContent>
            <div className="h-[300px]">
              {activitiesQuery.isLoading ? (
                <Skeleton className="h-full w-full" />
              ) : activitiesQuery.error ? (
                <p className="text-sm text-muted-foreground">Activity data is unavailable.</p>
              ) : (
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={volume}>
                    <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" />
                    <XAxis dataKey="date" stroke="hsl(var(--muted-foreground))" fontSize={12} />
                    <YAxis allowDecimals={false} stroke="hsl(var(--muted-foreground))" fontSize={12} />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: 'hsl(var(--card))',
                        border: '1px solid hsl(var(--border))',
                        borderRadius: '8px',
                      }}
                      labelStyle={{ color: 'hsl(var(--foreground))' }}
                    />
                    <Line
                      type="monotone"
                      dataKey="activities"
                      stroke="hsl(var(--primary))"
                      strokeWidth={3}
                      dot={{ fill: 'hsl(var(--primary))', strokeWidth: 2, r: 4 }}
                      activeDot={{ r: 6, fill: 'hsl(var(--primary))' }}
                    />
                  </LineChart>
                </ResponsiveContainer>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Event Types</CardTitle>
            {!activitiesQuery.isLoading && <p className="text-xs text-muted-foreground">{sampleNote}</p>}
          </CardHeader>
          <CardContent>
            <div className="h-[300px] flex flex-col items-center justify-center">
              {activitiesQuery.isLoading ? (
                <Skeleton className="h-40 w-40 rounded-full" />
              ) : eventMix.length === 0 ? (
                <p className="text-sm text-muted-foreground">No activities yet.</p>
              ) : (
                <>
                  <ResponsiveContainer width="100%" height="70%">
                    <PieChart>
                      <Pie data={eventMix} cx="50%" cy="50%" innerRadius={60} outerRadius={80} paddingAngle={4} dataKey="value">
                        {eventMix.map((entry, index) => (
                          <Cell key={entry.name} fill={PIE_COLORS[index % PIE_COLORS.length]} />
                        ))}
                      </Pie>
                      <Tooltip
                        contentStyle={{
                          backgroundColor: 'hsl(var(--card))',
                          border: '1px solid hsl(var(--border))',
                          borderRadius: '8px',
                        }}
                      />
                    </PieChart>
                  </ResponsiveContainer>
                  <div className="flex flex-wrap justify-center gap-3 mt-2">
                    {eventMix.map((entry, index) => (
                      <div key={entry.name} className="flex items-center gap-2">
                        <div className="w-3 h-3 rounded-full" style={{ backgroundColor: PIE_COLORS[index % PIE_COLORS.length] }} />
                        <span className="text-xs text-muted-foreground">
                          {entry.name} ({entry.value})
                        </span>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-lg">
              {leaderboardQuery.data?.board ? leaderboardQuery.data.board.name : 'Leaderboard'}
            </CardTitle>
            <Link to="/mechanics/leaderboards">
              <Button variant="ghost" size="sm">
                View all
                <ArrowRight className="w-4 h-4 ml-1" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            {leaderboardQuery.isLoading ? (
              <Skeleton className="h-32 w-full" />
            ) : !leaderboardQuery.data ? (
              <p className="text-sm text-muted-foreground">
                {leaderboardQuery.error ? 'Leaderboards are unavailable.' : 'No leaderboard yet.'}
              </p>
            ) : leaderboardQuery.data.entries.length === 0 ? (
              <p className="text-sm text-muted-foreground">No entries in the current period.</p>
            ) : (
              <ol className="space-y-3">
                {leaderboardQuery.data.entries.map(e => (
                  <li key={e.player_id} className="flex items-center gap-3">
                    <span className="w-6 text-sm font-bold text-muted-foreground">#{e.rank}</span>
                    <Link to={`/players/${e.player_id}`} className="flex-1 text-sm font-medium hover:underline truncate">
                      {e.display_name || e.external_id}
                    </Link>
                    <span className="text-sm tabular-nums">{e.score.toLocaleString()}</span>
                  </li>
                ))}
              </ol>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle className="text-lg">{statsForbidden ? 'Recently Updated Rules' : 'Top Rules (30 days)'}</CardTitle>
              {ruleStats.data && (
                <p className="text-xs text-muted-foreground mt-1">
                  {ruleStats.data.totals.fired.toLocaleString()} firings ·{' '}
                  {ruleStats.data.totals.points_awarded.toLocaleString()} points ·{' '}
                  {ruleStats.data.totals.xp_awarded.toLocaleString()} XP
                </p>
              )}
            </div>
            <Link to="/rules">
              <Button variant="ghost" size="sm">
                View all
                <ArrowRight className="w-4 h-4 ml-1" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            {statsForbidden ? (
              recentRules.isLoading ? (
                <Skeleton className="h-32 w-full" />
              ) : (recentRules.data ?? []).length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {recentRules.error ? 'Rules are unavailable.' : 'No rules yet.'}
                </p>
              ) : (
                <ul className="space-y-3">
                  {recentRules.data!.map(r => (
                    <li key={r.id} className="flex items-center gap-2">
                      <GitBranch className="w-4 h-4 text-primary shrink-0" />
                      <span className="flex-1 text-sm truncate">{r.name}</span>
                      <Badge variant="outline" className="capitalize text-xs">
                        {r.status}
                      </Badge>
                    </li>
                  ))}
                </ul>
              )
            ) : ruleStats.isLoading ? (
              <Skeleton className="h-32 w-full" />
            ) : ruleStats.error ? (
              <p className="text-sm text-muted-foreground">Rule statistics are unavailable.</p>
            ) : topRules.length === 0 ? (
              <p className="text-sm text-muted-foreground">No rule has fired in the last 30 days.</p>
            ) : (
              <ol className="space-y-3">
                {topRules.map((r, i) => (
                  <li key={r.rule_id} className="flex items-center gap-3">
                    <span className="w-6 text-sm font-bold text-muted-foreground">#{i + 1}</span>
                    <div className="flex-1 min-w-0">
                      {canManageRules ? (
                        <Link to={`/rules/${r.rule_id}`} className="text-sm font-medium hover:underline truncate block">
                          {r.name}
                        </Link>
                      ) : (
                        <span className="text-sm font-medium truncate block">{r.name}</span>
                      )}
                      {(r.points_awarded > 0 || r.xp_awarded > 0) && (
                        <p className="text-xs text-muted-foreground">
                          {r.points_awarded > 0 && `${r.points_awarded.toLocaleString()} pts`}
                          {r.points_awarded > 0 && r.xp_awarded > 0 && ' · '}
                          {r.xp_awarded > 0 && `${r.xp_awarded.toLocaleString()} XP`}
                        </p>
                      )}
                    </div>
                    <span className="text-sm tabular-nums" title="Times fired">
                      {r.fired.toLocaleString()}
                    </span>
                  </li>
                ))}
              </ol>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="text-lg">Recent Activity</CardTitle>
            <Link to="/events">
              <Button variant="ghost" size="sm">
                View all
                <ArrowRight className="w-4 h-4 ml-1" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            {activitiesQuery.isLoading ? (
              <Skeleton className="h-32 w-full" />
            ) : activities.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {activitiesQuery.error ? 'Activities are unavailable.' : 'No activities yet.'}
              </p>
            ) : (
              <div className="space-y-4 max-h-[250px] overflow-y-auto">
                {activities.slice(0, 8).map(a => (
                  <div key={a.id} className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-secondary flex items-center justify-center">
                      <ActivityIcon className="w-4 h-4 text-primary" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm truncate">
                        <span className="font-medium text-primary">{a.player_external_id ?? 'unknown player'}</span>{' '}
                        <code className="text-xs">{a.event_type}</code>
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {relativeTime(a.occurred_at)} ·{' '}
                        <span className={cn(a.status === 'rejected' && 'text-destructive')}>{a.status}</span>
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Link to={canManageRules ? '/rules/new' : '/rules'}>
          <Card className="stat-card cursor-pointer group">
            <CardContent className="p-6 flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-primary/10 flex items-center justify-center group-hover:bg-primary/20 transition-colors">
                <Plus className="w-6 h-6 text-primary" />
              </div>
              <div>
                <h3 className="font-semibold">{canManageRules ? 'Create New Rule' : 'Browse Rules'}</h3>
                <p className="text-sm text-muted-foreground">Define what earns points, XP and badges</p>
              </div>
              <ArrowRight className="w-5 h-5 text-muted-foreground ml-auto group-hover:translate-x-1 transition-transform" />
            </CardContent>
          </Card>
        </Link>

        <Link to="/rules">
          <Card className="stat-card cursor-pointer group">
            <CardContent className="p-6 flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-blue-500/10 flex items-center justify-center group-hover:bg-blue-500/20 transition-colors">
                <FileText className="w-6 h-6 text-blue-500" />
              </div>
              <div>
                <h3 className="font-semibold">Decisions & Simulation</h3>
                <p className="text-sm text-muted-foreground">See why rules matched or not</p>
              </div>
              <ArrowRight className="w-5 h-5 text-muted-foreground ml-auto group-hover:translate-x-1 transition-transform" />
            </CardContent>
          </Card>
        </Link>

        <Link to="/events">
          <Card className="stat-card cursor-pointer group">
            <CardContent className="p-6 flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-green-500/10 flex items-center justify-center group-hover:bg-green-500/20 transition-colors">
                <Send className="w-6 h-6 text-green-500" />
              </div>
              <div>
                <h3 className="font-semibold">Events & Activities</h3>
                <p className="text-sm text-muted-foreground">Event types and the activity log</p>
              </div>
              <ArrowRight className="w-5 h-5 text-muted-foreground ml-auto group-hover:translate-x-1 transition-transform" />
            </CardContent>
          </Card>
        </Link>
      </div>
    </div>
  );
}

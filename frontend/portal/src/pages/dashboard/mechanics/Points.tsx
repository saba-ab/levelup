import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { Search, Coins, Wallet, Send, TrendingUp, TrendingDown, History, ArrowDownRight, ArrowUpRight } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { useAuth } from '@/contexts/AuthContext';
import { StatBarChart } from '@/components/mechanics/StatBarChart';
import {
  walletAnalyticsKey,
  useWalletSummaryQuery,
  useWalletDistributionQuery,
  useWalletDailyQuery,
  describeMechanicsError,
} from '@/services/queries/mechanics';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { usePlayersQuery, usePlayerWalletsQuery } from '@/services/queries/players';
import type { Player } from '@/services/api/types';
import { getPlayerName } from '@/lib/player-utils';
import {
  PlayerAvatar,
  PaginationControls,
  WalletOperationDialog,
  useDebouncedValue,
  type WalletOperation,
} from '@/components/players';

/** YYYY-MM-DD of a date in UTC (the ledger buckets by UTC day). */
const utcDay = (d: Date) => d.toISOString().slice(0, 10);
const daysAgo = (n: number) => utcDay(new Date(Date.now() - n * 86_400_000));

function compact(n: number): string {
  return Math.abs(n) >= 10_000 ? new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(n) : n.toLocaleString();
}

/** Balance histogram (GET /wallets/distribution). */
function DistributionCard() {
  const { data: buckets, isLoading, error } = useWalletDistributionQuery();
  const chartData = (buckets ?? []).map((b) => ({
    range: b.from === b.to ? compact(b.from) : `${compact(b.from)}–${compact(b.to)}`,
    players: b.players,
  }));
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Balance distribution</CardTitle>
        <CardDescription>Wallets per balance range.</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-[260px] w-full" />
        ) : error ? (
          <p className="text-sm text-destructive py-8 text-center">Failed to load the distribution: {error.message}</p>
        ) : chartData.length === 0 ? (
          <p className="text-sm text-muted-foreground py-8 text-center">No wallets yet.</p>
        ) : (
          <StatBarChart data={chartData} xKey="range" series={[{ key: 'players', label: 'Wallets', color: 'hsl(var(--primary))' }]} />
        )}
      </CardContent>
    </Card>
  );
}

/** Daily credited / debited totals (GET /wallets/daily?from&to). */
function DailyCard() {
  const [from, setFrom] = useState(() => daysAgo(29));
  const [to, setTo] = useState(() => utcDay(new Date()));
  const { data: days, isLoading, error } = useWalletDailyQuery({ from: from || undefined, to: to || undefined });
  const chartData = (days ?? []).map((d) => ({ day: d.day, credited: d.credited, debited: d.debited }));
  const empty = chartData.every((d) => d.credited === 0 && d.debited === 0);
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col sm:flex-row sm:items-end sm:justify-between gap-3">
          <div>
            <CardTitle className="text-lg">Daily points flow</CardTitle>
            <CardDescription>Points credited and debited per day (UTC, at most 366 days).</CardDescription>
          </div>
          <div className="flex gap-2">
            <div className="space-y-1">
              <Label htmlFor="daily-from" className="text-xs">From</Label>
              <Input id="daily-from" type="date" className="w-40" value={from} max={to || undefined} onChange={(e) => setFrom(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label htmlFor="daily-to" className="text-xs">To</Label>
              <Input id="daily-to" type="date" className="w-40" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} />
            </div>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-[260px] w-full" />
        ) : error ? (
          <p className="text-sm text-destructive py-8 text-center">
            {describeMechanicsError(error, 'Failed to load daily totals', {
              invalid_range: 'Pick a range where "from" is not after "to" and that spans at most 366 days.',
            })}
          </p>
        ) : empty ? (
          <p className="text-sm text-muted-foreground py-8 text-center">No points moved in this range.</p>
        ) : (
          <StatBarChart
            data={chartData}
            xKey="day"
            tickFormatter={(d) => d.slice(5)}
            series={[
              { key: 'credited', label: 'Credited', color: 'hsl(142 71% 45%)' },
              { key: 'debited', label: 'Debited', color: 'hsl(var(--destructive))' },
            ]}
          />
        )}
      </CardContent>
    </Card>
  );
}

export default function Points() {
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [operation, setOperation] = useState<WalletOperation | null>(null);
  const [selectedPlayer, setSelectedPlayer] = useState<Player | null>(null);
  const queryClient = useQueryClient();
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:mechanics');
  const summary = useWalletSummaryQuery();

  const search = useDebouncedValue(searchQuery.trim());
  const pager = useCursorPagination(10);
  const { reset } = pager;
  useEffect(() => {
    reset();
  }, [search, reset]);

  const { data: playersData, isLoading } = usePlayersQuery({
    limit: pager.limit,
    cursor: pager.cursor,
    search: search || undefined,
  });
  const players = useMemo(() => playersData?.data ?? [], [playersData]);
  const playerIds = useMemo(() => players.map((p) => p.id), [players]);
  const wallets = usePlayerWalletsQuery(playerIds);

  const closeDialog = () => {
    setOperation(null);
    // Wallet operations change the tenant-wide figures and charts.
    queryClient.invalidateQueries({ queryKey: walletAnalyticsKey() });
  };

  const openDialog = (type: WalletOperation, player?: Player) => {
    setSelectedPlayer(player ?? null);
    setOperation(type);
  };

  const totals = summary.data;
  const statValue = (n: number | undefined) =>
    summary.isLoading ? '...' : summary.error || n === undefined ? '-' : n.toLocaleString();

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Points & Wallets</h1>
          <p className="text-muted-foreground mt-1">Manage player points and view transactions.</p>
        </div>
        {canManage && <div className="flex flex-wrap gap-2">
          <Button variant="outline" className="gap-2" onClick={() => openDialog('credit')}>
            <TrendingUp className="w-4 h-4" />
            Credit Points
          </Button>
          <Button variant="outline" className="gap-2" onClick={() => openDialog('debit')}>
            <TrendingDown className="w-4 h-4" />
            Debit Points
          </Button>
          <Button variant="glow" className="gap-2" onClick={() => openDialog('transfer')}>
            <Send className="w-4 h-4" />
            Transfer Points
          </Button>
        </div>}
      </div>

      {/* Tenant-wide totals (GET /wallets/summary) */}
      {summary.error && (
        <Card className="p-4 border-destructive">
          <p className="text-sm text-destructive">Failed to load the wallet summary: {summary.error.message}</p>
        </Card>
      )}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card className="stat-card">
          <CardContent className="p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-violet-500/20 flex items-center justify-center">
                <Coins className="w-6 h-6 text-violet-500" />
              </div>
              <Badge variant="outline" className="border-violet-500 text-violet-500">
                BALANCE
              </Badge>
            </div>
            <h3 className="font-semibold text-lg mb-1">Points Held</h3>
            <p className="text-3xl font-bold mt-4">{statValue(totals?.total_balance)}</p>
            <p className="text-xs text-muted-foreground">
              Across {statValue(totals?.open_wallets)} open wallet{totals?.open_wallets === 1 ? '' : 's'}
            </p>
          </CardContent>
        </Card>

        <Card className="stat-card" style={{ animationDelay: '100ms' }}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-amber-500/20 flex items-center justify-center">
                <Wallet className="w-6 h-6 text-amber-500" />
              </div>
              <Badge variant="outline" className="border-amber-500 text-amber-500">
                EARNED
              </Badge>
            </div>
            <h3 className="font-semibold text-lg mb-1">Lifetime Earned</h3>
            <p className="text-3xl font-bold mt-4">{statValue(totals?.lifetime_earned)}</p>
            <p className="text-xs text-muted-foreground flex items-center gap-1">
              <ArrowUpRight className="w-3 h-3 text-emerald-500" /> {statValue(totals?.credited_last_30d)} credited in the last 30 days
            </p>
          </CardContent>
        </Card>

        <Card className="stat-card" style={{ animationDelay: '200ms' }}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-emerald-500/20 flex items-center justify-center">
                <TrendingDown className="w-6 h-6 text-emerald-500" />
              </div>
              <Badge variant="outline" className="border-emerald-500 text-emerald-500">
                SPENT
              </Badge>
            </div>
            <h3 className="font-semibold text-lg mb-1">Lifetime Spent</h3>
            <p className="text-3xl font-bold mt-4">{statValue(totals?.lifetime_spent)}</p>
            <p className="text-xs text-muted-foreground flex items-center gap-1">
              <ArrowDownRight className="w-3 h-3 text-destructive" /> {statValue(totals?.debited_last_30d)} debited in the last 30 days
            </p>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <DailyCard />
        <DistributionCard />
      </div>

      {/* Players List */}
      <Card>
        <CardHeader>
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
            <CardTitle className="flex items-center gap-2">
              <Wallet className="w-5 h-5" />
              Player Wallets
            </CardTitle>
            <div className="relative w-full sm:w-64">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder="Search players..."
                className="pl-9"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-8 text-center text-muted-foreground">Loading players...</div>
          ) : players.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              {search ? 'No players match your search.' : 'No players found. Create a player to get started.'}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left p-4 text-sm font-medium text-muted-foreground">Player</th>
                    <th className="text-right p-4 text-sm font-medium text-muted-foreground">Balance</th>
                    <th className="text-right p-4 text-sm font-medium text-muted-foreground">Earned</th>
                    <th className="text-right p-4 text-sm font-medium text-muted-foreground">Spent</th>
                    <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                    <th className="text-right p-4 text-sm font-medium text-muted-foreground">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {players.map((player) => {
                    const name = getPlayerName(player);
                    const wallet = wallets.byPlayer[player.id];
                    const amount = (n: number | undefined) =>
                      wallet ? (wallet.opened ? (n ?? 0).toLocaleString() : '-') : '...';
                    return (
                      <tr key={player.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors">
                        <td className="p-4">
                          <div className="flex items-center gap-3">
                            <PlayerAvatar name={name} size="sm" />
                            <div className="min-w-0">
                              <p className="font-medium truncate">{name}</p>
                              <p className="text-xs text-muted-foreground font-mono truncate">{player.external_id}</p>
                            </div>
                          </div>
                        </td>
                        <td className="p-4 text-right font-mono font-semibold">{amount(wallet?.balance)}</td>
                        <td className="p-4 text-right font-mono text-muted-foreground">{amount(wallet?.lifetime_earned)}</td>
                        <td className="p-4 text-right font-mono text-muted-foreground">{amount(wallet?.lifetime_spent)}</td>
                        <td className="p-4">
                          {wallet && !wallet.opened ? (
                            <Badge variant="outline">No wallet</Badge>
                          ) : (
                            <Badge variant={player.is_active && wallet?.is_active !== false ? 'default' : 'secondary'}>
                              {!player.is_active ? 'Player inactive' : wallet?.is_active === false ? 'Wallet inactive' : 'Active'}
                            </Badge>
                          )}
                        </td>
                        <td className="p-4 text-right">
                          <div className="flex gap-2 justify-end">
                            {canManage && <>
                            <Button size="sm" variant="outline" onClick={() => openDialog('credit', player)}>
                              <TrendingUp className="w-3 h-3 mr-1" />
                              Credit
                            </Button>
                            <Button size="sm" variant="outline" onClick={() => openDialog('debit', player)}>
                              <TrendingDown className="w-3 h-3 mr-1" />
                              Debit
                            </Button>
                            <Button size="sm" variant="outline" onClick={() => openDialog('transfer', player)}>
                              <Send className="w-3 h-3 mr-1" />
                              Transfer
                            </Button>
                            </>}
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => navigate(`/players/${player.id}`)}
                              aria-label={`Transactions of ${name}`}
                            >
                              <History className="w-3 h-3" />
                            </Button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
          <PaginationControls
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            hasNext={!!playersData?.next_cursor}
            onPrevious={pager.previous}
            onNext={() => playersData && pager.next(playersData.next_cursor)}
            itemCount={players.length}
            itemLabel="players"
          />
        </CardContent>
      </Card>

      <WalletOperationDialog operation={operation} player={selectedPlayer} onClose={closeDialog} />
    </div>
  );
}

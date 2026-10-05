import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, Coins, Wallet, Send, TrendingUp, TrendingDown, History } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
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

export default function Points() {
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [operation, setOperation] = useState<WalletOperation | null>(null);
  const [selectedPlayer, setSelectedPlayer] = useState<Player | null>(null);

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

  /** Real totals over the wallets loaded on this page (the API has no tenant-wide aggregate). */
  const pageTotals = useMemo(() => {
    const loaded = Object.values(wallets.byPlayer);
    return {
      balance: loaded.reduce((sum, w) => sum + w.balance, 0),
      earned: loaded.reduce((sum, w) => sum + w.lifetime_earned, 0),
      spent: loaded.reduce((sum, w) => sum + w.lifetime_spent, 0),
      openWallets: loaded.filter((w) => w.opened).length,
    };
  }, [wallets.byPlayer]);

  const openDialog = (type: WalletOperation, player?: Player) => {
    setSelectedPlayer(player ?? null);
    setOperation(type);
  };

  const statValue = (n: number) => (wallets.isLoading ? '...' : n.toLocaleString());

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Points & Wallets</h1>
          <p className="text-muted-foreground mt-1">Manage player points and view transactions.</p>
        </div>
        <div className="flex flex-wrap gap-2">
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
        </div>
      </div>

      {/* Totals over the wallets on the current page */}
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
            <p className="text-3xl font-bold mt-4">{statValue(pageTotals.balance)}</p>
            <p className="text-xs text-muted-foreground">
              Across {pageTotals.openWallets} open wallet{pageTotals.openWallets === 1 ? '' : 's'} on this page
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
            <p className="text-3xl font-bold mt-4">{statValue(pageTotals.earned)}</p>
            <p className="text-xs text-muted-foreground">Players on this page</p>
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
            <p className="text-3xl font-bold mt-4">{statValue(pageTotals.spent)}</p>
            <p className="text-xs text-muted-foreground">Players on this page</p>
          </CardContent>
        </Card>
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

      <WalletOperationDialog operation={operation} player={selectedPlayer} onClose={() => setOperation(null)} />
    </div>
  );
}

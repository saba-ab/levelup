import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, GitCompare, Plus, X } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useAuth } from '@/contexts/AuthContext';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { usePlayersQuery, usePlayersProgressQuery, usePlayersWalletsQuery } from '@/services/queries/players';
import { usePlayersLastSeenQuery } from '@/services/queries/activities';
import type { PlayerSort } from '@/services/api/models/players';
import {
  PlayerAvatar,
  PaginationControls,
  PlayerFormDialog,
  LevelBadge,
  LastSeen,
  useDebouncedValue,
} from '@/components/players';
import { formatDate, getPlayerName } from '@/lib/player-utils';

type StatusFilter = 'all' | 'active' | 'inactive';

const PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
const COLUMN_COUNT = 9;

const SORT_LABELS: Record<PlayerSort, string> = {
  '-created_at': 'Newest first',
  created_at: 'Oldest first',
  display_name: 'Name (A-Z)',
};

/** YYYY-MM-DD + 1 day: created_to is exclusive, the date input means "through that day". */
function nextDay(date: string): string {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + 1);
  return d.toISOString().slice(0, 10);
}

/** A batch column cell: skeleton while its page loads, "-" when the batch call failed. */
function BatchCell({ loading, failed, children }: { loading: boolean; failed: boolean; children: ReactNode }) {
  if (loading) return <Skeleton className="h-4 w-12" />;
  if (failed) return <span className="text-muted-foreground">-</span>;
  return <>{children}</>;
}

const batchLoading = (q: { isLoading: boolean; isPlaceholderData: boolean }) => q.isLoading || q.isPlaceholderData;

export default function Players() {
  const navigate = useNavigate();
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:players');
  const [searchQuery, setSearchQuery] = useState('');
  const [status, setStatus] = useState<StatusFilter>('all');
  const [sort, setSort] = useState<PlayerSort>('-created_at');
  const [createdFrom, setCreatedFrom] = useState('');
  const [createdTo, setCreatedTo] = useState('');
  const [pageSize, setPageSize] = useState(25);
  const [selectedPlayers, setSelectedPlayers] = useState<string[]>([]);
  const [createOpen, setCreateOpen] = useState(false);

  const search = useDebouncedValue(searchQuery.trim());
  const pager = useCursorPagination(pageSize);
  const { reset } = pager;

  useEffect(() => {
    reset();
  }, [search, status, sort, createdFrom, createdTo, pageSize, reset]);

  const dateRangeInvalid = !!createdFrom && !!createdTo && createdTo < createdFrom;

  const { data, isLoading, isError, error } = usePlayersQuery(
    {
      limit: pager.limit,
      cursor: pager.cursor,
      search: search || undefined,
      is_active: status === 'all' ? undefined : status === 'active',
      sort: sort === '-created_at' ? undefined : sort,
      created_from: createdFrom || undefined,
      created_to: createdTo ? nextDay(createdTo) : undefined,
    },
    { enabled: !dateRangeInvalid },
  );
  const players = useMemo(() => data?.data ?? [], [data]);
  const playerIds = useMemo(() => players.map((p) => p.id), [players]);

  // One batch call per page for each column, not one per row.
  const progress = usePlayersProgressQuery(playerIds);
  const wallets = usePlayersWalletsQuery(playerIds);
  const lastSeen = usePlayersLastSeenQuery(playerIds);

  const hasFilters = !!search || status !== 'all' || !!createdFrom || !!createdTo;

  const togglePlayerSelection = (playerId: string) => {
    setSelectedPlayers((prev) => {
      if (prev.includes(playerId)) {
        return prev.filter((id) => id !== playerId);
      }
      if (prev.length >= 2) {
        return [prev[1], playerId];
      }
      return [...prev, playerId];
    });
  };

  const handleCompare = () => {
    if (selectedPlayers.length === 2) {
      navigate(`/players/compare?players=${selectedPlayers.join(',')}`);
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Players</h1>
          <p className="text-muted-foreground mt-1">View and manage your platform players.</p>
        </div>
        <div className="flex gap-2">
          {selectedPlayers.length > 0 && (
            <Button variant="outline" onClick={handleCompare} disabled={selectedPlayers.length !== 2} className="gap-2">
              <GitCompare className="w-4 h-4" />
              Compare ({selectedPlayers.length}/2)
            </Button>
          )}
          {canManage && (
            <Button onClick={() => setCreateOpen(true)} className="gap-2">
              <Plus className="w-4 h-4" />
              New Player
            </Button>
          )}
        </div>
      </div>

      <div className="space-y-2">
        <div className="flex flex-col lg:flex-row lg:flex-wrap gap-3">
          <div className="relative w-full lg:max-w-sm">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
            <Input
              placeholder="Search by name, email or external ID..."
              className="pl-9"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
          <Select value={status} onValueChange={(v) => setStatus(v as StatusFilter)}>
            <SelectTrigger className="w-full lg:w-[150px]" aria-label="Status">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All statuses</SelectItem>
              <SelectItem value="active">Active</SelectItem>
              <SelectItem value="inactive">Inactive</SelectItem>
            </SelectContent>
          </Select>
          <Select value={sort} onValueChange={(v) => setSort(v as PlayerSort)}>
            <SelectTrigger className="w-full lg:w-[160px]" aria-label="Sort">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(Object.keys(SORT_LABELS) as PlayerSort[]).map((s) => (
                <SelectItem key={s} value={s}>
                  {SORT_LABELS[s]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm text-muted-foreground whitespace-nowrap">Joined</span>
            <Input
              type="date"
              aria-label="Joined from"
              value={createdFrom}
              max={createdTo || undefined}
              onChange={(e) => setCreatedFrom(e.target.value)}
              className="w-[150px]"
            />
            <span className="text-sm text-muted-foreground">to</span>
            <Input
              type="date"
              aria-label="Joined to"
              value={createdTo}
              min={createdFrom || undefined}
              onChange={(e) => setCreatedTo(e.target.value)}
              className="w-[150px]"
            />
            {(createdFrom || createdTo) && (
              <Button
                variant="ghost"
                size="icon"
                aria-label="Clear dates"
                onClick={() => {
                  setCreatedFrom('');
                  setCreatedTo('');
                }}
              >
                <X className="w-4 h-4" />
              </Button>
            )}
          </div>
        </div>
        {dateRangeInvalid && <p className="text-sm text-destructive">The end date must not be before the start date.</p>}
      </div>

      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="w-12 p-4"></th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Player</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Email</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Level</th>
                  <th className="text-right p-4 text-sm font-medium text-muted-foreground">XP</th>
                  <th className="text-right p-4 text-sm font-medium text-muted-foreground">Balance</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Last seen</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Created</th>
                </tr>
              </thead>
              <tbody>
                {dateRangeInvalid ? (
                  <tr>
                    <td colSpan={COLUMN_COUNT} className="p-8 text-center text-muted-foreground">
                      Fix the date range to see players.
                    </td>
                  </tr>
                ) : isLoading ? (
                  <tr>
                    <td colSpan={COLUMN_COUNT} className="p-8 text-center text-muted-foreground">
                      Loading players...
                    </td>
                  </tr>
                ) : isError ? (
                  <tr>
                    <td colSpan={COLUMN_COUNT} className="p-8 text-center text-destructive">
                      {error instanceof Error ? error.message : 'Failed to load players.'}
                    </td>
                  </tr>
                ) : players.length > 0 ? (
                  players.map((player) => {
                    const name = getPlayerName(player);
                    const prog = progress.data?.[player.id];
                    const wallet = wallets.data?.[player.id];
                    const seen = lastSeen.data?.[player.id];
                    return (
                      <tr
                        key={player.id}
                        className="border-b border-border/50 hover:bg-secondary/30 transition-colors cursor-pointer"
                        onClick={() => navigate(`/players/${player.id}`)}
                      >
                        <td className="p-4" onClick={(e) => e.stopPropagation()}>
                          <Checkbox
                            checked={selectedPlayers.includes(player.id)}
                            onCheckedChange={() => togglePlayerSelection(player.id)}
                            aria-label={`Select ${name}`}
                          />
                        </td>
                        <td className="p-4">
                          <div className="flex items-center gap-3">
                            <PlayerAvatar name={name} />
                            <div className="min-w-0">
                              <p className="font-medium truncate">{name}</p>
                              <p className="text-xs text-muted-foreground font-mono truncate">{player.external_id}</p>
                            </div>
                          </div>
                        </td>
                        <td className="p-4 text-muted-foreground">{player.email || '-'}</td>
                        <td className="p-4">
                          <BatchCell loading={batchLoading(progress)} failed={progress.isError}>
                            {prog?.current_level ? (
                              <LevelBadge
                                level={prog.current_level.name || `Level ${prog.current_level.level_number}`}
                                levelNumber={prog.current_level.level_number}
                                size="sm"
                              />
                            ) : (
                              <span className="text-muted-foreground">-</span>
                            )}
                          </BatchCell>
                        </td>
                        <td className="p-4 text-right tabular-nums">
                          <BatchCell loading={batchLoading(progress)} failed={progress.isError}>
                            {(prog?.total_xp ?? 0).toLocaleString()}
                          </BatchCell>
                        </td>
                        <td className="p-4 text-right tabular-nums">
                          <BatchCell loading={batchLoading(wallets)} failed={wallets.isError}>
                            {(wallet?.balance ?? 0).toLocaleString()}
                          </BatchCell>
                        </td>
                        <td className="p-4 text-sm">
                          <BatchCell loading={batchLoading(lastSeen)} failed={lastSeen.isError}>
                            <LastSeen at={seen?.last_activity_at} eventType={seen?.last_event_type} />
                          </BatchCell>
                        </td>
                        <td className="p-4">
                          <Badge variant={player.is_active ? 'default' : 'secondary'}>
                            {player.is_active ? 'Active' : 'Inactive'}
                          </Badge>
                        </td>
                        <td className="p-4 text-muted-foreground">{formatDate(player.created_at)}</td>
                      </tr>
                    );
                  })
                ) : (
                  <tr>
                    <td colSpan={COLUMN_COUNT} className="p-8 text-center text-muted-foreground">
                      {hasFilters ? 'No players match your filters.' : 'No players yet.'}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <PaginationControls
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            hasNext={!!data?.next_cursor}
            onPrevious={pager.previous}
            onNext={() => data && pager.next(data.next_cursor)}
            itemCount={players.length}
            pageSize={pageSize}
            pageSizeOptions={PAGE_SIZE_OPTIONS}
            onPageSizeChange={setPageSize}
            itemLabel="players"
          />
        </CardContent>
      </Card>

      <PlayerFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onSaved={(player) => navigate(`/players/${player.id}`)}
      />
    </div>
  );
}

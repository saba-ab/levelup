import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, GitCompare, Plus } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { usePlayersQuery } from '@/services/queries/players';
import {
  PlayerAvatar,
  PaginationControls,
  PlayerFormDialog,
  useDebouncedValue,
} from '@/components/players';
import { formatDate, getPlayerName } from '@/lib/player-utils';

type StatusFilter = 'all' | 'active' | 'inactive';

const PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

export default function Players() {
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [status, setStatus] = useState<StatusFilter>('all');
  const [pageSize, setPageSize] = useState(25);
  const [selectedPlayers, setSelectedPlayers] = useState<string[]>([]);
  const [createOpen, setCreateOpen] = useState(false);

  const search = useDebouncedValue(searchQuery.trim());
  const pager = useCursorPagination(pageSize);
  const { reset } = pager;

  useEffect(() => {
    reset();
  }, [search, status, pageSize, reset]);

  const { data, isLoading, isError, error } = usePlayersQuery({
    limit: pager.limit,
    cursor: pager.cursor,
    search: search || undefined,
    is_active: status === 'all' ? undefined : status === 'active',
  });
  const players = data?.data ?? [];

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
          <Button onClick={() => setCreateOpen(true)} className="gap-2">
            <Plus className="w-4 h-4" />
            New Player
          </Button>
        </div>
      </div>

      <div className="flex flex-col sm:flex-row gap-3">
        <div className="relative w-full sm:max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <Input
            placeholder="Search by name, email or external ID..."
            className="pl-9"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
        <Select value={status} onValueChange={(v) => setStatus(v as StatusFilter)}>
          <SelectTrigger className="w-full sm:w-[160px]" aria-label="Status">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="inactive">Inactive</SelectItem>
          </SelectContent>
        </Select>
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
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Created</th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center text-muted-foreground">
                      Loading players...
                    </td>
                  </tr>
                ) : isError ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center text-destructive">
                      {error instanceof Error ? error.message : 'Failed to load players.'}
                    </td>
                  </tr>
                ) : players.length > 0 ? (
                  players.map((player) => {
                    const name = getPlayerName(player);
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
                    <td colSpan={5} className="p-8 text-center text-muted-foreground">
                      {search || status !== 'all' ? 'No players match your filters.' : 'No players yet.'}
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

import { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, GitCompare } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { players } from '@/lib/mockData';
import { usePagination } from '@/hooks/usePagination';
import { PlayerAvatar, LevelBadge, PaginationControls } from '@/components/players';

export default function Players() {
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedPlayers, setSelectedPlayers] = useState<string[]>([]);
  const navigate = useNavigate();

  const filteredPlayers = useMemo(
    () =>
      players.filter(
        (player) =>
          player.email.toLowerCase().includes(searchQuery.toLowerCase()) ||
          player.id.toLowerCase().includes(searchQuery.toLowerCase())
      ),
    [searchQuery]
  );

  const pagination = usePagination(filteredPlayers, {
    initialPageSize: 10,
    pageSizeOptions: [5, 10, 25, 50],
  });

  const togglePlayerSelection = (playerId: string, e: React.MouseEvent) => {
    e.stopPropagation();
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Players</h1>
          <p className="text-muted-foreground mt-1">View and manage your platform players.</p>
        </div>
        {selectedPlayers.length > 0 && (
          <Button onClick={handleCompare} disabled={selectedPlayers.length !== 2} className="gap-2">
            <GitCompare className="w-4 h-4" />
            Compare ({selectedPlayers.length}/2)
          </Button>
        )}
      </div>

      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <Input
          placeholder="Search by email or player ID..."
          className="pl-9"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border">
                <th className="w-12 p-4"></th>
                <th className="text-left p-4 text-sm font-medium text-muted-foreground">Player</th>
                <th className="text-left p-4 text-sm font-medium text-muted-foreground">Level</th>
                <th className="text-left p-4 text-sm font-medium text-muted-foreground">Total XP</th>
                <th className="text-left p-4 text-sm font-medium text-muted-foreground">Last Active</th>
              </tr>
            </thead>
            <tbody>
              {pagination.paginatedData.length > 0 ? (
                pagination.paginatedData.map((player) => (
                  <tr
                    key={player.id}
                    className="border-b border-border/50 hover:bg-secondary/30 transition-colors cursor-pointer"
                    onClick={() => navigate(`/players/${player.id}`)}
                  >
                    <td className="p-4" onClick={(e) => e.stopPropagation()}>
                      <Checkbox
                        checked={selectedPlayers.includes(player.id)}
                        onCheckedChange={() => {}}
                        onClick={(e) => togglePlayerSelection(player.id, e)}
                      />
                    </td>
                    <td className="p-4">
                      <div className="flex items-center gap-3">
                        <PlayerAvatar email={player.email} />
                        <div>
                          <p className="font-medium">{player.email}</p>
                          <p className="text-xs text-muted-foreground">{player.id}</p>
                        </div>
                      </div>
                    </td>
                    <td className="p-4">
                      <LevelBadge level={player.level} />
                    </td>
                    <td className="p-4 font-mono">{player.totalXp.toLocaleString()}</td>
                    <td className="p-4 text-muted-foreground">{player.lastActive}</td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={5} className="p-8 text-center text-muted-foreground">
                    No players found matching your search.
                  </td>
                </tr>
              )}
            </tbody>
          </table>

          <PaginationControls
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={pagination.totalItems}
            pageSize={pagination.pageSize}
            pageSizeOptions={pagination.pageSizeOptions}
            onPageChange={pagination.goToPage}
            onPageSizeChange={pagination.setPageSize}
            canGoNext={pagination.canGoNext}
            canGoPrevious={pagination.canGoPrevious}
            itemLabel="players"
          />
        </CardContent>
      </Card>
    </div>
  );
}

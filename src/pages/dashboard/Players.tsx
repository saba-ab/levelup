import React, { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { Search, GitCompare, ChevronLeft, ChevronRight } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { players } from '@/lib/mockData';
import { cn } from '@/lib/utils';

const PAGE_SIZE_OPTIONS = [5, 10, 25, 50];

export default function Players() {
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedPlayers, setSelectedPlayers] = useState<string[]>([]);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const navigate = useNavigate();

  const filteredPlayers = useMemo(() => 
    players.filter(player =>
      player.email.toLowerCase().includes(searchQuery.toLowerCase()) ||
      player.id.toLowerCase().includes(searchQuery.toLowerCase())
    ), [searchQuery]
  );

  const totalPages = Math.ceil(filteredPlayers.length / pageSize);
  const startIndex = (currentPage - 1) * pageSize;
  const endIndex = startIndex + pageSize;
  const paginatedPlayers = filteredPlayers.slice(startIndex, endIndex);

  // Reset to page 1 when search changes
  React.useEffect(() => {
    setCurrentPage(1);
  }, [searchQuery, pageSize]);

  const getLevelColor = (level: string) => {
    switch (level.toLowerCase()) {
      case 'diamond': return 'border-cyan-400 text-cyan-400 bg-cyan-400/10';
      case 'platinum': return 'border-slate-300 text-slate-300 bg-slate-300/10';
      case 'gold': return 'border-yellow-500 text-yellow-500 bg-yellow-500/10';
      case 'silver': return 'border-gray-400 text-gray-400 bg-gray-400/10';
      default: return 'border-amber-700 text-amber-700 bg-amber-700/10';
    }
  };

  const togglePlayerSelection = (playerId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setSelectedPlayers(prev => {
      if (prev.includes(playerId)) {
        return prev.filter(id => id !== playerId);
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

  const goToPage = (page: number) => {
    setCurrentPage(Math.max(1, Math.min(page, totalPages)));
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Players</h1>
          <p className="text-muted-foreground mt-1">View and manage your platform players.</p>
        </div>
        {selectedPlayers.length > 0 && (
          <Button 
            onClick={handleCompare} 
            disabled={selectedPlayers.length !== 2}
            className="gap-2"
          >
            <GitCompare className="w-4 h-4" />
            Compare ({selectedPlayers.length}/2)
          </Button>
        )}
      </div>

      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <Input placeholder="Search by email or player ID..." className="pl-9" value={searchQuery} onChange={(e) => setSearchQuery(e.target.value)} />
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
              {paginatedPlayers.length > 0 ? (
                paginatedPlayers.map((player) => (
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
                        <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center">
                          <span className="text-sm font-medium">{player.email[0].toUpperCase()}</span>
                        </div>
                        <div>
                          <p className="font-medium">{player.email}</p>
                          <p className="text-xs text-muted-foreground">{player.id}</p>
                        </div>
                      </div>
                    </td>
                    <td className="p-4">
                      <Badge variant="outline" className={cn("capitalize", getLevelColor(player.level))}>{player.level}</Badge>
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

          {/* Pagination Controls */}
          <div className="flex items-center justify-between p-4 border-t border-border">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <span>Showing</span>
              <Select value={pageSize.toString()} onValueChange={(value) => setPageSize(Number(value))}>
                <SelectTrigger className="w-[70px] h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PAGE_SIZE_OPTIONS.map((size) => (
                    <SelectItem key={size} value={size.toString()}>
                      {size}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <span>of {filteredPlayers.length} players</span>
            </div>

            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">
                Page {currentPage} of {totalPages || 1}
              </span>
              <div className="flex items-center gap-1">
                <Button
                  variant="outline"
                  size="icon"
                  className="h-8 w-8"
                  onClick={() => goToPage(currentPage - 1)}
                  disabled={currentPage <= 1}
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  size="icon"
                  className="h-8 w-8"
                  onClick={() => goToPage(currentPage + 1)}
                  disabled={currentPage >= totalPages}
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

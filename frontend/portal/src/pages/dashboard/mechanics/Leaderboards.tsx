import React, { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Trophy, Medal, Award, TrendingUp, Users, Target } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useLeaderboardsQuery, useLeaderboardEntriesQuery } from '@/services/queries/mechanics';

export default function Leaderboards() {
  const [selectedLeaderboardId, setSelectedLeaderboardId] = useState<number | null>(null);

  // Fetch all leaderboards
  const { data: leaderboardsData, isLoading: isLoadingLeaderboards } = useLeaderboardsQuery();
  const leaderboards = React.useMemo(() => leaderboardsData || [], [leaderboardsData]);

  // Auto-select first leaderboard
  React.useEffect(() => {
    if (leaderboards.length > 0 && !selectedLeaderboardId) {
      setSelectedLeaderboardId(leaderboards[0].id);
    }
  }, [leaderboards, selectedLeaderboardId]);

  // Fetch entries for selected leaderboard
  const { data: entriesData, isLoading: isLoadingEntries } = useLeaderboardEntriesQuery(
    selectedLeaderboardId || 0,
    100,
    0
  );
  const entries = React.useMemo(() => entriesData || [], [entriesData]);

  const getRankIcon = (rank: number) => {
    if (rank === 1) return <Trophy className="w-5 h-5 text-yellow-500" />;
    if (rank === 2) return <Medal className="w-5 h-5 text-gray-400" />;
    if (rank === 3) return <Award className="w-5 h-5 text-amber-700" />;
    return null;
  };

  const getRankStyle = (rank: number) => {
    if (rank === 1) return 'bg-gradient-to-r from-yellow-500/20 to-yellow-500/5 border-yellow-500/30';
    if (rank === 2) return 'bg-gradient-to-r from-gray-400/20 to-gray-400/5 border-gray-400/30';
    if (rank === 3) return 'bg-gradient-to-r from-amber-700/20 to-amber-700/5 border-amber-700/30';
    return '';
  };

  const selectedLeaderboard = leaderboards.find(l => l.id === selectedLeaderboardId);
  const topThree = entries.slice(0, 3);

  if (isLoadingLeaderboards) {
    return (
      <div className="space-y-6 animate-fade-in">
        <div className="text-center py-12 text-muted-foreground">Loading leaderboards...</div>
      </div>
    );
  }

  if (leaderboards.length === 0) {
    return (
      <div className="space-y-6 animate-fade-in">
        <div className="text-center py-12">
          <Trophy className="w-16 h-16 mx-auto text-muted-foreground mb-4" />
          <h3 className="text-lg font-semibold mb-2">No Leaderboards Yet</h3>
          <p className="text-muted-foreground">Leaderboards will appear here once they are created.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Leaderboards</h1>
          <p className="text-muted-foreground mt-1">View and manage competitive rankings.</p>
        </div>
        <div className="w-full sm:w-64">
          <Select
            value={selectedLeaderboardId?.toString()}
            onValueChange={(value) => setSelectedLeaderboardId(parseInt(value))}
          >
            <SelectTrigger>
              <SelectValue placeholder="Select leaderboard" />
            </SelectTrigger>
            <SelectContent>
              {leaderboards.map((leaderboard) => (
                <SelectItem key={leaderboard.id} value={leaderboard.id.toString()}>
                  {leaderboard.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Stats Cards */}
      {selectedLeaderboard && (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Card className="stat-card">
            <CardContent className="p-6">
              <div className="flex items-center justify-between mb-4">
                <div className="w-12 h-12 rounded-xl bg-violet-500/20 flex items-center justify-center">
                  <Target className="w-6 h-6 text-violet-500" />
                </div>
              </div>
              <h3 className="font-semibold text-lg mb-1">Type</h3>
              <p className="text-2xl font-bold capitalize">{selectedLeaderboard.type}</p>
            </CardContent>
          </Card>

          <Card className="stat-card" style={{ animationDelay: '100ms' }}>
            <CardContent className="p-6">
              <div className="flex items-center justify-between mb-4">
                <div className="w-12 h-12 rounded-xl bg-amber-500/20 flex items-center justify-center">
                  <Users className="w-6 h-6 text-amber-500" />
                </div>
              </div>
              <h3 className="font-semibold text-lg mb-1">Scope</h3>
              <p className="text-2xl font-bold capitalize">{selectedLeaderboard.scope}</p>
            </CardContent>
          </Card>

          <Card className="stat-card" style={{ animationDelay: '200ms' }}>
            <CardContent className="p-6">
              <div className="flex items-center justify-between mb-4">
                <div className="w-12 h-12 rounded-xl bg-emerald-500/20 flex items-center justify-center">
                  <TrendingUp className="w-6 h-6 text-emerald-500" />
                </div>
              </div>
              <h3 className="font-semibold text-lg mb-1">Reset</h3>
              <p className="text-2xl font-bold capitalize">{selectedLeaderboard.reset_frequency}</p>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Podium */}
      {topThree.length >= 3 && (
        <div className="flex justify-center items-end gap-4 py-8">
          {/* Second Place */}
          <div className="text-center">
            <div className="w-20 h-20 rounded-full bg-gray-400/20 border-2 border-gray-400 flex items-center justify-center mx-auto mb-3">
              {topThree[1].player.avatar_url ? (
                <img src={topThree[1].player.avatar_url} alt={topThree[1].player.display_name} className="w-full h-full rounded-full object-cover" />
              ) : (
                <span className="text-2xl font-bold">{topThree[1].player.display_name[0]}</span>
              )}
            </div>
            <div className="bg-gray-400/20 rounded-t-lg px-6 py-4 border border-gray-400/30">
              <Medal className="w-8 h-8 text-gray-400 mx-auto mb-2" />
              <p className="font-semibold">{topThree[1].player.display_name}</p>
              <p className="text-sm text-muted-foreground">{topThree[1].score.toLocaleString()} pts</p>
            </div>
            <div className="h-24 bg-gray-400/10 rounded-b-lg border-x border-b border-gray-400/30" />
          </div>

          {/* First Place */}
          <div className="text-center -mt-8">
            <div className="w-24 h-24 rounded-full bg-yellow-500/20 border-2 border-yellow-500 flex items-center justify-center mx-auto mb-3 animate-glow">
              {topThree[0].player.avatar_url ? (
                <img src={topThree[0].player.avatar_url} alt={topThree[0].player.display_name} className="w-full h-full rounded-full object-cover" />
              ) : (
                <span className="text-3xl font-bold">{topThree[0].player.display_name[0]}</span>
              )}
            </div>
            <div className="bg-yellow-500/20 rounded-t-lg px-8 py-4 border border-yellow-500/30">
              <Trophy className="w-10 h-10 text-yellow-500 mx-auto mb-2" />
              <p className="font-bold text-lg">{topThree[0].player.display_name}</p>
              <p className="text-sm text-muted-foreground">{topThree[0].score.toLocaleString()} pts</p>
            </div>
            <div className="h-32 bg-yellow-500/10 rounded-b-lg border-x border-b border-yellow-500/30" />
          </div>

          {/* Third Place */}
          <div className="text-center">
            <div className="w-20 h-20 rounded-full bg-amber-700/20 border-2 border-amber-700 flex items-center justify-center mx-auto mb-3">
              {topThree[2].player.avatar_url ? (
                <img src={topThree[2].player.avatar_url} alt={topThree[2].player.display_name} className="w-full h-full rounded-full object-cover" />
              ) : (
                <span className="text-2xl font-bold">{topThree[2].player.display_name[0]}</span>
              )}
            </div>
            <div className="bg-amber-700/20 rounded-t-lg px-6 py-4 border border-amber-700/30">
              <Award className="w-8 h-8 text-amber-700 mx-auto mb-2" />
              <p className="font-semibold">{topThree[2].player.display_name}</p>
              <p className="text-sm text-muted-foreground">{topThree[2].score.toLocaleString()} pts</p>
            </div>
            <div className="h-16 bg-amber-700/10 rounded-b-lg border-x border-b border-amber-700/30" />
          </div>
        </div>
      )}

      {/* Full Leaderboard Table */}
      <Card>
        <CardHeader>
          <CardTitle>Full Rankings</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {isLoadingEntries ? (
            <div className="p-8 text-center text-muted-foreground">Loading rankings...</div>
          ) : entries.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              No rankings yet. Players will appear here as they earn points.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left p-4 text-sm font-medium text-muted-foreground w-20">Rank</th>
                    <th className="text-left p-4 text-sm font-medium text-muted-foreground">Player</th>
                    <th className="text-right p-4 text-sm font-medium text-muted-foreground">Score</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((entry) => (
                    <tr
                      key={entry.player_id}
                      className={cn(
                        "border-b border-border/50 transition-colors",
                        entry.rank <= 3 ? getRankStyle(entry.rank) : "hover:bg-secondary/30"
                      )}
                    >
                      <td className="p-4">
                        <div className="flex items-center gap-2">
                          {getRankIcon(entry.rank) || (
                            <span className="w-5 text-center text-muted-foreground">{entry.rank}</span>
                          )}
                        </div>
                      </td>
                      <td className="p-4">
                        <div className="flex items-center gap-3">
                          <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center overflow-hidden">
                            {entry.player.avatar_url ? (
                              <img src={entry.player.avatar_url} alt={entry.player.display_name} className="w-full h-full object-cover" />
                            ) : (
                              <span className="text-sm font-medium">{entry.player.display_name[0]}</span>
                            )}
                          </div>
                          <div>
                            <p className="font-medium">{entry.player.display_name}</p>
                            <p className="text-xs text-muted-foreground">Player #{entry.player_id}</p>
                          </div>
                        </div>
                      </td>
                      <td className="p-4 text-right">
                        <span className="font-mono font-semibold">{entry.score.toLocaleString()}</span>
                        <span className="text-muted-foreground text-sm ml-1">pts</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

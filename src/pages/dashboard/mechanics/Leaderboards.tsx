import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Trophy, Medal, Award } from 'lucide-react';
import { leaderboardUsers } from '@/lib/mockData';
import { cn } from '@/lib/utils';

export default function Leaderboards() {
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

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Leaderboards</h1>
          <p className="text-muted-foreground mt-1">View and manage competitive rankings.</p>
        </div>
      </div>

      {/* Podium */}
      <div className="flex justify-center items-end gap-4 py-8">
        {/* Second Place */}
        <div className="text-center">
          <div className="w-20 h-20 rounded-full bg-gray-400/20 border-2 border-gray-400 flex items-center justify-center mx-auto mb-3">
            <span className="text-2xl font-bold">{leaderboardUsers[1].avatar}</span>
          </div>
          <div className="bg-gray-400/20 rounded-t-lg px-6 py-4 border border-gray-400/30">
            <Medal className="w-8 h-8 text-gray-400 mx-auto mb-2" />
            <p className="font-semibold">{leaderboardUsers[1].name}</p>
            <p className="text-sm text-muted-foreground">{leaderboardUsers[1].score.toLocaleString()} pts</p>
          </div>
          <div className="h-24 bg-gray-400/10 rounded-b-lg border-x border-b border-gray-400/30" />
        </div>

        {/* First Place */}
        <div className="text-center -mt-8">
          <div className="w-24 h-24 rounded-full bg-yellow-500/20 border-2 border-yellow-500 flex items-center justify-center mx-auto mb-3 animate-glow">
            <span className="text-3xl font-bold">{leaderboardUsers[0].avatar}</span>
          </div>
          <div className="bg-yellow-500/20 rounded-t-lg px-8 py-4 border border-yellow-500/30">
            <Trophy className="w-10 h-10 text-yellow-500 mx-auto mb-2" />
            <p className="font-bold text-lg">{leaderboardUsers[0].name}</p>
            <p className="text-sm text-muted-foreground">{leaderboardUsers[0].score.toLocaleString()} pts</p>
          </div>
          <div className="h-32 bg-yellow-500/10 rounded-b-lg border-x border-b border-yellow-500/30" />
        </div>

        {/* Third Place */}
        <div className="text-center">
          <div className="w-20 h-20 rounded-full bg-amber-700/20 border-2 border-amber-700 flex items-center justify-center mx-auto mb-3">
            <span className="text-2xl font-bold">{leaderboardUsers[2].avatar}</span>
          </div>
          <div className="bg-amber-700/20 rounded-t-lg px-6 py-4 border border-amber-700/30">
            <Award className="w-8 h-8 text-amber-700 mx-auto mb-2" />
            <p className="font-semibold">{leaderboardUsers[2].name}</p>
            <p className="text-sm text-muted-foreground">{leaderboardUsers[2].score.toLocaleString()} pts</p>
          </div>
          <div className="h-16 bg-amber-700/10 rounded-b-lg border-x border-b border-amber-700/30" />
        </div>
      </div>

      {/* Full Leaderboard Table */}
      <Card>
        <CardHeader>
          <CardTitle>Full Rankings</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground w-20">Rank</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">User</th>
                  <th className="text-right p-4 text-sm font-medium text-muted-foreground">Score</th>
                </tr>
              </thead>
              <tbody>
                {leaderboardUsers.map((user) => (
                  <tr
                    key={user.userId}
                    className={cn(
                      "border-b border-border/50 transition-colors",
                      user.rank <= 3 ? getRankStyle(user.rank) : "hover:bg-secondary/30"
                    )}
                  >
                    <td className="p-4">
                      <div className="flex items-center gap-2">
                        {getRankIcon(user.rank) || (
                          <span className="w-5 text-center text-muted-foreground">{user.rank}</span>
                        )}
                      </div>
                    </td>
                    <td className="p-4">
                      <div className="flex items-center gap-3">
                        <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center">
                          <span className="text-sm font-medium">{user.avatar}</span>
                        </div>
                        <div>
                          <p className="font-medium">{user.name}</p>
                          <p className="text-xs text-muted-foreground">{user.userId}</p>
                        </div>
                      </div>
                    </td>
                    <td className="p-4 text-right">
                      <span className="font-mono font-semibold">{user.score.toLocaleString()}</span>
                      <span className="text-muted-foreground text-sm ml-1">pts</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

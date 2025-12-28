import React from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Trophy, Target, Zap, Calendar, TrendingUp, Award } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { players } from '@/lib/mockData';
import { cn } from '@/lib/utils';

// Extended mock data for comparison
const getPlayerDetails = (playerId: string) => {
  const player = players.find(p => p.id === playerId);
  if (!player) return null;

  // Generate consistent mock stats based on player ID
  const seed = playerId.charCodeAt(playerId.length - 1);
  
  return {
    ...player,
    stats: {
      totalPoints: player.totalXp,
      badgesEarned: 3 + (seed % 7),
      missionsCompleted: 5 + (seed % 15),
      currentStreak: 2 + (seed % 25),
      loginDays: 15 + (seed % 60),
      redemptions: seed % 10,
    },
    badges: [
      { name: 'First Steps', icon: '🚀', earned: true },
      { name: 'Early Bird', icon: '🌅', earned: seed % 2 === 0 },
      { name: 'Shopaholic', icon: '🛒', earned: seed % 3 === 0 },
      { name: 'Streak Master', icon: '🔥', earned: seed > 50 },
      { name: 'Social Butterfly', icon: '🦋', earned: seed % 4 === 0 },
      { name: 'Power User', icon: '⚡', earned: seed > 55 },
    ],
    progress: {
      currentLevelXp: player.totalXp % 10000,
      nextLevelXp: 10000,
      levelProgress: (player.totalXp % 10000) / 100,
    },
  };
};

const getLevelColor = (level: string) => {
  switch (level.toLowerCase()) {
    case 'diamond': return 'border-cyan-400 text-cyan-400 bg-cyan-400/10';
    case 'platinum': return 'border-slate-300 text-slate-300 bg-slate-300/10';
    case 'gold': return 'border-yellow-500 text-yellow-500 bg-yellow-500/10';
    case 'silver': return 'border-gray-400 text-gray-400 bg-gray-400/10';
    default: return 'border-amber-700 text-amber-700 bg-amber-700/10';
  }
};

interface StatComparisonProps {
  label: string;
  icon: React.ReactNode;
  player1Value: number;
  player2Value: number;
  format?: (val: number) => string;
}

function StatComparison({ label, icon, player1Value, player2Value, format = (v) => v.toLocaleString() }: StatComparisonProps) {
  const p1Better = player1Value > player2Value;
  const p2Better = player2Value > player1Value;

  return (
    <div className="grid grid-cols-3 items-center gap-4 py-3 border-b border-border/50 last:border-0">
      <div className={cn("text-right font-mono text-lg", p1Better && "text-primary font-bold")}>
        {format(player1Value)}
        {p1Better && <span className="ml-2 text-xs text-primary">↑</span>}
      </div>
      <div className="flex items-center justify-center gap-2 text-muted-foreground">
        {icon}
        <span className="text-sm">{label}</span>
      </div>
      <div className={cn("font-mono text-lg", p2Better && "text-primary font-bold")}>
        {format(player2Value)}
        {p2Better && <span className="ml-2 text-xs text-primary">↑</span>}
      </div>
    </div>
  );
}

export default function PlayerComparison() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  
  const playerIds = searchParams.get('players')?.split(',') || [];
  
  if (playerIds.length !== 2) {
    return (
      <div className="flex flex-col items-center justify-center h-96 space-y-4">
        <p className="text-muted-foreground">Please select exactly 2 players to compare.</p>
        <Button onClick={() => navigate('/players')}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back to Players
        </Button>
      </div>
    );
  }

  const player1 = getPlayerDetails(playerIds[0]);
  const player2 = getPlayerDetails(playerIds[1]);

  if (!player1 || !player2) {
    return (
      <div className="flex flex-col items-center justify-center h-96 space-y-4">
        <p className="text-muted-foreground">One or more players not found.</p>
        <Button onClick={() => navigate('/players')}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back to Players
        </Button>
      </div>
    );
  }

  const allBadges = [...new Set([...player1.badges.map(b => b.name), ...player2.badges.map(b => b.name)])];

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/players')}>
          <ArrowLeft className="w-5 h-5" />
        </Button>
        <div>
          <h1 className="text-3xl font-bold">Player Comparison</h1>
          <p className="text-muted-foreground mt-1">Compare stats, badges, and progress side by side.</p>
        </div>
      </div>

      {/* Player Headers */}
      <div className="grid grid-cols-2 gap-6">
        {[player1, player2].map((player) => (
          <Card key={player.id} className="border-primary/20">
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="w-16 h-16 rounded-full bg-primary/10 flex items-center justify-center text-2xl font-bold">
                  {player.email[0].toUpperCase()}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="font-semibold truncate">{player.email}</p>
                  <p className="text-xs text-muted-foreground">{player.id}</p>
                  <Badge variant="outline" className={cn("mt-2 capitalize", getLevelColor(player.level))}>
                    {player.level}
                  </Badge>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Stats Comparison */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <TrendingUp className="w-5 h-5" />
            Stats Comparison
          </CardTitle>
        </CardHeader>
        <CardContent>
          <StatComparison 
            label="Total XP" 
            icon={<Zap className="w-4 h-4" />}
            player1Value={player1.stats.totalPoints}
            player2Value={player2.stats.totalPoints}
          />
          <StatComparison 
            label="Badges Earned" 
            icon={<Award className="w-4 h-4" />}
            player1Value={player1.stats.badgesEarned}
            player2Value={player2.stats.badgesEarned}
          />
          <StatComparison 
            label="Missions Completed" 
            icon={<Target className="w-4 h-4" />}
            player1Value={player1.stats.missionsCompleted}
            player2Value={player2.stats.missionsCompleted}
          />
          <StatComparison 
            label="Current Streak" 
            icon={<Trophy className="w-4 h-4" />}
            player1Value={player1.stats.currentStreak}
            player2Value={player2.stats.currentStreak}
            format={(v) => `${v} days`}
          />
          <StatComparison 
            label="Login Days" 
            icon={<Calendar className="w-4 h-4" />}
            player1Value={player1.stats.loginDays}
            player2Value={player2.stats.loginDays}
          />
        </CardContent>
      </Card>

      {/* Level Progress */}
      <Card>
        <CardHeader>
          <CardTitle>Level Progress</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-6">
            {[player1, player2].map((player) => (
              <div key={player.id} className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">Progress to next level</span>
                  <span className="font-mono">{player.progress.currentLevelXp.toLocaleString()} / {player.progress.nextLevelXp.toLocaleString()} XP</span>
                </div>
                <Progress value={player.progress.levelProgress} className="h-2" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Badges Comparison */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Award className="w-5 h-5" />
            Badges Comparison
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              {player1.badges.map((badge) => (
                <div 
                  key={badge.name} 
                  className={cn(
                    "flex items-center gap-2 p-2 rounded-lg",
                    badge.earned ? "bg-primary/10" : "opacity-30"
                  )}
                >
                  <span className="text-xl">{badge.icon}</span>
                  <span className="text-sm">{badge.name}</span>
                </div>
              ))}
            </div>
            <div className="flex items-center justify-center">
              <div className="w-px h-full bg-border" />
            </div>
            <div className="space-y-2">
              {player2.badges.map((badge) => (
                <div 
                  key={badge.name} 
                  className={cn(
                    "flex items-center gap-2 p-2 rounded-lg",
                    badge.earned ? "bg-primary/10" : "opacity-30"
                  )}
                >
                  <span className="text-xl">{badge.icon}</span>
                  <span className="text-sm">{badge.name}</span>
                </div>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

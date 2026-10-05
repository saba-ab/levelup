import React from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Trophy, Target, Zap, Coins, TrendingUp, Award } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { cn } from '@/lib/utils';
import { getPlayerName } from '@/lib/player-utils';
import { PlayerAvatar, LevelBadge } from '@/components/players';
import {
  usePlayerQuery,
  usePlayerLevelQuery,
  usePlayerWalletQuery,
  usePlayerBadgesQuery,
  usePlayerMissionsQuery,
  usePlayerStreaksQuery,
} from '@/services/queries/players';
import type { ID } from '@/services/api/types';

/** Everything the comparison shows for one player, from the real endpoints. */
function usePlayerSnapshot(playerId: ID | undefined) {
  const player = usePlayerQuery(playerId);
  const progress = usePlayerLevelQuery(playerId);
  const wallet = usePlayerWalletQuery(playerId);
  const badges = usePlayerBadgesQuery(playerId);
  const missions = usePlayerMissionsQuery(playerId);
  const streaks = usePlayerStreaksQuery(playerId);

  const streakList = streaks.data ?? [];
  return {
    player: player.data,
    isLoading: player.isLoading,
    isError: player.isError,
    progress: progress.data,
    badges: badges.data ?? [],
    stats: {
      totalXp: progress.data?.total_xp ?? 0,
      balance: wallet.data?.balance ?? 0,
      lifetimeEarned: wallet.data?.lifetime_earned ?? 0,
      badgesEarned: badges.data?.length ?? 0,
      missionsCompleted: (missions.data ?? []).filter((m) => m.status === 'completed').length,
      currentStreak: streakList.reduce((max, s) => Math.max(max, s.current_count), 0),
    },
  };
}

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
      <div className={cn('text-right font-mono text-lg', p1Better && 'text-primary font-bold')}>
        {format(player1Value)}
        {p1Better && <span className="ml-2 text-xs text-primary">↑</span>}
      </div>
      <div className="flex items-center justify-center gap-2 text-muted-foreground">
        {icon}
        <span className="text-sm">{label}</span>
      </div>
      <div className={cn('font-mono text-lg', p2Better && 'text-primary font-bold')}>
        {format(player2Value)}
        {p2Better && <span className="ml-2 text-xs text-primary">↑</span>}
      </div>
    </div>
  );
}

export default function PlayerComparison() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();

  const playerIds = searchParams.get('players')?.split(',').filter(Boolean) || [];
  const validPair = playerIds.length === 2;

  const p1 = usePlayerSnapshot(validPair ? playerIds[0] : undefined);
  const p2 = usePlayerSnapshot(validPair ? playerIds[1] : undefined);

  const backButton = (
    <Button onClick={() => navigate('/players')}>
      <ArrowLeft className="w-4 h-4 mr-2" />
      Back to Players
    </Button>
  );

  if (!validPair) {
    return (
      <div className="flex flex-col items-center justify-center h-96 space-y-4">
        <p className="text-muted-foreground">Please select exactly 2 players to compare.</p>
        {backButton}
      </div>
    );
  }

  if (p1.isLoading || p2.isLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <p className="text-muted-foreground">Loading players...</p>
      </div>
    );
  }

  if (!p1.player || !p2.player) {
    return (
      <div className="flex flex-col items-center justify-center h-96 space-y-4">
        <p className="text-muted-foreground">One or more players not found.</p>
        {backButton}
      </div>
    );
  }

  const snapshots = [p1, p2];
  const badgeNames = new Map<string, string>();
  snapshots.forEach((s) =>
    s.badges.forEach((pb) => badgeNames.set(String(pb.badge_id), pb.badge?.name ?? 'Badge')),
  );
  const earnedBy = snapshots.map((s) => new Set(s.badges.map((pb) => String(pb.badge_id))));
  const allBadges = [...badgeNames.entries()];

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
        {snapshots.map(({ player, progress }) => {
          const name = getPlayerName(player!);
          return (
            <Card key={player!.id} className="border-primary/20 cursor-pointer" onClick={() => navigate(`/players/${player!.id}`)}>
              <CardContent className="pt-6">
                <div className="flex items-center gap-4">
                  <PlayerAvatar name={name} className="w-16 h-16 text-2xl font-bold" />
                  <div className="flex-1 min-w-0">
                    <p className="font-semibold truncate">{name}</p>
                    <p className="text-xs text-muted-foreground font-mono truncate">{player!.external_id}</p>
                    <div className="flex flex-wrap gap-2 mt-2">
                      {progress?.current_level && (
                        <LevelBadge
                          level={progress.current_level.name || `Level ${progress.current_level.level_number}`}
                          levelNumber={progress.current_level.level_number}
                          size="sm"
                        />
                      )}
                      {!player!.is_active && <Badge variant="secondary">Inactive</Badge>}
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          );
        })}
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
          <StatComparison label="Total XP" icon={<Zap className="w-4 h-4" />} player1Value={p1.stats.totalXp} player2Value={p2.stats.totalXp} />
          <StatComparison label="Points Balance" icon={<Coins className="w-4 h-4" />} player1Value={p1.stats.balance} player2Value={p2.stats.balance} />
          <StatComparison label="Points Earned" icon={<Coins className="w-4 h-4" />} player1Value={p1.stats.lifetimeEarned} player2Value={p2.stats.lifetimeEarned} />
          <StatComparison label="Badges Earned" icon={<Award className="w-4 h-4" />} player1Value={p1.stats.badgesEarned} player2Value={p2.stats.badgesEarned} />
          <StatComparison label="Missions Completed" icon={<Target className="w-4 h-4" />} player1Value={p1.stats.missionsCompleted} player2Value={p2.stats.missionsCompleted} />
          <StatComparison
            label="Current Streak"
            icon={<Trophy className="w-4 h-4" />}
            player1Value={p1.stats.currentStreak}
            player2Value={p2.stats.currentStreak}
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
            {snapshots.map(({ player, progress }) => (
              <div key={player!.id} className="space-y-2">
                <div className="flex justify-between text-sm gap-2">
                  <span className="text-muted-foreground">Progress to next level</span>
                  <span className="font-mono">
                    {progress?.next_level
                      ? `${progress.total_xp.toLocaleString()} / ${progress.next_level.xp_required.toLocaleString()} XP`
                      : `${(progress?.total_xp ?? 0).toLocaleString()} XP`}
                  </span>
                </div>
                <Progress value={progress?.next_level ? progress.progress_percent : progress?.current_level ? 100 : 0} className="h-2" />
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
          {allBadges.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-4">Neither player has earned a badge yet.</p>
          ) : (
            <div className="grid grid-cols-2 gap-6 divide-x divide-border">
              {earnedBy.map((earned, owner) => (
                <div key={owner} className={cn('space-y-2', owner === 1 && 'pl-6')}>
                  {allBadges.map(([badgeId, badgeName]) => (
                    <div
                      key={badgeId}
                      className={cn(
                        'flex items-center gap-2 p-2 rounded-lg',
                        earned.has(badgeId) ? 'bg-primary/10' : 'opacity-30'
                      )}
                    >
                      <Award className="w-4 h-4" />
                      <span className="text-sm truncate">{badgeName}</span>
                    </div>
                  ))}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

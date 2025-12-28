import { useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Trophy, Flame, Target, TrendingUp, Award, Calendar } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { players, badges } from '@/lib/mockData';
import { cn } from '@/lib/utils';
import { useExport } from '@/hooks/useExport';
import { calculateLevelProgress, getActivityIcon } from '@/lib/player-utils';
import { PlayerAvatar, LevelBadge, StatCard, ExportMenu } from '@/components/players';

// Player-specific data (would come from API in production)
const playerStats: Record<string, { currentStreak: number; longestStreak: number; missionsCompleted: number; badgesEarned: number; totalActions: number }> = {
  usr_001: { currentStreak: 12, longestStreak: 45, missionsCompleted: 28, badgesEarned: 8, totalActions: 1247 },
  usr_002: { currentStreak: 7, longestStreak: 21, missionsCompleted: 15, badgesEarned: 5, totalActions: 892 },
  usr_003: { currentStreak: 3, longestStreak: 14, missionsCompleted: 9, badgesEarned: 4, totalActions: 456 },
  usr_004: { currentStreak: 0, longestStreak: 8, missionsCompleted: 6, badgesEarned: 3, totalActions: 312 },
  usr_005: { currentStreak: 5, longestStreak: 12, missionsCompleted: 4, badgesEarned: 2, totalActions: 189 },
};

const playerBadges: Record<string, string[]> = {
  usr_001: ['1', '2', '3', '4', '5', '6', '7', '8'],
  usr_002: ['1', '2', '3', '5', '7'],
  usr_003: ['1', '2', '3', '7'],
  usr_004: ['1', '2', '7'],
  usr_005: ['1', '2'],
};

const playerActivity: Record<string, Array<{ id: string; action: string; timestamp: string; type: string }>> = {
  usr_001: [
    { id: '1', action: 'Earned 150 XP from daily login', timestamp: '2 hours ago', type: 'points' },
    { id: '2', action: 'Unlocked "Streak Master" badge', timestamp: '1 day ago', type: 'badge' },
    { id: '3', action: 'Completed "Welcome Journey" mission', timestamp: '2 days ago', type: 'mission' },
    { id: '4', action: 'Reached Diamond tier', timestamp: '3 days ago', type: 'level' },
    { id: '5', action: 'Earned 500 XP bonus', timestamp: '4 days ago', type: 'points' },
    { id: '6', action: 'Redeemed "Free Shipping" reward', timestamp: '5 days ago', type: 'reward' },
  ],
  usr_002: [
    { id: '1', action: 'Earned 100 XP from purchase', timestamp: '5 hours ago', type: 'points' },
    { id: '2', action: 'Unlocked "Social Butterfly" badge', timestamp: '2 days ago', type: 'badge' },
    { id: '3', action: 'Achieved 7-day streak', timestamp: '3 days ago', type: 'streak' },
  ],
  usr_003: [
    { id: '1', action: 'Earned 50 XP from review', timestamp: '1 day ago', type: 'points' },
    { id: '2', action: 'Reached Gold tier', timestamp: '5 days ago', type: 'level' },
  ],
  usr_004: [{ id: '1', action: 'Earned 25 XP from login', timestamp: '3 days ago', type: 'points' }],
  usr_005: [
    { id: '1', action: 'Earned 75 XP from referral', timestamp: '1 day ago', type: 'points' },
    { id: '2', action: 'Achieved 5-day streak', timestamp: '2 days ago', type: 'streak' },
  ],
};

export default function PlayerProfile() {
  const { playerId } = useParams();
  const navigate = useNavigate();
  const profileRef = useRef<HTMLDivElement>(null);

  const player = players.find((p) => p.id === playerId);
  const stats = playerStats[playerId as keyof typeof playerStats];
  const earnedBadgeIds = playerBadges[playerId as keyof typeof playerBadges] || [];
  const activity = playerActivity[playerId as keyof typeof playerActivity] || [];

  const { isExporting, exportAsImage, exportAsPDF } = useExport(profileRef, {
    filenamePrefix: `player-${playerId}-profile`,
  });

  if (!player) {
    return (
      <div className="space-y-6 animate-fade-in">
        <Button variant="ghost" onClick={() => navigate('/players')} className="gap-2">
          <ArrowLeft className="w-4 h-4" />
          Back to Players
        </Button>
        <div className="text-center py-12">
          <p className="text-muted-foreground">Player not found</p>
        </div>
      </div>
    );
  }

  const levelProgress = calculateLevelProgress(player.totalXp, player.level);

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex items-center justify-between">
        <Button variant="ghost" onClick={() => navigate('/players')} className="gap-2">
          <ArrowLeft className="w-4 h-4" />
          Back to Players
        </Button>
        <ExportMenu isExporting={isExporting} onExportImage={exportAsImage} onExportPDF={exportAsPDF} />
      </div>

      <div ref={profileRef} className="space-y-6 p-4 -m-4">
        {/* Player Header */}
        <div className="flex items-start gap-6">
          <PlayerAvatar email={player.email} size="lg" className="bg-primary/20" />
          <div className="flex-1">
            <h1 className="text-3xl font-bold">{player.email}</h1>
            <p className="text-muted-foreground font-mono text-sm mt-1">{player.id}</p>
            <div className="flex items-center gap-4 mt-3">
              <LevelBadge level={player.level} size="lg" />
              <span className="text-muted-foreground text-sm">Last active: {player.lastActive}</span>
            </div>
          </div>
        </div>

        {/* XP Progress */}
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium">Level Progress</span>
              <span className="text-sm text-muted-foreground">
                {player.totalXp.toLocaleString()} / {levelProgress.nextLevelXp.toLocaleString()} XP
              </span>
            </div>
            <Progress value={levelProgress.progress} className="h-3" />
            {player.level.toLowerCase() !== 'diamond' && levelProgress.xpToNext && (
              <p className="text-xs text-muted-foreground mt-2">
                {levelProgress.xpToNext.toLocaleString()} XP until next level
              </p>
            )}
          </CardContent>
        </Card>

        {/* Stats Grid */}
        <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
          <StatCard
            icon={<TrendingUp className="w-6 h-6" />}
            value={player.totalXp}
            label="Total XP"
            iconClassName="text-primary"
          />
          <StatCard
            icon={<Flame className="w-6 h-6" />}
            value={stats?.currentStreak || 0}
            label="Current Streak"
            iconClassName="text-orange-500"
          />
          <StatCard
            icon={<Trophy className="w-6 h-6" />}
            value={stats?.longestStreak || 0}
            label="Longest Streak"
            iconClassName="text-yellow-500"
          />
          <StatCard
            icon={<Target className="w-6 h-6" />}
            value={stats?.missionsCompleted || 0}
            label="Missions"
            iconClassName="text-green-500"
          />
          <StatCard
            icon={<Award className="w-6 h-6" />}
            value={stats?.badgesEarned || 0}
            label="Badges"
            iconClassName="text-purple-500"
          />
        </div>

        <div className="grid md:grid-cols-2 gap-6">
          {/* Badges Earned */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg flex items-center gap-2">
                <Award className="w-5 h-5" />
                Badges Earned
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-4 gap-3">
                {badges.map((badge) => {
                  const isEarned = earnedBadgeIds.includes(badge.id);
                  return (
                    <div
                      key={badge.id}
                      className={cn(
                        'aspect-square rounded-lg flex flex-col items-center justify-center p-2 transition-all',
                        isEarned ? 'bg-secondary' : 'bg-secondary/30 opacity-40 grayscale'
                      )}
                      title={`${badge.name}: ${badge.description}`}
                    >
                      <span className="text-2xl">{badge.icon}</span>
                      <span className="text-[10px] text-center mt-1 text-muted-foreground line-clamp-1">
                        {badge.name}
                      </span>
                    </div>
                  );
                })}
              </div>
            </CardContent>
          </Card>

          {/* Activity History */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg flex items-center gap-2">
                <Calendar className="w-5 h-5" />
                Recent Activity
              </CardTitle>
            </CardHeader>
            <CardContent>
              {activity.length > 0 ? (
                <div className="space-y-3">
                  {activity.map((item) => (
                    <div
                      key={item.id}
                      className="flex items-start gap-3 p-2 rounded-lg hover:bg-secondary/50 transition-colors"
                    >
                      <span className="text-lg">{getActivityIcon(item.type)}</span>
                      <div className="flex-1 min-w-0">
                        <p className="text-sm">{item.action}</p>
                        <p className="text-xs text-muted-foreground">{item.timestamp}</p>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-4">No recent activity</p>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

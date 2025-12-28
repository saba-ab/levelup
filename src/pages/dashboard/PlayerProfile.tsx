import React, { useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Trophy, Flame, Target, Calendar, TrendingUp, Award, Download, ImageIcon, FileText, Loader2 } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { players, badges } from '@/lib/mockData';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

const playerStats = {
  usr_001: { currentStreak: 12, longestStreak: 45, missionsCompleted: 28, badgesEarned: 8, totalActions: 1247 },
  usr_002: { currentStreak: 7, longestStreak: 21, missionsCompleted: 15, badgesEarned: 5, totalActions: 892 },
  usr_003: { currentStreak: 3, longestStreak: 14, missionsCompleted: 9, badgesEarned: 4, totalActions: 456 },
  usr_004: { currentStreak: 0, longestStreak: 8, missionsCompleted: 6, badgesEarned: 3, totalActions: 312 },
  usr_005: { currentStreak: 5, longestStreak: 12, missionsCompleted: 4, badgesEarned: 2, totalActions: 189 },
};

const playerBadges = {
  usr_001: ['1', '2', '3', '4', '5', '6', '7', '8'],
  usr_002: ['1', '2', '3', '5', '7'],
  usr_003: ['1', '2', '3', '7'],
  usr_004: ['1', '2', '7'],
  usr_005: ['1', '2'],
};

const playerActivity = {
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
  usr_004: [
    { id: '1', action: 'Earned 25 XP from login', timestamp: '3 days ago', type: 'points' },
  ],
  usr_005: [
    { id: '1', action: 'Earned 75 XP from referral', timestamp: '1 day ago', type: 'points' },
    { id: '2', action: 'Achieved 5-day streak', timestamp: '2 days ago', type: 'streak' },
  ],
};

export default function PlayerProfile() {
  const { playerId } = useParams();
  const navigate = useNavigate();
  const profileRef = useRef<HTMLDivElement>(null);
  const [isExporting, setIsExporting] = useState(false);

  const player = players.find(p => p.id === playerId);
  const stats = playerStats[playerId as keyof typeof playerStats];
  const earnedBadgeIds = playerBadges[playerId as keyof typeof playerBadges] || [];
  const activity = playerActivity[playerId as keyof typeof playerActivity] || [];

  const exportAsImage = async () => {
    if (!profileRef.current) return;
    setIsExporting(true);
    try {
      const html2canvas = (await import('html2canvas')).default;
      const canvas = await html2canvas(profileRef.current, {
        backgroundColor: '#0a0a0a',
        scale: 2,
      });
      const link = document.createElement('a');
      link.download = `player-${playerId}-profile.png`;
      link.href = canvas.toDataURL('image/png');
      link.click();
      toast.success('Profile exported as image');
    } catch (error) {
      toast.error('Failed to export image');
    } finally {
      setIsExporting(false);
    }
  };

  const exportAsPDF = async () => {
    if (!profileRef.current) return;
    setIsExporting(true);
    try {
      const html2canvas = (await import('html2canvas')).default;
      const { jsPDF } = await import('jspdf');
      const canvas = await html2canvas(profileRef.current, {
        backgroundColor: '#0a0a0a',
        scale: 2,
      });
      const imgData = canvas.toDataURL('image/png');
      const pdf = new jsPDF({
        orientation: canvas.width > canvas.height ? 'landscape' : 'portrait',
        unit: 'px',
        format: [canvas.width, canvas.height],
      });
      pdf.addImage(imgData, 'PNG', 0, 0, canvas.width, canvas.height);
      pdf.save(`player-${playerId}-profile.pdf`);
      toast.success('Profile exported as PDF');
    } catch (error) {
      toast.error('Failed to export PDF');
    } finally {
      setIsExporting(false);
    }
  };

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

  const getLevelColor = (level: string) => {
    switch (level.toLowerCase()) {
      case 'diamond': return 'border-cyan-400 text-cyan-400 bg-cyan-400/10';
      case 'platinum': return 'border-slate-300 text-slate-300 bg-slate-300/10';
      case 'gold': return 'border-yellow-500 text-yellow-500 bg-yellow-500/10';
      case 'silver': return 'border-gray-400 text-gray-400 bg-gray-400/10';
      default: return 'border-amber-700 text-amber-700 bg-amber-700/10';
    }
  };

  const getActivityIcon = (type: string) => {
    switch (type) {
      case 'points': return '⚡';
      case 'badge': return '🏅';
      case 'mission': return '🎯';
      case 'level': return '🎖️';
      case 'streak': return '🔥';
      case 'reward': return '🎁';
      default: return '📌';
    }
  };

  const nextLevelXp = player.level === 'Diamond' ? player.totalXp : 
    player.level === 'Platinum' ? 50000 :
    player.level === 'Gold' ? 15000 :
    player.level === 'Silver' ? 5000 : 1000;

  const currentLevelXp = player.level === 'Diamond' ? 50000 :
    player.level === 'Platinum' ? 15000 :
    player.level === 'Gold' ? 5000 :
    player.level === 'Silver' ? 1000 : 0;

  const progressToNext = player.level === 'Diamond' ? 100 :
    ((player.totalXp - currentLevelXp) / (nextLevelXp - currentLevelXp)) * 100;

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex items-center justify-between">
        <Button variant="ghost" onClick={() => navigate('/players')} className="gap-2">
          <ArrowLeft className="w-4 h-4" />
          Back to Players
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" className="gap-2" disabled={isExporting}>
              {isExporting ? <Loader2 className="w-4 h-4 animate-spin" /> : <Download className="w-4 h-4" />}
              Export
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={exportAsImage} className="gap-2 cursor-pointer">
              <ImageIcon className="w-4 h-4" />
              Export as Image
            </DropdownMenuItem>
            <DropdownMenuItem onClick={exportAsPDF} className="gap-2 cursor-pointer">
              <FileText className="w-4 h-4" />
              Export as PDF
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <div ref={profileRef} className="space-y-6 p-4 -m-4">
        {/* Player Header */}
        <div className="flex items-start gap-6">
          <div className="w-20 h-20 rounded-full bg-primary/20 flex items-center justify-center text-3xl font-bold">
            {player.email[0].toUpperCase()}
          </div>
          <div className="flex-1">
            <h1 className="text-3xl font-bold">{player.email}</h1>
            <p className="text-muted-foreground font-mono text-sm mt-1">{player.id}</p>
            <div className="flex items-center gap-4 mt-3">
              <Badge variant="outline" className={cn("capitalize text-sm px-3 py-1", getLevelColor(player.level))}>
                {player.level}
              </Badge>
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
              {player.totalXp.toLocaleString()} / {nextLevelXp.toLocaleString()} XP
            </span>
          </div>
          <Progress value={progressToNext} className="h-3" />
          {player.level !== 'Diamond' && (
            <p className="text-xs text-muted-foreground mt-2">
              {(nextLevelXp - player.totalXp).toLocaleString()} XP until next level
            </p>
          )}
        </CardContent>
      </Card>

      {/* Stats Grid */}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
        <Card>
          <CardContent className="pt-6 text-center">
            <TrendingUp className="w-6 h-6 mx-auto mb-2 text-primary" />
            <p className="text-2xl font-bold">{player.totalXp.toLocaleString()}</p>
            <p className="text-xs text-muted-foreground">Total XP</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6 text-center">
            <Flame className="w-6 h-6 mx-auto mb-2 text-orange-500" />
            <p className="text-2xl font-bold">{stats?.currentStreak || 0}</p>
            <p className="text-xs text-muted-foreground">Current Streak</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6 text-center">
            <Trophy className="w-6 h-6 mx-auto mb-2 text-yellow-500" />
            <p className="text-2xl font-bold">{stats?.longestStreak || 0}</p>
            <p className="text-xs text-muted-foreground">Longest Streak</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6 text-center">
            <Target className="w-6 h-6 mx-auto mb-2 text-green-500" />
            <p className="text-2xl font-bold">{stats?.missionsCompleted || 0}</p>
            <p className="text-xs text-muted-foreground">Missions</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6 text-center">
            <Award className="w-6 h-6 mx-auto mb-2 text-purple-500" />
            <p className="text-2xl font-bold">{stats?.badgesEarned || 0}</p>
            <p className="text-xs text-muted-foreground">Badges</p>
          </CardContent>
        </Card>
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
                      "aspect-square rounded-lg flex flex-col items-center justify-center p-2 transition-all",
                      isEarned 
                        ? "bg-secondary" 
                        : "bg-secondary/30 opacity-40 grayscale"
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
                  <div key={item.id} className="flex items-start gap-3 p-2 rounded-lg hover:bg-secondary/50 transition-colors">
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

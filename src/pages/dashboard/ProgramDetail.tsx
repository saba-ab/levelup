import React, { useState, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  ArrowLeft,
  Play,
  Pause,
  StopCircle,
  Pencil,
  Users,
  UserPlus,
  UserMinus,
  Loader2,
  Calendar,
  Settings,
  Zap,
  ChevronLeft,
  ChevronRight,
  UsersRound,
  Award,
  Target,
  Gift,
} from 'lucide-react';
import { format } from 'date-fns';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { useToast } from '@/hooks/use-toast';
import { ProgramFormDialog } from '@/components/programs/ProgramFormDialog';
import { BulkEnrollDialog } from '@/components/programs/BulkEnrollDialog';
import {
  useProgramQuery,
  useProgramStatsQuery,
  useProgramPlayersQuery,
  useUpdateProgramMutation,
  useActivateProgramMutation,
  usePauseProgramMutation,
  useEndProgramMutation,
  useRemovePlayerFromProgramMutation,
  useBulkAddPlayersToProgramMutation,
} from '@/services/queries/programs';
import type { ProgramStatus, Player, UpdateProgramData } from '@/services/api/types';

const statusColors: Record<ProgramStatus, string> = {
  draft: 'border-amber-500/50 text-amber-500 bg-amber-500/10',
  active: 'border-green-500/50 text-green-500 bg-green-500/10',
  paused: 'border-blue-500/50 text-blue-500 bg-blue-500/10',
  ended: 'border-muted-foreground/50 text-muted-foreground bg-muted/50',
};

export default function ProgramDetail() {
  const { programId } = useParams<{ programId: string }>();
  const navigate = useNavigate();
  const { toast } = useToast();
  const id = Number(programId);

  const [isEditOpen, setIsEditOpen] = useState(false);
  const [isBulkEnrollOpen, setIsBulkEnrollOpen] = useState(false);
  const [removingPlayer, setRemovingPlayer] = useState<Player | null>(null);
  const [playersPage, setPlayersPage] = useState(1);

  // Queries
  const { data: program, isLoading: programLoading, error: programError } = useProgramQuery(id);
  const { data: stats, isLoading: statsLoading } = useProgramStatsQuery(id);
  const { data: playersData, isLoading: playersLoading } = useProgramPlayersQuery(id, playersPage, 10);

  // Mutations
  const updateMutation = useUpdateProgramMutation();
  const activateMutation = useActivateProgramMutation();
  const pauseMutation = usePauseProgramMutation();
  const endMutation = useEndProgramMutation();
  const removePlayerMutation = useRemovePlayerFromProgramMutation();
  const bulkAddMutation = useBulkAddPlayersToProgramMutation();

  const players = playersData?.data || [];
  const enrolledPlayerIds = useMemo(() => new Set(players.map(p => p.id)), [players]);

  const handleFormSubmit = async (data: UpdateProgramData) => {
    try {
      await updateMutation.mutateAsync({ programId: id, data });
      toast({ title: 'Program updated', description: 'Changes saved successfully.' });
      setIsEditOpen(false);
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to update program',
        variant: 'destructive',
      });
    }
  };

  const handleActivate = async () => {
    try {
      await activateMutation.mutateAsync(id);
      toast({ title: 'Program activated', description: `"${program?.name}" is now active.` });
    } catch (err) {
      toast({ title: 'Error', description: err instanceof Error ? err.message : 'Failed to activate', variant: 'destructive' });
    }
  };

  const handlePause = async () => {
    try {
      await pauseMutation.mutateAsync(id);
      toast({ title: 'Program paused', description: `"${program?.name}" has been paused.` });
    } catch (err) {
      toast({ title: 'Error', description: err instanceof Error ? err.message : 'Failed to pause', variant: 'destructive' });
    }
  };

  const handleEnd = async () => {
    try {
      await endMutation.mutateAsync(id);
      toast({ title: 'Program ended', description: `"${program?.name}" has been ended.` });
    } catch (err) {
      toast({ title: 'Error', description: err instanceof Error ? err.message : 'Failed to end', variant: 'destructive' });
    }
  };

  const handleBulkEnroll = async (playerIds: number[]) => {
    try {
      const result = await bulkAddMutation.mutateAsync({ programId: id, playerIds });
      toast({
        title: 'Players enrolled',
        description: `Successfully enrolled ${result?.successful || playerIds.length} players.`,
      });
      setIsBulkEnrollOpen(false);
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to enroll players',
        variant: 'destructive',
      });
    }
  };

  const handleRemovePlayer = async () => {
    if (!removingPlayer) return;
    try {
      await removePlayerMutation.mutateAsync({ programId: id, playerId: removingPlayer.id });
      toast({ title: 'Player removed', description: `${removingPlayer.display_name} has been removed from the program.` });
      setRemovingPlayer(null);
    } catch (err) {
      toast({ title: 'Error', description: err instanceof Error ? err.message : 'Failed to remove player', variant: 'destructive' });
    }
  };

  const isAnyMutating =
    updateMutation.isPending ||
    activateMutation.isPending ||
    pauseMutation.isPending ||
    endMutation.isPending ||
    removePlayerMutation.isPending ||
    bulkAddMutation.isPending;

  if (programLoading) {
    return (
      <div className="space-y-6 animate-fade-in">
        <div className="flex items-center gap-4">
          <Skeleton className="h-10 w-10 rounded-lg" />
          <div className="space-y-2">
            <Skeleton className="h-8 w-64" />
            <Skeleton className="h-4 w-48" />
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24 rounded-lg" />
          ))}
        </div>
      </div>
    );
  }

  if (programError || !program) {
    return (
      <div className="flex flex-col items-center justify-center py-16 space-y-4">
        <p className="text-destructive font-medium">Failed to load program</p>
        <p className="text-muted-foreground">{programError instanceof Error ? programError.message : 'Program not found'}</p>
        <Button variant="outline" onClick={() => navigate('/programs')}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back to Programs
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
        <div className="flex items-start gap-4">
          <Button variant="ghost" size="icon" onClick={() => navigate('/programs')}>
            <ArrowLeft className="w-5 h-5" />
          </Button>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-3xl font-bold">{program.name}</h1>
              <Badge variant="outline" className={cn('capitalize', statusColors[program.status])}>
                {program.status}
              </Badge>
            </div>
            {program.description && (
              <p className="text-muted-foreground mt-1">{program.description}</p>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2 ml-12 lg:ml-0">
          {program.status === 'draft' && (
            <Button onClick={handleActivate} disabled={isAnyMutating}>
              {activateMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <Play className="w-4 h-4 mr-2" />}
              Activate
            </Button>
          )}
          {program.status === 'active' && (
            <Button variant="outline" onClick={handlePause} disabled={isAnyMutating}>
              {pauseMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <Pause className="w-4 h-4 mr-2" />}
              Pause
            </Button>
          )}
          {program.status === 'paused' && (
            <>
              <Button onClick={handleActivate} disabled={isAnyMutating}>
                {activateMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <Play className="w-4 h-4 mr-2" />}
                Resume
              </Button>
              <Button variant="destructive" onClick={handleEnd} disabled={isAnyMutating}>
                {endMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <StopCircle className="w-4 h-4 mr-2" />}
                End
              </Button>
            </>
          )}
          <Button variant="outline" onClick={() => setIsEditOpen(true)} disabled={isAnyMutating}>
            <Pencil className="w-4 h-4 mr-2" />
            Edit
          </Button>
        </div>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
                <Users className="w-5 h-5 text-primary" />
              </div>
              <div>
                <p className="text-2xl font-bold">{statsLoading ? '—' : stats?.total_players?.toLocaleString() || 0}</p>
                <p className="text-xs text-muted-foreground">Total Players</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-green-500/10 flex items-center justify-center">
                <UsersRound className="w-5 h-5 text-green-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{statsLoading ? '—' : stats?.active_players?.toLocaleString() || 0}</p>
                <p className="text-xs text-muted-foreground">Active Players</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-amber-500/10 flex items-center justify-center">
                <Zap className="w-5 h-5 text-amber-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{statsLoading ? '—' : stats?.total_points_awarded?.toLocaleString() || 0}</p>
                <p className="text-xs text-muted-foreground">Points Awarded</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-purple-500/10 flex items-center justify-center">
                <Award className="w-5 h-5 text-purple-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{statsLoading ? '—' : stats?.total_badges_awarded?.toLocaleString() || 0}</p>
                <p className="text-xs text-muted-foreground">Badges Awarded</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center">
                <Target className="w-5 h-5 text-blue-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{statsLoading ? '—' : stats?.total_missions_completed?.toLocaleString() || 0}</p>
                <p className="text-xs text-muted-foreground">Missions Done</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-pink-500/10 flex items-center justify-center">
                <Gift className="w-5 h-5 text-pink-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{statsLoading ? '—' : stats?.total_rewards_redeemed?.toLocaleString() || 0}</p>
                <p className="text-xs text-muted-foreground">Rewards Redeemed</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Date Info */}
      <div className="grid grid-cols-2 gap-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-amber-500/10 flex items-center justify-center">
                <Calendar className="w-5 h-5 text-amber-500" />
              </div>
              <div>
                <p className="text-sm font-medium">
                  {program.start_date ? format(new Date(program.start_date), 'MMM d, yyyy') : 'Not set'}
                </p>
                <p className="text-sm text-muted-foreground">Start Date</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center">
                <Calendar className="w-5 h-5 text-blue-500" />
              </div>
              <div>
                <p className="text-sm font-medium">
                  {program.end_date ? format(new Date(program.end_date), 'MMM d, yyyy') : 'Not set'}
                </p>
                <p className="text-sm text-muted-foreground">End Date</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="players" className="space-y-4">
        <TabsList>
          <TabsTrigger value="players">
            <Users className="w-4 h-4 mr-2" />
            Players
          </TabsTrigger>
          <TabsTrigger value="settings">
            <Settings className="w-4 h-4 mr-2" />
            Settings
          </TabsTrigger>
        </TabsList>

        <TabsContent value="players" className="space-y-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle>Enrolled Players</CardTitle>
                <CardDescription>Manage players in this program</CardDescription>
              </div>
              <Button onClick={() => setIsBulkEnrollOpen(true)} disabled={program.status === 'ended'}>
                <UserPlus className="w-4 h-4 mr-2" />
                Add Players
              </Button>
            </CardHeader>
            <CardContent>
              {playersLoading ? (
                <div className="space-y-3">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <div key={i} className="flex items-center gap-3 p-3 border border-border/50 rounded-lg">
                      <Skeleton className="w-10 h-10 rounded-full" />
                      <div className="space-y-1 flex-1">
                        <Skeleton className="h-4 w-32" />
                        <Skeleton className="h-3 w-24" />
                      </div>
                    </div>
                  ))}
                </div>
              ) : players.length === 0 ? (
                <div className="text-center py-12">
                  <Users className="w-12 h-12 mx-auto text-muted-foreground/50" />
                  <p className="mt-4 text-muted-foreground">No players enrolled yet</p>
                  <Button className="mt-4" onClick={() => setIsBulkEnrollOpen(true)} disabled={program.status === 'ended'}>
                    <UserPlus className="w-4 h-4 mr-2" />
                    Add First Player
                  </Button>
                </div>
              ) : (
                <div className="space-y-2">
                  {players.map((player) => (
                    <div
                      key={player.id}
                      className="flex items-center justify-between p-3 border border-border/50 rounded-lg hover:bg-secondary/30 transition-colors"
                    >
                      <div className="flex items-center gap-3">
                        <Avatar>
                          <AvatarImage src={player.avatar_url} />
                          <AvatarFallback>
                            {player.display_name?.substring(0, 2).toUpperCase() || player.external_id.substring(0, 2).toUpperCase()}
                          </AvatarFallback>
                        </Avatar>
                        <div>
                          <p className="font-medium">{player.display_name || player.external_id}</p>
                          <p className="text-sm text-muted-foreground">{player.email}</p>
                        </div>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setRemovingPlayer(player)}
                        disabled={program.status === 'ended'}
                      >
                        <UserMinus className="w-4 h-4" />
                      </Button>
                    </div>
                  ))}
                  {/* Pagination */}
                  {playersData && playersData.meta.last_page > 1 && (
                    <div className="flex items-center justify-between pt-4 border-t border-border/50">
                      <p className="text-sm text-muted-foreground">
                        Showing {playersData.meta.from}–{playersData.meta.to} of {playersData.meta.total}
                      </p>
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="icon"
                          disabled={playersPage === 1}
                          onClick={() => setPlayersPage((p) => p - 1)}
                        >
                          <ChevronLeft className="w-4 h-4" />
                        </Button>
                        <span className="text-sm">
                          {playersPage} / {playersData.meta.last_page}
                        </span>
                        <Button
                          variant="outline"
                          size="icon"
                          disabled={playersPage === playersData.meta.last_page}
                          onClick={() => setPlayersPage((p) => p + 1)}
                        >
                          <ChevronRight className="w-4 h-4" />
                        </Button>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="settings" className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Program Settings */}
            <Card>
              <CardHeader>
                <CardTitle>Program Settings</CardTitle>
                <CardDescription>Configuration options for this program</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <Label>Allow Public Signup</Label>
                    <p className="text-sm text-muted-foreground">Players can join without invitation</p>
                  </div>
                  <Switch checked={program.settings?.allow_public_signup ?? false} disabled />
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <Label>Require Email Verification</Label>
                    <p className="text-sm text-muted-foreground">Players must verify email</p>
                  </div>
                  <Switch checked={program.settings?.require_email_verification ?? false} disabled />
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <Label>Welcome Points</Label>
                    <p className="text-sm text-muted-foreground">Points given on enrollment</p>
                  </div>
                  <Badge variant="secondary">{program.settings?.welcome_points ?? 0}</Badge>
                </div>
              </CardContent>
            </Card>

            {/* Mechanics */}
            <Card>
              <CardHeader>
                <CardTitle>Enabled Mechanics</CardTitle>
                <CardDescription>Gamification features for this program</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                {[
                  { key: 'points_enabled', label: 'Points' },
                  { key: 'badges_enabled', label: 'Badges' },
                  { key: 'levels_enabled', label: 'Levels' },
                  { key: 'missions_enabled', label: 'Missions' },
                  { key: 'streaks_enabled', label: 'Streaks' },
                  { key: 'leaderboards_enabled', label: 'Leaderboards' },
                  { key: 'rewards_enabled', label: 'Rewards' },
                ].map(({ key, label }) => (
                  <div key={key} className="flex items-center justify-between">
                    <Label>{label}</Label>
                    <Switch checked={Boolean((program.mechanics as unknown as Record<string, boolean>)?.[key])} disabled />
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>

      {/* Edit Dialog */}
      <ProgramFormDialog
        open={isEditOpen}
        onOpenChange={setIsEditOpen}
        program={program}
        onSubmit={handleFormSubmit}
        isLoading={updateMutation.isPending}
      />

      {/* Bulk Enroll Dialog */}
      <BulkEnrollDialog
        open={isBulkEnrollOpen}
        onOpenChange={setIsBulkEnrollOpen}
        enrolledPlayerIds={enrolledPlayerIds}
        onEnroll={handleBulkEnroll}
        isLoading={bulkAddMutation.isPending}
        programName={program.name}
      />

      {/* Remove Player Confirmation */}
      <AlertDialog open={!!removingPlayer} onOpenChange={() => setRemovingPlayer(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove Player</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to remove "{removingPlayer?.display_name || removingPlayer?.external_id}" from this program?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleRemovePlayer}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {removePlayerMutation.isPending ? 'Removing...' : 'Remove'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

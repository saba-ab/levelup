import { useState, useMemo } from 'react';
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
  ChevronLeft,
  ChevronRight,
  Hash,
} from 'lucide-react';
import { format } from 'date-fns';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
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
import { useAuth } from '@/contexts/AuthContext';
import { ProgramFormDialog } from '@/components/programs/ProgramFormDialog';
import { BulkEnrollDialog } from '@/components/programs/BulkEnrollDialog';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import {
  describeProgramError,
  useProgramQuery,
  useProgramPlayersQuery,
  useUpdateProgramMutation,
  useActivateProgramMutation,
  usePauseProgramMutation,
  useEndProgramMutation,
  useRemovePlayerFromProgramMutation,
  useBulkAddPlayersToProgramMutation,
} from '@/services/queries/programs';
import type { ID, ProgramMember, ProgramStatus, UpdateProgramData } from '@/services/api/types';

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
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:programs');
  const id: ID = programId ?? '';

  const [isEditOpen, setIsEditOpen] = useState(false);
  const [isBulkEnrollOpen, setIsBulkEnrollOpen] = useState(false);
  const [removingPlayer, setRemovingPlayer] = useState<ProgramMember | null>(null);
  const playersPager = useCursorPagination(10);

  // Queries
  const { data: program, isLoading: programLoading, error: programError } = useProgramQuery(programId);
  const { data: playersData, isLoading: playersLoading } = useProgramPlayersQuery(programId, {
    limit: playersPager.limit,
    cursor: playersPager.cursor,
  });

  // Mutations
  const updateMutation = useUpdateProgramMutation();
  const activateMutation = useActivateProgramMutation();
  const pauseMutation = usePauseProgramMutation();
  const endMutation = useEndProgramMutation();
  const removePlayerMutation = useRemovePlayerFromProgramMutation();
  const bulkAddMutation = useBulkAddPlayersToProgramMutation();

  const players = useMemo(() => playersData?.data ?? [], [playersData]);
  const playersNextCursor = playersData?.next_cursor ?? '';
  const enrolledPlayerIds = useMemo(() => new Set(players.map(m => m.player_id)), [players]);
  const memberName = (m: ProgramMember | null) => m?.player?.display_name || m?.player?.external_id || m?.player_id || '';

  const showError = (err: unknown, fallback: string) =>
    toast({ title: 'Error', description: describeProgramError(err, fallback), variant: 'destructive' });

  const handleFormSubmit = async (data: UpdateProgramData) => {
    try {
      await updateMutation.mutateAsync({ programId: id, data });
      toast({ title: 'Program updated', description: 'Changes saved successfully.' });
      setIsEditOpen(false);
    } catch (err) {
      showError(err, 'Failed to update program');
    }
  };

  const handleActivate = async () => {
    try {
      await activateMutation.mutateAsync(id);
      toast({ title: 'Program activated', description: `"${program?.name}" is now active.` });
    } catch (err) {
      showError(err, 'Failed to activate');
    }
  };

  const handlePause = async () => {
    try {
      await pauseMutation.mutateAsync(id);
      toast({ title: 'Program paused', description: `"${program?.name}" has been paused.` });
    } catch (err) {
      showError(err, 'Failed to pause');
    }
  };

  const handleEnd = async () => {
    try {
      await endMutation.mutateAsync(id);
      toast({ title: 'Program ended', description: `"${program?.name}" has been ended.` });
    } catch (err) {
      showError(err, 'Failed to end');
    }
  };

  const handleBulkEnroll = async (playerIds: ID[]) => {
    try {
      const result = await bulkAddMutation.mutateAsync({ programId: id, playerIds });
      const parts = [`${result.enrolled} enrolled`];
      if (result.alreadyEnrolled) parts.push(`${result.alreadyEnrolled} already enrolled`);
      if (result.failures.length) parts.push(`${result.failures.length} failed`);
      toast({
        title: result.failures.length ? 'Enrollment finished with errors' : 'Players enrolled',
        description: parts.join(', ') + '.',
        variant: result.failures.length && !result.enrolled ? 'destructive' : undefined,
      });
      if (result.failures.length === 0) setIsBulkEnrollOpen(false);
      return result;
    } catch (err) {
      showError(err, 'Failed to enroll players');
      return undefined;
    }
  };

  const handleRemovePlayer = async () => {
    if (!removingPlayer) return;
    try {
      await removePlayerMutation.mutateAsync({ programId: id, playerId: removingPlayer.player_id });
      toast({ title: 'Player removed', description: `${memberName(removingPlayer)} has been removed from the program.` });
      setRemovingPlayer(null);
    } catch (err) {
      showError(err, 'Failed to remove player');
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
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, i) => (
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
        <div className={cn('flex items-center gap-2 ml-12 lg:ml-0', !canManage && 'hidden')}>
          {program.status === 'draft' && (
            <Button onClick={handleActivate} disabled={isAnyMutating}>
              {activateMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <Play className="w-4 h-4 mr-2" />}
              Activate
            </Button>
          )}
          {program.status === 'active' && (
            <>
              <Button variant="outline" onClick={handlePause} disabled={isAnyMutating}>
                {pauseMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <Pause className="w-4 h-4 mr-2" />}
                Pause
              </Button>
              <Button variant="destructive" onClick={handleEnd} disabled={isAnyMutating}>
                {endMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin mr-2" /> : <StopCircle className="w-4 h-4 mr-2" />}
                End
              </Button>
            </>
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

      {/* Program Info */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-green-500/10 flex items-center justify-center">
                <Users className="w-5 h-5 text-green-500" />
              </div>
              <div>
                <p className="text-sm font-medium tabular-nums">{(program.member_count ?? 0).toLocaleString()}</p>
                <p className="text-sm text-muted-foreground">Enrolled players</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
                <Hash className="w-5 h-5 text-primary" />
              </div>
              <div className="min-w-0">
                <p className="text-sm font-medium font-mono truncate">{program.slug}</p>
                <p className="text-sm text-muted-foreground">Slug</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-amber-500/10 flex items-center justify-center">
                <Calendar className="w-5 h-5 text-amber-500" />
              </div>
              <div>
                <p className="text-sm font-medium">
                  {program.starts_at ? format(new Date(program.starts_at), 'MMM d, yyyy') : 'Not set'}
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
                  {program.ends_at ? format(new Date(program.ends_at), 'MMM d, yyyy') : 'Not set'}
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
                <CardTitle>Enrolled Players ({(program.member_count ?? 0).toLocaleString()})</CardTitle>
                <CardDescription>
                  {program.status === 'active'
                    ? 'Manage players in this program'
                    : 'Players can only be enrolled while the program is active'}
                </CardDescription>
              </div>
              <Button onClick={() => setIsBulkEnrollOpen(true)} disabled={!canManage || program.status !== 'active'}>
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
                  <Button
                    className="mt-4"
                    onClick={() => setIsBulkEnrollOpen(true)}
                    disabled={!canManage || program.status !== 'active'}
                  >
                    <UserPlus className="w-4 h-4 mr-2" />
                    Add First Player
                  </Button>
                </div>
              ) : (
                <div className="space-y-2">
                  {players.map((member) => (
                    <div
                      key={member.player_id}
                      className="flex items-center justify-between p-3 border border-border/50 rounded-lg hover:bg-secondary/30 transition-colors"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <Avatar>
                          <AvatarFallback>{memberName(member).substring(0, 2).toUpperCase()}</AvatarFallback>
                        </Avatar>
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <p className="font-medium truncate">{member.player ? memberName(member) : 'Unknown player'}</p>
                            {member.player && !member.player.active && (
                              <Badge variant="outline" className="text-xs">Inactive</Badge>
                            )}
                          </div>
                          <p className="text-sm text-muted-foreground truncate">
                            {member.player?.external_id ?? member.player_id}
                            {' · enrolled '}
                            {format(new Date(member.enrolled_at), 'MMM d, yyyy')}
                          </p>
                        </div>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        className={cn('text-destructive hover:text-destructive', !canManage && 'hidden')}
                        onClick={() => setRemovingPlayer(member)}
                      >
                        <UserMinus className="w-4 h-4" />
                      </Button>
                    </div>
                  ))}
                  {/* Pagination */}
                  {(playersPager.hasPrevious || !!playersNextCursor) && (
                    <div className="flex items-center justify-end gap-2 pt-4 border-t border-border/50">
                      <Button
                        variant="outline"
                        size="icon"
                        disabled={!playersPager.hasPrevious}
                        onClick={playersPager.previous}
                      >
                        <ChevronLeft className="w-4 h-4" />
                      </Button>
                      <span className="text-sm">Page {playersPager.page}</span>
                      <Button
                        variant="outline"
                        size="icon"
                        disabled={!playersNextCursor}
                        onClick={() => playersPager.next(playersNextCursor)}
                      >
                        <ChevronRight className="w-4 h-4" />
                      </Button>
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
                  <Switch checked={Boolean(program.settings.allow_public_signup)} disabled />
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <Label>Require Email Verification</Label>
                    <p className="text-sm text-muted-foreground">Players must verify email</p>
                  </div>
                  <Switch checked={Boolean(program.settings.require_email_verification)} disabled />
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <Label>Welcome Points</Label>
                    <p className="text-sm text-muted-foreground">Points given on enrollment</p>
                  </div>
                  <Badge variant="secondary">{program.settings.welcome_points ?? 0}</Badge>
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
                    <Switch checked={Boolean(program.mechanics[key])} disabled />
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
        onSubmit={(data) => handleFormSubmit(data as UpdateProgramData)}
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
              Are you sure you want to remove "{memberName(removingPlayer)}" from this program?
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

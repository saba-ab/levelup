import { useRef, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  ArrowLeft,
  Trophy,
  Flame,
  Target,
  TrendingUp,
  Award,
  Coins,
  Sparkles,
  Pencil,
  Power,
  Trash2,
  ArrowUpRight,
  ArrowDownRight,
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
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
import { useToast } from '@/hooks/use-toast';
import { useExport } from '@/hooks/useExport';
import {
  usePlayerQuery,
  usePlayerLevelQuery,
  usePlayerBadgesQuery,
  usePlayerMissionsQuery,
  usePlayerStreaksQuery,
  usePlayerWalletQuery,
  usePlayerXpGrantsQuery,
  useSetPlayerActiveMutation,
  useDeletePlayerMutation,
} from '@/services/queries/players';
import { formatDate, formatDateTime, getActivityIcon, getPlayerName } from '@/lib/player-utils';
import {
  PlayerAvatar,
  LevelBadge,
  StatCard,
  ExportMenu,
  PlayerFormDialog,
  WalletOperationDialog,
  WalletTransactionsCard,
  type WalletOperation,
} from '@/components/players';

export default function PlayerProfile() {
  const { playerId } = useParams<{ playerId: string }>();
  const navigate = useNavigate();
  const { toast } = useToast();
  const profileRef = useRef<HTMLDivElement>(null);

  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [walletOperation, setWalletOperation] = useState<WalletOperation | null>(null);

  const { data: player, isLoading, isError, error } = usePlayerQuery(playerId);
  const { data: progress } = usePlayerLevelQuery(playerId);
  const { data: wallet } = usePlayerWalletQuery(playerId);
  const { data: playerBadges = [] } = usePlayerBadgesQuery(playerId);
  const { data: playerMissions = [] } = usePlayerMissionsQuery(playerId);
  const { data: playerStreaks = [] } = usePlayerStreaksQuery(playerId);
  const { data: xpGrants } = usePlayerXpGrantsQuery(playerId, { limit: 10 });

  const setActiveMutation = useSetPlayerActiveMutation();
  const deleteMutation = useDeletePlayerMutation();

  const { isExporting, exportAsImage, exportAsPDF } = useExport(profileRef, {
    filenamePrefix: `player-${playerId}-profile`,
  });

  const backButton = (
    <Button variant="ghost" onClick={() => navigate('/players')} className="gap-2">
      <ArrowLeft className="w-4 h-4" />
      Back to Players
    </Button>
  );

  if (isLoading) {
    return (
      <div className="space-y-6 animate-fade-in">
        {backButton}
        <p className="text-center py-12 text-muted-foreground">Loading player...</p>
      </div>
    );
  }

  if (isError || !player) {
    return (
      <div className="space-y-6 animate-fade-in">
        {backButton}
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            {error instanceof Error ? error.message : 'Player not found'}
          </p>
        </div>
      </div>
    );
  }

  const name = getPlayerName(player);
  const currentStreak = playerStreaks.reduce((max, s) => Math.max(max, s.current_count), 0);
  const longestStreak = playerStreaks.reduce((max, s) => Math.max(max, s.longest_count), 0);
  const missionsCompleted = playerMissions.filter((m) => m.status === 'completed').length;
  const attributes = Object.entries(player.attributes ?? {});

  const handleToggleActive = async () => {
    try {
      await setActiveMutation.mutateAsync({ playerId: player.id, active: !player.is_active });
      toast({ title: player.is_active ? 'Player deactivated' : 'Player activated' });
    } catch {
      // the API client already showed the error
    }
  };

  const handleDelete = async () => {
    try {
      await deleteMutation.mutateAsync(player.id);
      toast({ title: 'Player deleted' });
      navigate('/players');
    } catch {
      // the API client already showed the error
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-wrap items-center justify-between gap-2">
        {backButton}
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" className="gap-2" onClick={() => setWalletOperation('credit')}>
            <ArrowUpRight className="w-4 h-4" />
            Credit
          </Button>
          <Button variant="outline" className="gap-2" onClick={() => setWalletOperation('debit')}>
            <ArrowDownRight className="w-4 h-4" />
            Debit
          </Button>
          <Button variant="outline" className="gap-2" onClick={() => setEditOpen(true)}>
            <Pencil className="w-4 h-4" />
            Edit
          </Button>
          <Button variant="outline" className="gap-2" onClick={handleToggleActive} disabled={setActiveMutation.isPending}>
            <Power className="w-4 h-4" />
            {player.is_active ? 'Deactivate' : 'Activate'}
          </Button>
          <Button variant="outline" className="gap-2 text-destructive" onClick={() => setDeleteOpen(true)}>
            <Trash2 className="w-4 h-4" />
            Delete
          </Button>
          <ExportMenu isExporting={isExporting} onExportImage={exportAsImage} onExportPDF={exportAsPDF} />
        </div>
      </div>

      <div ref={profileRef} className="space-y-6 p-4 -m-4">
        {/* Player Header */}
        <div className="flex items-start gap-6">
          <PlayerAvatar name={name} size="lg" className="bg-primary/20" />
          <div className="flex-1 min-w-0">
            <h1 className="text-3xl font-bold truncate">{name}</h1>
            <p className="text-muted-foreground font-mono text-sm mt-1">{player.external_id}</p>
            {player.email && <p className="text-muted-foreground text-sm">{player.email}</p>}
            <div className="flex flex-wrap items-center gap-3 mt-3">
              {progress?.current_level && (
                <LevelBadge
                  level={progress.current_level.name || `Level ${progress.current_level.level_number}`}
                  levelNumber={progress.current_level.level_number}
                  size="lg"
                />
              )}
              <Badge variant={player.is_active ? 'default' : 'secondary'}>
                {player.is_active ? 'Active' : 'Inactive'}
              </Badge>
              <span className="text-muted-foreground text-sm">Joined {formatDate(player.created_at)}</span>
            </div>
          </div>
        </div>

        {/* XP Progress */}
        {progress && (
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center justify-between mb-2">
                <span className="text-sm font-medium">Level Progress</span>
                <span className="text-sm text-muted-foreground">
                  {progress.next_level
                    ? `${progress.total_xp.toLocaleString()} / ${progress.next_level.xp_required.toLocaleString()} XP`
                    : `${progress.total_xp.toLocaleString()} XP`}
                </span>
              </div>
              <Progress value={progress.next_level ? progress.progress_percent : 100} className="h-3" />
              <p className="text-xs text-muted-foreground mt-2">
                {progress.next_level && progress.xp_to_next !== null
                  ? `${progress.xp_to_next.toLocaleString()} XP until ${progress.next_level.name || `level ${progress.next_level.level_number}`}`
                  : progress.current_level
                    ? 'Highest level reached'
                    : 'No levels configured'}
              </p>
            </CardContent>
          </Card>
        )}

        {/* Stats Grid */}
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
          <StatCard
            icon={<TrendingUp className="w-6 h-6" />}
            value={progress?.total_xp ?? 0}
            label="Total XP"
            iconClassName="text-primary"
          />
          <StatCard
            icon={<Coins className="w-6 h-6" />}
            value={wallet?.balance ?? 0}
            label="Points Balance"
            iconClassName="text-violet-500"
          />
          <StatCard
            icon={<Flame className="w-6 h-6" />}
            value={currentStreak}
            label="Current Streak"
            iconClassName="text-orange-500"
          />
          <StatCard
            icon={<Trophy className="w-6 h-6" />}
            value={longestStreak}
            label="Longest Streak"
            iconClassName="text-yellow-500"
          />
          <StatCard
            icon={<Target className="w-6 h-6" />}
            value={missionsCompleted}
            label="Missions Completed"
            iconClassName="text-green-500"
          />
          <StatCard
            icon={<Award className="w-6 h-6" />}
            value={playerBadges.length}
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
              {playerBadges.length > 0 ? (
                <div className="grid grid-cols-3 sm:grid-cols-4 gap-3">
                  {playerBadges.map((pb) => (
                    <div
                      key={pb.id}
                      className="aspect-square rounded-lg bg-secondary flex flex-col items-center justify-center p-2"
                      title={pb.badge?.name}
                    >
                      <span className="text-2xl">{getActivityIcon('badge')}</span>
                      <span className="text-[10px] text-center mt-1 text-muted-foreground line-clamp-2">
                        {pb.badge?.name ?? 'Badge'}
                      </span>
                      {pb.earned_count > 1 && (
                        <span className="text-[10px] font-medium">x{pb.earned_count}</span>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-4">No badges earned yet</p>
              )}
            </CardContent>
          </Card>

          {/* XP history */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg flex items-center gap-2">
                <Sparkles className="w-5 h-5" />
                Recent XP
              </CardTitle>
            </CardHeader>
            <CardContent>
              {xpGrants && xpGrants.data.length > 0 ? (
                <div className="space-y-3">
                  {xpGrants.data.map((grant) => (
                    <div
                      key={grant.id}
                      className="flex items-start gap-3 p-2 rounded-lg hover:bg-secondary/50 transition-colors"
                    >
                      <span className="text-lg">{getActivityIcon('xp')}</span>
                      <div className="flex-1 min-w-0">
                        <p className="text-sm">
                          +{grant.amount.toLocaleString()} XP
                          {grant.description ? ` · ${grant.description}` : ''}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {formatDateTime(grant.occurred_at)}
                          {grant.source_kind ? ` · ${grant.source_kind}` : ''}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground text-center py-4">No XP granted yet</p>
              )}
            </CardContent>
          </Card>
        </div>

        {attributes.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Attributes</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="grid sm:grid-cols-2 gap-x-6 gap-y-2 text-sm">
                {attributes.map(([key, value]) => (
                  <div key={key} className="flex gap-2 min-w-0">
                    <dt className="text-muted-foreground shrink-0">{key}</dt>
                    <dd className="font-mono truncate">
                      {typeof value === 'object' ? JSON.stringify(value) : String(value)}
                    </dd>
                  </div>
                ))}
              </dl>
            </CardContent>
          </Card>
        )}

        {wallet && !wallet.opened ? (
          <Card>
            <CardContent className="pt-6 text-sm text-muted-foreground text-center">
              This player has no wallet yet. It opens on the first credit.
            </CardContent>
          </Card>
        ) : (
          <WalletTransactionsCard playerId={player.id} />
        )}
      </div>

      <PlayerFormDialog open={editOpen} onOpenChange={setEditOpen} player={player} />
      <WalletOperationDialog
        operation={walletOperation}
        player={player}
        onClose={() => setWalletOperation(null)}
      />
      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {name}?</AlertDialogTitle>
            <AlertDialogDescription>
              The player is removed from lists and can no longer earn points or XP.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

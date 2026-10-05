import React, { useEffect, useMemo, useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Trophy, Medal, Award, TrendingUp, Target, Plus, RefreshCw, Loader2, Calendar, Hash } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useToast } from '@/hooks/use-toast';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import CursorPager from '@/components/CursorPager';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import { PlayerPicker } from '@/components/mechanics/PlayerPicker';
import { changedFields, dateToRfc3339 } from '@/components/mechanics/patch';
import {
  useLeaderboardsQuery,
  useLeaderboardEntriesQuery,
  usePlayerRankQuery,
  useCreateLeaderboardMutation,
  useUpdateLeaderboardMutation,
  useDeleteLeaderboardMutation,
  useRebuildLeaderboardMutation,
  describeMechanicsError,
  MechanicsApiError,
} from '@/services/queries/mechanics';
import type {
  Leaderboard,
  LeaderboardEntry,
  LeaderboardMetric,
  LeaderboardType,
  ResetFrequency,
  CreateLeaderboardData,
  UpdateLeaderboardData,
} from '@/services/api/types';

/** Valid metrics per type (backend type x metric matrix); the first is the default. */
const metricsByType: Record<LeaderboardType, LeaderboardMetric[]> = {
  points: ['earned', 'net', 'balance'],
  xp: ['earned', 'balance'],
  badges: ['count'],
  missions: ['count'],
};

const typeLabels: Record<LeaderboardType, string> = {
  points: 'Points',
  xp: 'XP',
  badges: 'Badges',
  missions: 'Missions',
};

const leaderboardErrors: Record<string, string> = {
  leaderboard_slug_taken: 'A leaderboard with this name/slug already exists.',
  leaderboard_invalid_metric: 'That metric is not valid for this leaderboard type.',
  leaderboard_balance_requires_never: 'The balance metric requires reset frequency "never".',
  leaderboard_invalid_period: 'That period is not valid for this leaderboard.',
  leaderboard_version_conflict: 'The leaderboard was changed by someone else. Reload and try again.',
};

const scoreUnit = (lb?: Leaderboard) => {
  if (!lb) return '';
  if (lb.type === 'xp') return 'XP';
  if (lb.type === 'points') return 'pts';
  return lb.type;
};

interface LeaderboardFormState {
  name: string;
  description: string;
  type: LeaderboardType;
  metric: LeaderboardMetric;
  reset_frequency: ResetFrequency;
  max_entries: number;
  is_active: boolean;
}

const initialForm: LeaderboardFormState = {
  name: '',
  description: '',
  type: 'points',
  metric: 'earned',
  reset_frequency: 'weekly',
  max_entries: 100,
  is_active: true,
};

export default function Leaderboards() {
  const { toast } = useToast();
  const [selectedLeaderboardId, setSelectedLeaderboardId] = useState<string>('');
  const [periodDate, setPeriodDate] = useState('');
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Leaderboard | null>(null);
  const [form, setForm] = useState<LeaderboardFormState>(initialForm);
  const [rankPlayerId, setRankPlayerId] = useState('');
  const pager = useCursorPagination(50);

  const { data: leaderboardsData, isLoading: isLoadingLeaderboards, error: leaderboardsError } = useLeaderboardsQuery({ limit: 100 });
  const leaderboards = useMemo(() => leaderboardsData?.data ?? [], [leaderboardsData]);

  const createMutation = useCreateLeaderboardMutation();
  const updateMutation = useUpdateLeaderboardMutation();
  const deleteMutation = useDeleteLeaderboardMutation();
  const rebuildMutation = useRebuildLeaderboardMutation();

  // Auto-select the first leaderboard, and recover when the selected one is deleted.
  useEffect(() => {
    if (leaderboards.length > 0 && !leaderboards.some((l) => l.id === selectedLeaderboardId)) {
      setSelectedLeaderboardId(leaderboards[0].id);
    }
  }, [leaderboards, selectedLeaderboardId]);

  const period = periodDate ? dateToRfc3339(periodDate) : 'current';
  const { data: entriesData, isLoading: isLoadingEntries, isFetching: isFetchingEntries, error: entriesError } =
    useLeaderboardEntriesQuery(selectedLeaderboardId, { period, limit: pager.limit, cursor: pager.cursor });
  const entries = entriesData?.data ?? [];

  const rankQuery = usePlayerRankQuery(selectedLeaderboardId, rankPlayerId, { period, around: 2 });
  const rankNotFound = rankQuery.error instanceof MechanicsApiError && rankQuery.error.status === 404;

  const selectedLeaderboard = leaderboards.find((l) => l.id === selectedLeaderboardId);
  const unit = scoreUnit(selectedLeaderboard);
  const topThree = pager.page === 1 ? entries.slice(0, 3) : [];

  const selectLeaderboard = (id: string) => {
    setSelectedLeaderboardId(id);
    pager.reset();
  };

  const changePeriod = (date: string) => {
    setPeriodDate(date);
    pager.reset();
  };

  const openCreate = () => {
    setEditing(null);
    setForm(initialForm);
    setIsDialogOpen(true);
  };

  const openEdit = (lb: Leaderboard) => {
    setEditing(lb);
    setForm({
      name: lb.name,
      description: lb.description,
      type: lb.type,
      metric: lb.metric,
      reset_frequency: lb.reset_frequency,
      max_entries: lb.max_entries,
      is_active: lb.is_active,
    });
    setIsDialogOpen(true);
  };

  const setType = (type: LeaderboardType) => {
    setForm((prev) => ({ ...prev, type, metric: metricsByType[type][0] }));
  };

  const setMetric = (metric: LeaderboardMetric) => {
    setForm((prev) => ({ ...prev, metric, reset_frequency: metric === 'balance' ? 'never' : prev.reset_frequency }));
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    try {
      if (editing) {
        const next: UpdateLeaderboardData = {
          name: form.name,
          description: form.description,
          max_entries: form.max_entries,
          is_active: form.is_active,
        };
        const patch = changedFields(editing, next);
        if (Object.keys(patch).length > 0) {
          await updateMutation.mutateAsync({ leaderboardId: editing.id, data: patch });
        }
        toast({ title: 'Leaderboard updated', description: `${form.name} has been updated.` });
      } else {
        const data: CreateLeaderboardData = {
          name: form.name,
          description: form.description || undefined,
          type: form.type,
          metric: form.metric,
          reset_frequency: form.reset_frequency,
          max_entries: form.max_entries,
          is_active: form.is_active,
        };
        const created = await createMutation.mutateAsync(data);
        setSelectedLeaderboardId(created.id);
        pager.reset();
        toast({ title: 'Leaderboard created', description: `${created.name} is ready.` });
      }
      setIsDialogOpen(false);
    } catch (err) {
      toast({
        title: 'Error',
        description: describeMechanicsError(err, 'Failed to save leaderboard', leaderboardErrors),
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async (lb: Leaderboard) => {
    try {
      await deleteMutation.mutateAsync(lb.id);
      toast({ title: 'Leaderboard deleted', description: `${lb.name} has been deleted.` });
    } catch (err) {
      toast({ title: 'Error', description: describeMechanicsError(err, 'Failed to delete leaderboard'), variant: 'destructive' });
    }
  };

  const handleRebuild = async (lb: Leaderboard) => {
    try {
      const result = await rebuildMutation.mutateAsync(lb.id);
      toast({
        title: 'Leaderboard rebuilt',
        description: `Recomputed ${result.entries} entries across ${result.periods} period(s).`,
      });
    } catch (err) {
      toast({ title: 'Error', description: describeMechanicsError(err, 'Failed to rebuild leaderboard'), variant: 'destructive' });
    }
  };

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

  const playerName = (entry: LeaderboardEntry) => entry.display_name || entry.external_id;
  const initial = (entry: LeaderboardEntry) => (playerName(entry)[0] ?? '?').toUpperCase();
  const formatDate = (value: string | null | undefined) => (value ? new Date(value).toLocaleDateString() : 'open');

  const isSaving = createMutation.isPending || updateMutation.isPending;

  const formDialog = (
    <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
      <DialogContent className="max-w-lg">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{editing ? 'Edit Leaderboard' : 'Create Leaderboard'}</DialogTitle>
            <DialogDescription>
              {editing
                ? 'Type, metric and reset frequency cannot change after creation.'
                : 'Rank players by points, XP, badges or missions.'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="lb-name">Name *</Label>
              <Input id="lb-name" required maxLength={255} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="e.g., Top Earners" />
            </div>
            <div className="space-y-2">
              <Label htmlFor="lb-description">Description</Label>
              <Textarea id="lb-description" rows={2} maxLength={2000} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </div>
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="lb-type">Type</Label>
                <Select value={form.type} onValueChange={(v) => setType(v as LeaderboardType)} disabled={!!editing}>
                  <SelectTrigger id="lb-type"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {(Object.keys(typeLabels) as LeaderboardType[]).map((t) => (
                      <SelectItem key={t} value={t}>{typeLabels[t]}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="lb-metric">Metric</Label>
                <Select value={form.metric} onValueChange={(v) => setMetric(v as LeaderboardMetric)} disabled={!!editing}>
                  <SelectTrigger id="lb-metric"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {metricsByType[form.type].map((m) => (
                      <SelectItem key={m} value={m} className="capitalize">{m}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="lb-reset">Resets</Label>
                <Select
                  value={form.reset_frequency}
                  onValueChange={(v) => setForm({ ...form, reset_frequency: v as ResetFrequency })}
                  disabled={!!editing || form.metric === 'balance'}
                >
                  <SelectTrigger id="lb-reset"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="never">Never</SelectItem>
                    <SelectItem value="daily">Daily</SelectItem>
                    <SelectItem value="weekly">Weekly</SelectItem>
                    <SelectItem value="monthly">Monthly</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4 items-end">
              <div className="space-y-2">
                <Label htmlFor="lb-max">Max entries</Label>
                <Input
                  id="lb-max"
                  type="number"
                  min={1}
                  max={1000}
                  value={form.max_entries}
                  onChange={(e) => setForm({ ...form, max_entries: Math.min(1000, Math.max(1, Number(e.target.value) || 1)) })}
                />
              </div>
              <div className="flex items-center gap-2 pb-2">
                <Switch id="lb-active" checked={form.is_active} onCheckedChange={(c) => setForm({ ...form, is_active: c })} />
                <Label htmlFor="lb-active">Active</Label>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>Cancel</Button>
            <Button type="submit" variant="glow" disabled={isSaving}>
              {isSaving && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
              {editing ? 'Save Changes' : 'Create Leaderboard'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );

  if (isLoadingLeaderboards) {
    return (
      <div className="space-y-6 animate-fade-in">
        <div className="text-center py-12 text-muted-foreground">Loading leaderboards...</div>
      </div>
    );
  }

  if (leaderboardsError) {
    return (
      <div className="p-6 text-center">
        <p className="text-destructive">Failed to load leaderboards: {leaderboardsError.message}</p>
      </div>
    );
  }

  if (leaderboards.length === 0) {
    return (
      <div className="space-y-6 animate-fade-in">
        <div className="text-center py-12">
          <Trophy className="w-16 h-16 mx-auto text-muted-foreground mb-4" />
          <h3 className="text-lg font-semibold mb-2">No Leaderboards Yet</h3>
          <p className="text-muted-foreground mb-4">Create a leaderboard to rank your players.</p>
          <Button variant="glow" onClick={openCreate}>
            <Plus className="w-4 h-4 mr-2" />
            Create Leaderboard
          </Button>
        </div>
        {formDialog}
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
        <div className="flex flex-wrap items-center gap-2">
          <div className="w-full sm:w-64">
            <Select value={selectedLeaderboardId || undefined} onValueChange={selectLeaderboard}>
              <SelectTrigger aria-label="Select leaderboard">
                <SelectValue placeholder="Select leaderboard" />
              </SelectTrigger>
              <SelectContent>
                {leaderboards.map((leaderboard) => (
                  <SelectItem key={leaderboard.id} value={leaderboard.id}>
                    {leaderboard.name}
                    {!leaderboard.is_active && ' (inactive)'}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {selectedLeaderboard && (
            <ItemActionsMenu
              itemName={selectedLeaderboard.name}
              onEdit={() => openEdit(selectedLeaderboard)}
              onDelete={() => handleDelete(selectedLeaderboard)}
              actions={[
                {
                  label: rebuildMutation.isPending ? 'Rebuilding...' : 'Rebuild standings',
                  icon: RefreshCw,
                  onClick: () => handleRebuild(selectedLeaderboard),
                  disabled: rebuildMutation.isPending,
                },
              ]}
            />
          )}
          <Button variant="glow" onClick={openCreate}>
            <Plus className="w-4 h-4" />
            Create
          </Button>
        </div>
      </div>

      {formDialog}

      {/* Stats Cards */}
      {selectedLeaderboard && (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Card className="stat-card">
            <CardContent className="p-6">
              <div className="w-12 h-12 rounded-xl bg-violet-500/20 flex items-center justify-center mb-4">
                <Target className="w-6 h-6 text-violet-500" />
              </div>
              <h3 className="font-semibold text-lg mb-1">Type</h3>
              <p className="text-2xl font-bold">
                {typeLabels[selectedLeaderboard.type]}
                <span className="text-base font-normal text-muted-foreground capitalize"> · {selectedLeaderboard.metric}</span>
              </p>
            </CardContent>
          </Card>

          <Card className="stat-card" style={{ animationDelay: '100ms' }}>
            <CardContent className="p-6">
              <div className="w-12 h-12 rounded-xl bg-amber-500/20 flex items-center justify-center mb-4">
                <Calendar className="w-6 h-6 text-amber-500" />
              </div>
              <h3 className="font-semibold text-lg mb-1">Period</h3>
              <p className="text-lg font-bold">
                {entriesData ? `${formatDate(entriesData.period_start)} – ${formatDate(entriesData.period_end)}` : '-'}
              </p>
            </CardContent>
          </Card>

          <Card className="stat-card" style={{ animationDelay: '200ms' }}>
            <CardContent className="p-6">
              <div className="w-12 h-12 rounded-xl bg-emerald-500/20 flex items-center justify-center mb-4">
                <TrendingUp className="w-6 h-6 text-emerald-500" />
              </div>
              <h3 className="font-semibold text-lg mb-1">Reset</h3>
              <p className="text-2xl font-bold capitalize">{selectedLeaderboard.reset_frequency}</p>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Period picker */}
      {selectedLeaderboard && selectedLeaderboard.reset_frequency !== 'never' && (
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-2">
            <Label htmlFor="lb-period">Show period containing</Label>
            <Input id="lb-period" type="date" className="w-48" value={periodDate} onChange={(e) => changePeriod(e.target.value)} />
          </div>
          {periodDate && (
            <Button variant="outline" onClick={() => changePeriod('')}>Current period</Button>
          )}
        </div>
      )}

      {/* Podium */}
      {topThree.length >= 3 && (
        <div className="flex justify-center items-end gap-4 py-8">
          {/* Second Place */}
          <div className="text-center">
            <div className="w-20 h-20 rounded-full bg-gray-400/20 border-2 border-gray-400 flex items-center justify-center mx-auto mb-3">
              <span className="text-2xl font-bold">{initial(topThree[1])}</span>
            </div>
            <div className="bg-gray-400/20 rounded-t-lg px-6 py-4 border border-gray-400/30">
              <Medal className="w-8 h-8 text-gray-400 mx-auto mb-2" />
              <p className="font-semibold">{playerName(topThree[1])}</p>
              <p className="text-sm text-muted-foreground">{topThree[1].score.toLocaleString()} {unit}</p>
            </div>
            <div className="h-24 bg-gray-400/10 rounded-b-lg border-x border-b border-gray-400/30" />
          </div>

          {/* First Place */}
          <div className="text-center -mt-8">
            <div className="w-24 h-24 rounded-full bg-yellow-500/20 border-2 border-yellow-500 flex items-center justify-center mx-auto mb-3 animate-glow">
              <span className="text-3xl font-bold">{initial(topThree[0])}</span>
            </div>
            <div className="bg-yellow-500/20 rounded-t-lg px-8 py-4 border border-yellow-500/30">
              <Trophy className="w-10 h-10 text-yellow-500 mx-auto mb-2" />
              <p className="font-bold text-lg">{playerName(topThree[0])}</p>
              <p className="text-sm text-muted-foreground">{topThree[0].score.toLocaleString()} {unit}</p>
            </div>
            <div className="h-32 bg-yellow-500/10 rounded-b-lg border-x border-b border-yellow-500/30" />
          </div>

          {/* Third Place */}
          <div className="text-center">
            <div className="w-20 h-20 rounded-full bg-amber-700/20 border-2 border-amber-700 flex items-center justify-center mx-auto mb-3">
              <span className="text-2xl font-bold">{initial(topThree[2])}</span>
            </div>
            <div className="bg-amber-700/20 rounded-t-lg px-6 py-4 border border-amber-700/30">
              <Award className="w-8 h-8 text-amber-700 mx-auto mb-2" />
              <p className="font-semibold">{playerName(topThree[2])}</p>
              <p className="text-sm text-muted-foreground">{topThree[2].score.toLocaleString()} {unit}</p>
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
          ) : entriesError ? (
            <div className="p-8 text-center text-destructive">
              {describeMechanicsError(entriesError, 'Failed to load rankings', leaderboardErrors)}
            </div>
          ) : entries.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              No rankings for this period yet. Players appear here as activity is recorded.
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
                            <span className="text-sm font-medium">{initial(entry)}</span>
                          </div>
                          <div>
                            <p className="font-medium">{playerName(entry)}</p>
                            <p className="text-xs text-muted-foreground">{entry.external_id}</p>
                          </div>
                        </div>
                      </td>
                      <td className="p-4 text-right">
                        <span className="font-mono font-semibold">{entry.score.toLocaleString()}</span>
                        <span className="text-muted-foreground text-sm ml-1">{unit}</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <CursorPager
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            nextCursor={entriesData?.next_cursor}
            onPrevious={pager.previous}
            onNext={pager.next}
            isFetching={isFetchingEntries}
          />
        </CardContent>
      </Card>

      {/* Player rank lookup */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><Hash className="w-5 h-5" /> Player Standing</CardTitle>
          <CardDescription>Look up one player's rank and neighbours in this period.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="max-w-md">
            <PlayerPicker value={rankPlayerId} onChange={setRankPlayerId} id="rank-player" />
          </div>
          {rankPlayerId && (
            rankQuery.isLoading ? (
              <p className="text-sm text-muted-foreground">Loading standing...</p>
            ) : rankNotFound ? (
              <p className="text-sm text-muted-foreground">This player is not ranked in this period.</p>
            ) : rankQuery.error ? (
              <p className="text-sm text-destructive">{describeMechanicsError(rankQuery.error, 'Failed to load standing')}</p>
            ) : rankQuery.data ? (
              <div className="space-y-2">
                <p className="font-medium">
                  Rank #{rankQuery.data.entry.rank} with {rankQuery.data.entry.score.toLocaleString()} {unit}
                </p>
                <div className="divide-y divide-border rounded-md border border-border">
                  {rankQuery.data.neighbours.map((n) => (
                    <div
                      key={n.player_id}
                      className={cn('flex justify-between p-2 text-sm', n.player_id === rankPlayerId && 'bg-primary/10 font-medium')}
                    >
                      <span>#{n.rank} {playerName(n)}</span>
                      <span className="font-mono">{n.score.toLocaleString()}</span>
                    </div>
                  ))}
                </div>
                {rankQuery.data.neighbours.length === 0 && (
                  <Badge variant="secondary">No neighbours</Badge>
                )}
              </div>
            ) : null
          )}
        </CardContent>
      </Card>
    </div>
  );
}

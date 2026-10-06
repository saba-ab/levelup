import React, { useState } from 'react';
import { Plus, Flame, Clock, Gift, Zap, X, Loader2, CalendarCheck, RotateCcw, Info, Hand } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';
import { useAuth } from '@/contexts/AuthContext';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { cn } from '@/lib/utils';
import CursorPager from '@/components/CursorPager';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import { PlayerActionDialog } from '@/components/mechanics/PlayerActionDialog';
import { changedFields } from '@/components/mechanics/patch';
import {
  useStreaksQuery,
  useCreateStreakMutation,
  useUpdateStreakMutation,
  useDeleteStreakMutation,
  useRecordStreakActivityMutation,
  useResetStreakMutation,
  describeMechanicsError,
} from '@/services/queries/mechanics';
import { useEventsQuery } from '@/services/queries/events';
import type {
  Streak,
  StreakFilters,
  StreakPeriod,
  StreakMilestone,
  CreateStreakData,
  UpdateStreakData,
} from '@/services/api/types';

const periodLabels: Record<StreakPeriod, { label: string; unit: string }> = {
  daily: { label: 'Daily', unit: 'day' },
  weekly: { label: 'Weekly', unit: 'week' },
  monthly: { label: 'Monthly', unit: 'month' },
};

const streakErrors: Record<string, string> = {
  streak_slug_taken: 'A streak with this name/slug already exists.',
  streak_activity_key_taken: 'Another streak already tracks this activity.',
  streak_inactive: 'This streak is inactive. Activate it before recording.',
  streak_version_conflict: 'The streak was changed by someone else. Reload and try again.',
  player_inactive: 'This player is inactive.',
  player_streak_not_found: 'This player has no record for this streak yet.',
  invalid_streak: 'The streak settings are not valid.',
};

const ALL = 'all';

interface StreakFormData {
  name: string;
  description: string;
  period: StreakPeriod;
  activity_key: string;
  grace_periods: number;
  points_per_period: number;
  milestones: StreakMilestone[];
  auto_record: boolean;
  is_active: boolean;
}

const initialFormData: StreakFormData = {
  name: '',
  description: '',
  period: 'daily',
  activity_key: '',
  grace_periods: 0,
  points_per_period: 0,
  milestones: [],
  auto_record: true,
  is_active: true,
};

type PlayerAction = { kind: 'record' | 'reset'; streak: Streak };

export default function Streaks() {
  const [periodFilter, setPeriodFilter] = useState<string>(ALL);
  const [activeFilter, setActiveFilter] = useState<string>(ALL);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingStreak, setEditingStreak] = useState<Streak | null>(null);
  const [formData, setFormData] = useState<StreakFormData>(initialFormData);
  const [milestoneCount, setMilestoneCount] = useState('');
  const [milestoneBonus, setMilestoneBonus] = useState('');
  const [playerAction, setPlayerAction] = useState<PlayerAction | null>(null);
  const [occurredAt, setOccurredAt] = useState('');
  const { toast } = useToast();
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:mechanics');
  const pager = useCursorPagination(20);

  const filters: StreakFilters = {
    limit: pager.limit,
    cursor: pager.cursor,
    period: periodFilter === ALL ? undefined : (periodFilter as StreakPeriod),
    active: activeFilter === ALL ? undefined : activeFilter === 'active',
  };
  const { data: streaksData, isLoading, isFetching, error } = useStreaksQuery(filters);
  const { data: events = [] } = useEventsQuery();
  const createMutation = useCreateStreakMutation();
  const updateMutation = useUpdateStreakMutation();
  const deleteMutation = useDeleteStreakMutation();
  const recordMutation = useRecordStreakActivityMutation();
  const resetMutation = useResetStreakMutation();

  const streaks = streaksData?.data ?? [];
  const eventKeys = events.map((e) => e.slug);

  const setFilter = (setter: (v: string) => void) => (value: string) => {
    setter(value);
    pager.reset();
  };

  const openCreate = () => {
    setEditingStreak(null);
    setFormData(initialFormData);
    setMilestoneCount('');
    setMilestoneBonus('');
    setIsDialogOpen(true);
  };

  const openEdit = (streak: Streak) => {
    setEditingStreak(streak);
    setFormData({
      name: streak.name,
      description: streak.description,
      period: streak.period,
      activity_key: streak.activity_key,
      grace_periods: streak.grace_periods,
      points_per_period: streak.points_per_period,
      milestones: streak.milestones ?? [],
      auto_record: streak.auto_record ?? true,
      is_active: streak.is_active,
    });
    setMilestoneCount('');
    setMilestoneBonus('');
    setIsDialogOpen(true);
  };

  const addMilestone = () => {
    const count = Number(milestoneCount);
    const bonus = Number(milestoneBonus) || 0;
    if (!Number.isInteger(count) || count < 1 || formData.milestones.some((m) => m.count === count)) return;
    setFormData({
      ...formData,
      milestones: [...formData.milestones, { count, bonus_points: Math.max(0, bonus) }].sort((a, b) => a.count - b.count),
    });
    setMilestoneCount('');
    setMilestoneBonus('');
  };

  const removeMilestone = (count: number) => {
    setFormData({ ...formData, milestones: formData.milestones.filter((m) => m.count !== count) });
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!formData.activity_key.trim()) {
      toast({ title: 'Validation Error', description: 'Activity key is required', variant: 'destructive' });
      return;
    }

    try {
      if (editingStreak) {
        const next: UpdateStreakData = {
          name: formData.name,
          description: formData.description,
          activity_key: formData.activity_key.trim(),
          grace_periods: formData.grace_periods,
          points_per_period: formData.points_per_period,
          milestones: formData.milestones,
          auto_record: formData.auto_record,
          is_active: formData.is_active,
        };
        const patch = changedFields(editingStreak, next);
        if (Object.keys(patch).length > 0) {
          await updateMutation.mutateAsync({ streakId: editingStreak.id, data: patch });
        }
        toast({ title: 'Streak updated', description: `${formData.name} has been updated successfully.` });
      } else {
        const data: CreateStreakData = {
          name: formData.name,
          description: formData.description || undefined,
          period: formData.period,
          activity_key: formData.activity_key.trim(),
          grace_periods: formData.grace_periods,
          points_per_period: formData.points_per_period,
          milestones: formData.milestones,
          auto_record: formData.auto_record,
          is_active: formData.is_active,
        };
        await createMutation.mutateAsync(data);
        toast({ title: 'Streak created', description: 'New streak has been created successfully.' });
      }
      setIsDialogOpen(false);
    } catch (err) {
      toast({
        title: 'Error',
        description: describeMechanicsError(err, 'Failed to save streak', streakErrors),
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async (streak: Streak) => {
    try {
      await deleteMutation.mutateAsync(streak.id);
      toast({ title: 'Streak deleted', description: `${streak.name} has been deleted.` });
    } catch (err) {
      toast({ title: 'Error', description: describeMechanicsError(err, 'Failed to delete streak'), variant: 'destructive' });
    }
  };

  const openPlayerAction = (kind: PlayerAction['kind'], streak: Streak) => {
    setOccurredAt('');
    setPlayerAction({ kind, streak });
  };

  const handlePlayerAction = async (playerId: string) => {
    if (!playerAction) return;
    const { kind, streak } = playerAction;
    try {
      if (kind === 'record') {
        const result = await recordMutation.mutateAsync({
          streak_id: streak.id,
          player_id: playerId,
          occurred_at: occurredAt ? new Date(occurredAt).toISOString() : undefined,
        });
        const count = result.player_streak.current_count;
        const milestones = result.milestones_reached ?? [];
        toast({
          title:
            result.outcome === 'recorded'
              ? 'Activity recorded'
              : result.outcome === 'noop'
              ? 'Already recorded this period'
              : 'Already applied',
          description: `Current streak: ${count} ${periodLabels[streak.period].unit}${count === 1 ? '' : 's'}.${
            milestones.length > 0 ? ` Milestone reached: ${milestones.join(', ')}!` : ''
          }`,
        });
      } else {
        await resetMutation.mutateAsync({ playerId, streakId: streak.id });
        toast({ title: 'Streak reset', description: `${streak.name} was reset for the player.` });
      }
    } catch (err) {
      toast({
        title: 'Action failed',
        description: describeMechanicsError(err, 'Streak action failed', streakErrors),
        variant: 'destructive',
      });
      throw err;
    }
  };


  const isSaving = createMutation.isPending || updateMutation.isPending;
  const hasFilters = periodFilter !== ALL || activeFilter !== ALL;
  const unit = periodLabels[formData.period].unit;

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Streaks</h1>
          <p className="text-muted-foreground mt-1">Configure streak mechanics and milestone rewards.</p>
        </div>
        {canManage && <div className="flex gap-2">
          <Button variant="glow" onClick={openCreate}>
            <Plus className="w-4 h-4" />
            Create Streak
          </Button>
        </div>}
      </div>

      <Card className="border-primary/20 bg-primary/5">
        <CardContent className="p-4 flex gap-3 text-sm">
          <Info className="w-5 h-5 text-primary shrink-0 mt-0.5" />
          <p className="text-muted-foreground">
            Streaks are recorded automatically from activities: when an activity arrives whose event type equals a
            streak's activity key, that player's current period is recorded (once per period), with points and
            milestones applied. Turn off <span className="font-medium text-foreground">Record automatically</span> on a
            streak to record it only through the API or rules.
          </p>
        </CardContent>
      </Card>

      {/* Create / Edit Dialog */}
      <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>{editingStreak ? 'Edit Streak' : 'Create New Streak'}</DialogTitle>
              <DialogDescription>
                {editingStreak ? 'The period cannot change after creation.' : 'Reward players for consecutive activity.'}
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="streak-name">Streak Name *</Label>
                <Input
                  id="streak-name"
                  required
                  maxLength={255}
                  placeholder="e.g., Daily Login"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="streak-description">Description</Label>
                <Textarea
                  id="streak-description"
                  rows={2}
                  maxLength={1000}
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="streak-period">Period</Label>
                  <Select
                    value={formData.period}
                    onValueChange={(value) => setFormData({ ...formData, period: value as StreakPeriod })}
                    disabled={!!editingStreak}
                  >
                    <SelectTrigger id="streak-period"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {(Object.keys(periodLabels) as StreakPeriod[]).map((p) => (
                        <SelectItem key={p} value={p}>{periodLabels[p].label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="streak-activity">Activity Key *</Label>
                  <Input
                    id="streak-activity"
                    required
                    maxLength={100}
                    list="streak-activity-keys"
                    placeholder="e.g., user_login"
                    value={formData.activity_key}
                    onChange={(e) => setFormData({ ...formData, activity_key: e.target.value })}
                  />
                  <p className="text-xs text-muted-foreground">The event type that counts as activity for this streak.</p>
                  <datalist id="streak-activity-keys">
                    {eventKeys.map((key) => (
                      <option key={key} value={key} />
                    ))}
                  </datalist>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="streak-points">Points per {unit}</Label>
                  <Input
                    id="streak-points"
                    type="number"
                    min="0"
                    value={formData.points_per_period}
                    onChange={(e) => setFormData({ ...formData, points_per_period: Math.max(0, Number(e.target.value) || 0) })}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="streak-grace">Grace periods</Label>
                  <Input
                    id="streak-grace"
                    type="number"
                    min="0"
                    max="30"
                    value={formData.grace_periods}
                    onChange={(e) => setFormData({ ...formData, grace_periods: Math.min(30, Math.max(0, Number(e.target.value) || 0)) })}
                  />
                  <p className="text-xs text-muted-foreground">Missed {unit}s allowed before the streak breaks.</p>
                </div>
              </div>

              <div className="space-y-2">
                <Label>Milestones</Label>
                <div className="flex gap-2">
                  <Input
                    type="number"
                    min="1"
                    placeholder={`Count (${unit}s)`}
                    aria-label="Milestone count"
                    value={milestoneCount}
                    onChange={(e) => setMilestoneCount(e.target.value)}
                  />
                  <Input
                    type="number"
                    min="0"
                    placeholder="Bonus points"
                    aria-label="Milestone bonus points"
                    value={milestoneBonus}
                    onChange={(e) => setMilestoneBonus(e.target.value)}
                  />
                  <Button type="button" variant="outline" onClick={addMilestone} disabled={formData.milestones.length >= 50}>
                    Add
                  </Button>
                </div>
                {formData.milestones.length > 0 && (
                  <div className="flex flex-wrap gap-2 pt-1">
                    {formData.milestones.map((m) => (
                      <Badge key={m.count} variant="secondary" className="gap-1">
                        {m.count} {unit}s · +{m.bonus_points} pts
                        <button
                          type="button"
                          aria-label={`Remove ${m.count} ${unit} milestone`}
                          onClick={() => removeMilestone(m.count)}
                          className="ml-1 hover:text-destructive"
                        >
                          <X className="w-3 h-3" />
                        </button>
                      </Badge>
                    ))}
                  </div>
                )}
              </div>

              <div className="flex items-start gap-3 rounded-lg border border-border p-3">
                <Switch
                  id="streak-auto-record"
                  checked={formData.auto_record}
                  onCheckedChange={(checked) => setFormData({ ...formData, auto_record: checked })}
                />
                <div className="space-y-1">
                  <Label htmlFor="streak-auto-record">Record automatically</Label>
                  <p className="text-xs text-muted-foreground">
                    {formData.auto_record
                      ? `Every activity with event type "${formData.activity_key.trim() || 'activity key'}" records the player's current ${unit}.`
                      : 'Activities are ignored: record this streak through the API or rules ("Record activity for player").'}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-2">
                <Switch
                  id="streak-active"
                  checked={formData.is_active}
                  onCheckedChange={(checked) => setFormData({ ...formData, is_active: checked })}
                />
                <Label htmlFor="streak-active">Active</Label>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>Cancel</Button>
              <Button type="submit" variant="glow" disabled={isSaving}>
                {isSaving && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                {editingStreak ? 'Save Changes' : 'Create Streak'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Filters (server-side: ?period&active) */}
      <div className="flex flex-wrap gap-3">
        <Select value={periodFilter} onValueChange={setFilter(setPeriodFilter)}>
          <SelectTrigger className="w-40" aria-label="Filter by period"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>All periods</SelectItem>
            {(Object.keys(periodLabels) as StreakPeriod[]).map((p) => (
              <SelectItem key={p} value={p}>{periodLabels[p].label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={activeFilter} onValueChange={setFilter(setActiveFilter)}>
          <SelectTrigger className="w-36" aria-label="Filter by status"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Any status</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="inactive">Inactive</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Streaks Grid */}
      {isLoading ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {[...Array(4)].map((_, i) => (
            <Card key={i}>
              <CardHeader>
                <Skeleton className="h-6 w-3/4 mb-2" />
                <Skeleton className="h-4 w-full" />
              </CardHeader>
              <CardContent>
                <Skeleton className="h-20 w-full" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : error ? (
        <Card className="p-8 text-center border-destructive">
          <p className="text-destructive">Failed to load streaks: {error.message}</p>
        </Card>
      ) : streaks.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <Flame className="w-16 h-16 mx-auto mb-4 text-muted-foreground" />
            <h3 className="text-lg font-medium mb-2">No streaks found</h3>
            <p className="text-muted-foreground mb-4">
              {hasFilters ? 'Try different filters.' : 'Create your first streak to get started.'}
            </p>
            {!hasFilters && canManage && (
              <Button onClick={openCreate}>
                <Plus className="w-4 h-4 mr-2" />
                Create Streak
              </Button>
            )}
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {streaks.map((streak, index) => {
            const periodUnit = periodLabels[streak.period].unit;
            return (
              <Card key={streak.id} className="stat-card group" style={{ animationDelay: `${index * 100}ms` }}>
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <div className={cn(
                        'w-12 h-12 rounded-xl flex items-center justify-center',
                        streak.is_active ? 'bg-orange-500/10' : 'bg-muted'
                      )}>
                        <Flame className={cn('w-6 h-6', streak.is_active ? 'text-orange-500' : 'text-muted-foreground')} />
                      </div>
                      <div>
                        <CardTitle className="text-lg">{streak.name}</CardTitle>
                        <CardDescription>{streak.description || 'No description'}</CardDescription>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Badge variant="outline" className={cn(
                        streak.is_active
                          ? 'border-green-500/50 text-green-500 bg-green-500/10'
                          : 'border-muted-foreground text-muted-foreground'
                      )}>
                        {streak.is_active ? 'Active' : 'Inactive'}
                      </Badge>
                      {canManage && <ItemActionsMenu
                        itemName={streak.name}
                        onEdit={() => openEdit(streak)}
                        onDelete={() => handleDelete(streak)}
                        actions={[
                          { label: 'Record activity for player', icon: CalendarCheck, onClick: () => openPlayerAction('record', streak), disabled: !streak.is_active },
                          { label: 'Reset for player', icon: RotateCcw, onClick: () => openPlayerAction('reset', streak) },
                        ]}
                        showInGroup
                      />}
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="flex flex-wrap items-center gap-3 text-sm">
                    <Badge variant="secondary">{periodLabels[streak.period].label}</Badge>
                    {streak.auto_record ? (
                      <Badge variant="outline" className="gap-1 border-primary/50 text-primary">
                        <Zap className="w-3 h-3" /> Auto-recorded
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="gap-1 text-muted-foreground">
                        <Hand className="w-3 h-3" /> Manual
                      </Badge>
                    )}
                    <span className="text-muted-foreground">Activity: <code>{streak.activity_key}</code></span>
                  </div>
                  <div className="grid grid-cols-2 gap-4 text-sm">
                    <div className="flex items-center gap-2">
                      <Zap className="w-4 h-4 text-amber-500" />
                      <span>{streak.points_per_period} pts / {periodUnit}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <Clock className="w-4 h-4 text-muted-foreground" />
                      <span>{streak.grace_periods} grace {periodUnit}{streak.grace_periods === 1 ? '' : 's'}</span>
                    </div>
                  </div>
                  {streak.milestones && streak.milestones.length > 0 && (
                    <div className="space-y-2">
                      <p className="text-sm font-medium flex items-center gap-2">
                        <Gift className="w-4 h-4" /> Milestones
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {streak.milestones.map((m) => (
                          <Badge key={m.count} variant="outline">
                            {m.count} {periodUnit}s · +{m.bonus_points} pts
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <CursorPager
        page={pager.page}
        hasPrevious={pager.hasPrevious}
        nextCursor={streaksData?.next_cursor}
        onPrevious={pager.previous}
        onNext={pager.next}
        isFetching={isFetching}
      />

      <PlayerActionDialog
        open={!!playerAction}
        onOpenChange={(open) => !open && setPlayerAction(null)}
        title={
          playerAction?.kind === 'record'
            ? `Record "${playerAction.streak.name}"`
            : `Reset "${playerAction?.streak.name ?? ''}"`
        }
        description={
          playerAction?.kind === 'record'
            ? 'Records the activity for its period. Recording twice in one period is a no-op.'
            : 'Sets the player\'s current streak back to zero (the longest streak is kept).'
        }
        submitLabel={playerAction?.kind === 'record' ? 'Record' : 'Reset Streak'}
        isPending={recordMutation.isPending || resetMutation.isPending}
        onSubmit={handlePlayerAction}
        destructive={playerAction?.kind === 'reset'}
      >
        {playerAction?.kind === 'record' && (
          <div className="space-y-2">
            <Label htmlFor="streak-occurred">Occurred at (optional)</Label>
            <Input
              id="streak-occurred"
              type="datetime-local"
              value={occurredAt}
              onChange={(e) => setOccurredAt(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">Defaults to now; decides which period is recorded.</p>
          </div>
        )}
      </PlayerActionDialog>
    </div>
  );
}

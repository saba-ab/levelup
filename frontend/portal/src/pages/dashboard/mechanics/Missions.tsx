import React, { useState } from 'react';
import { Plus, Target, Calendar, Sparkles, Loader2, Play, TrendingUp, CheckCircle2, ListChecks, Zap, BarChart3 } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { Textarea } from '@/components/ui/textarea';
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
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useToast } from '@/hooks/use-toast';
import { useAuth } from '@/contexts/AuthContext';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { cn } from '@/lib/utils';
import CursorPager from '@/components/CursorPager';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import { PlayerActionDialog } from '@/components/mechanics/PlayerActionDialog';
import { changedFields, dateToRfc3339, optionalNumber, rfc3339ToDate } from '@/components/mechanics/patch';
import { MissionCriteriaEditor } from '@/components/mechanics/MissionCriteriaEditor';
import {
  criteriaToDraft,
  draftToCriteria,
  emptyCriteriaDraft,
  summarizeCriteria,
  validateCriteriaDraft,
  type CriteriaDraft,
} from '@/components/mechanics/criteria';
import {
  useMissionsQuery,
  useMissionStatsQuery,
  useMissionStatsListQuery,
  MechanicsApiError,
  useMissionAttemptsQuery,
  useAllBadgesQuery,
  useCreateMissionMutation,
  useUpdateMissionMutation,
  useDeleteMissionMutation,
  useStartMissionMutation,
  useUpdateMissionProgressMutation,
  useCompleteMissionMutation,
  useInvalidateCreatedDraft,
  describeMechanicsError,
} from '@/services/queries/mechanics';
import type {
  Mission,
  MissionFilters,
  MissionStatus,
  MissionType,
  MissionAttemptStatus,
  CreateMissionData,
  UpdateMissionData,
  MissionStats,
} from '@/services/api/types';

const typeLabels: Record<MissionType, string> = {
  one_time: 'One-time',
  daily: 'Daily',
  weekly: 'Weekly',
  repeating: 'Repeating',
};

const statusLabels: Record<MissionStatus, string> = {
  draft: 'Draft',
  active: 'Active',
  paused: 'Paused',
  expired: 'Expired',
  archived: 'Archived',
};

/** Mission state machine (backend missions/domain): allowed next statuses. */
const transitions: Record<MissionStatus, MissionStatus[]> = {
  draft: ['active', 'archived'],
  active: ['paused', 'expired', 'archived'],
  paused: ['active', 'expired', 'archived'],
  expired: ['archived'],
  archived: [],
};

const attemptStatusClass: Record<MissionAttemptStatus, string> = {
  in_progress: 'border-blue-500/50 text-blue-500 bg-blue-500/10',
  completed: 'border-green-500/50 text-green-500 bg-green-500/10',
  expired: 'border-muted-foreground text-muted-foreground',
  abandoned: 'border-muted-foreground text-muted-foreground',
};

const missionErrors: Record<string, string> = {
  slug_taken: 'A mission with this name/slug already exists.',
  invalid_status_transition: 'That status change is not allowed.',
  mission_type_immutable: 'The type can only change while the mission is a draft.',
  invalid_mission_window: 'The end date must be after the start date.',
  invalid_max_completions: 'A one-time mission completes at most once per player.',
  version_conflict: 'The mission was changed by someone else. Reload and try again.',
  mission_not_available: 'The mission is not active or is outside its schedule.',
  mission_limit_reached: 'The player reached this mission\'s completion limit.',
  mission_already_started: 'The player already started this mission for the current period.',
  mission_not_started: 'The player has not started this mission.',
  mission_not_completed: 'The mission target has not been reached yet.',
  attempt_not_in_progress: 'The player\'s attempt is not in progress.',
  player_inactive: 'This player is inactive.',
  invalid_mission_criteria: 'The criteria are not valid: see the highlighted fields.',
};

const ALL = 'all';
const NO_BADGE = 'none';

interface MissionFormData {
  name: string;
  description: string;
  type: MissionType;
  status: MissionStatus;
  target: number;
  points_reward: number;
  xp_reward: number;
  badge_reward_id: string;
  starts_at: string;
  ends_at: string;
  max_completions_per_player: string;
}

const initialFormData: MissionFormData = {
  name: '',
  description: '',
  type: 'one_time',
  status: 'draft',
  target: 1,
  points_reward: 0,
  xp_reward: 0,
  badge_reward_id: '',
  starts_at: '',
  ends_at: '',
  max_completions_per_player: '',
};

type PlayerAction = { kind: 'start' | 'progress' | 'complete'; mission: Mission };

const formatRate = (rate: number) => `${(rate * 100).toFixed(rate > 0 && rate < 0.1 ? 1 : 0)}%`;
const formatHours = (hours: number | null) =>
  hours === null ? '-' : hours < 1 ? `${Math.round(hours * 60)} min` : hours < 48 ? `${hours.toFixed(1)} h` : `${(hours / 24).toFixed(1)} days`;

/** Completion numbers of one mission (GET /missions/{id}/stats or a row of /missions/stats). */
function MissionStatsSummary({ stats }: { stats: MissionStats }) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
      {[
        { label: 'Started', value: stats.started.toLocaleString() },
        { label: 'In progress', value: stats.in_progress.toLocaleString() },
        { label: 'Completed', value: stats.completed.toLocaleString() },
        { label: 'Completion rate', value: formatRate(stats.completion_rate) },
        { label: 'Avg. time to complete', value: formatHours(stats.avg_hours_to_complete) },
      ].map((item) => (
        <div key={item.label} className="rounded-md border border-border p-3 text-center">
          <p className="text-lg font-bold">{item.value}</p>
          <p className="text-xs text-muted-foreground">{item.label}</p>
        </div>
      ))}
    </div>
  );
}

/** Tenant-wide mission analytics (GET /missions/stats, cursor paginated). */
function MissionAnalyticsPanel() {
  const [statusFilter, setStatusFilter] = useState<string>(ALL);
  const [typeFilter, setTypeFilter] = useState<string>(ALL);
  const pager = useCursorPagination(25);
  const { data, isLoading, isFetching, error } = useMissionStatsListQuery({
    limit: pager.limit,
    cursor: pager.cursor,
    status: statusFilter === ALL ? undefined : (statusFilter as MissionStatus),
    type: typeFilter === ALL ? undefined : (typeFilter as MissionType),
  });
  const rows = data?.data ?? [];
  const setFilter = (setter: (v: string) => void) => (value: string) => {
    setter(value);
    pager.reset();
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div>
            <CardTitle className="text-lg flex items-center gap-2"><BarChart3 className="w-5 h-5" /> Completion analytics</CardTitle>
            <CardDescription>Attempts started and completed per mission.</CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            <Select value={statusFilter} onValueChange={setFilter(setStatusFilter)}>
              <SelectTrigger className="w-36" aria-label="Filter analytics by status"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All statuses</SelectItem>
                {(Object.keys(statusLabels) as MissionStatus[]).map((s) => (
                  <SelectItem key={s} value={s}>{statusLabels[s]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={typeFilter} onValueChange={setFilter(setTypeFilter)}>
              <SelectTrigger className="w-36" aria-label="Filter analytics by type"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All types</SelectItem>
                {(Object.keys(typeLabels) as MissionType[]).map((t) => (
                  <SelectItem key={t} value={t}>{typeLabels[t]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {isLoading ? (
          <div className="p-6 space-y-2">{[...Array(4)].map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}</div>
        ) : error ? (
          <p className="p-6 text-sm text-destructive">Failed to load mission analytics: {error.message}</p>
        ) : rows.length === 0 ? (
          <p className="p-6 text-sm text-muted-foreground text-center">No missions match.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-3 text-sm font-medium text-muted-foreground">Mission</th>
                  <th className="text-left p-3 text-sm font-medium text-muted-foreground">Status</th>
                  <th className="text-right p-3 text-sm font-medium text-muted-foreground">Started</th>
                  <th className="text-right p-3 text-sm font-medium text-muted-foreground">In progress</th>
                  <th className="text-right p-3 text-sm font-medium text-muted-foreground">Completed</th>
                  <th className="text-left p-3 text-sm font-medium text-muted-foreground w-48">Completion rate</th>
                  <th className="text-right p-3 text-sm font-medium text-muted-foreground">Avg. time</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.mission_id} className="border-b border-border/50">
                    <td className="p-3">
                      <p className="font-medium">{row.name}</p>
                      <p className="text-xs text-muted-foreground font-mono">{row.slug}</p>
                    </td>
                    <td className="p-3"><Badge variant="outline" className="capitalize">{row.status}</Badge></td>
                    <td className="p-3 text-right font-mono">{row.started.toLocaleString()}</td>
                    <td className="p-3 text-right font-mono">{row.in_progress.toLocaleString()}</td>
                    <td className="p-3 text-right font-mono">{row.completed.toLocaleString()}</td>
                    <td className="p-3">
                      <div className="flex items-center gap-2">
                        <Progress value={row.completion_rate * 100} className="h-2" />
                        <span className="text-sm font-mono w-12 text-right">{formatRate(row.completion_rate)}</span>
                      </div>
                    </td>
                    <td className="p-3 text-right text-sm text-muted-foreground">{formatHours(row.avg_hours_to_complete)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <CursorPager
          page={pager.page}
          hasPrevious={pager.hasPrevious}
          nextCursor={data?.next_cursor}
          onPrevious={pager.previous}
          onNext={pager.next}
          isFetching={isFetching}
        />
      </CardContent>
    </Card>
  );
}

export default function Missions() {
  const [statusFilter, setStatusFilter] = useState<string>(ALL);
  const [typeFilter, setTypeFilter] = useState<string>(ALL);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingMission, setEditingMission] = useState<Mission | null>(null);
  const [formData, setFormData] = useState<MissionFormData>(initialFormData);
  const [playerAction, setPlayerAction] = useState<PlayerAction | null>(null);
  const [increment, setIncrement] = useState(1);
  const [attemptsMission, setAttemptsMission] = useState<Mission | null>(null);
  const [criteria, setCriteria] = useState<CriteriaDraft>(emptyCriteriaDraft);
  const [criteriaUnsupported, setCriteriaUnsupported] = useState(false);
  const [criteriaErrors, setCriteriaErrors] = useState<Record<string, string>>({});
  const { toast } = useToast();
  const invalidateCreatedDraft = useInvalidateCreatedDraft();
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:mechanics');
  const pager = useCursorPagination(20);
  const attemptsPager = useCursorPagination(20);

  const filters: MissionFilters = {
    limit: pager.limit,
    cursor: pager.cursor,
    status: statusFilter === ALL ? undefined : (statusFilter as MissionStatus),
    type: typeFilter === ALL ? undefined : (typeFilter as MissionType),
  };
  const { data: missionsData, isLoading, isFetching, error } = useMissionsQuery(filters);
  const { data: badges = [] } = useAllBadgesQuery();
  const attemptsQuery = useMissionAttemptsQuery(attemptsMission?.id ?? '', {
    limit: attemptsPager.limit,
    cursor: attemptsPager.cursor,
  });
  const missionStatsQuery = useMissionStatsQuery(attemptsMission?.id ?? '');
  const createMutation = useCreateMissionMutation();
  const updateMutation = useUpdateMissionMutation();
  const deleteMutation = useDeleteMissionMutation();
  const startMutation = useStartMissionMutation();
  const progressMutation = useUpdateMissionProgressMutation();
  const completeMutation = useCompleteMissionMutation();

  const missions = missionsData?.data ?? [];
  const badgeName = (id: string | null) => (id ? badges.find((b) => b.id === id)?.name ?? 'Badge' : null);

  const setFilter = (setter: (v: string) => void) => (value: string) => {
    setter(value);
    pager.reset();
  };

  const resetCriteria = (raw?: Record<string, unknown> | null) => {
    const { draft, unsupported } = criteriaToDraft(raw);
    setCriteria(draft);
    setCriteriaUnsupported(unsupported);
    setCriteriaErrors({});
  };

  const openCreate = () => {
    setEditingMission(null);
    setFormData(initialFormData);
    resetCriteria(null);
    setIsDialogOpen(true);
  };

  const openStats = (mission: Mission) => {
    attemptsPager.reset();
    setAttemptsMission(mission);
  };

  const openEdit = (mission: Mission) => {
    setEditingMission(mission);
    setFormData({
      name: mission.name,
      description: mission.description,
      type: mission.type,
      status: mission.status,
      target: mission.target,
      points_reward: mission.points_reward,
      xp_reward: mission.xp_reward,
      badge_reward_id: mission.badge_reward_id ?? '',
      starts_at: rfc3339ToDate(mission.starts_at),
      ends_at: rfc3339ToDate(mission.ends_at),
      max_completions_per_player: mission.max_completions_per_player?.toString() ?? '',
    });
    resetCriteria(mission.criteria);
    setIsDialogOpen(true);
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const clientErrors = validateCriteriaDraft(criteria);
    if (Object.keys(clientErrors).length > 0) {
      setCriteriaErrors(clientErrors);
      toast({ title: 'Validation Error', description: 'Fix the highlighted criteria fields.', variant: 'destructive' });
      return;
    }
    setCriteriaErrors({});
    const criteriaPayload = draftToCriteria(criteria) as Record<string, unknown>;

    try {
      if (editingMission) {
        const next: UpdateMissionData = {
          name: formData.name,
          description: formData.description,
          type: formData.type,
          status: formData.status,
          target: formData.target,
          criteria: criteriaPayload,
          points_reward: formData.points_reward,
          xp_reward: formData.xp_reward,
          badge_reward_id: formData.badge_reward_id || undefined,
          max_completions_per_player: optionalNumber(formData.max_completions_per_player),
          starts_at: dateToRfc3339(formData.starts_at),
          ends_at: dateToRfc3339(formData.ends_at),
        };
        const patch = changedFields(editingMission, next);
        // Dates round-trip through <input type="date">: only send them when the day changed.
        if (formData.starts_at === rfc3339ToDate(editingMission.starts_at)) delete patch.starts_at;
        if (formData.ends_at === rfc3339ToDate(editingMission.ends_at)) delete patch.ends_at;
        if (Object.keys(patch).length > 0) {
          await updateMutation.mutateAsync({ missionId: editingMission.id, data: patch });
        }
        toast({ title: 'Mission updated', description: `${formData.name} has been updated successfully.` });
      } else {
        const data: CreateMissionData = {
          name: formData.name,
          description: formData.description || undefined,
          type: formData.type,
          status: formData.status === 'active' ? 'active' : 'draft',
          target: formData.target,
          criteria: criteriaPayload,
          points_reward: formData.points_reward,
          xp_reward: formData.xp_reward,
          badge_reward_id: formData.badge_reward_id || undefined,
          max_completions_per_player: optionalNumber(formData.max_completions_per_player),
          starts_at: dateToRfc3339(formData.starts_at),
          ends_at: dateToRfc3339(formData.ends_at),
        };
        await createMutation.mutateAsync(data);
        toast({ title: 'Mission created', description: 'New mission has been created successfully.' });
      }
      setIsDialogOpen(false);
    } catch (err) {
      if (err instanceof MechanicsApiError && err.code === 'invalid_mission_criteria') {
        const fields = Object.fromEntries(
          Object.entries(err.validationErrors ?? {}).map(([field, msgs]) => [field, msgs.join(', ')]),
        );
        setCriteriaErrors(Object.keys(fields).length > 0 ? fields : { criteria: err.message });
      }
      toast({
        title: 'Error',
        description: describeMechanicsError(err, 'Failed to save mission', missionErrors),
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async (mission: Mission) => {
    try {
      await deleteMutation.mutateAsync(mission.id);
      toast({ title: 'Mission deleted', description: `${mission.name} has been deleted.` });
    } catch (err) {
      toast({ title: 'Error', description: describeMechanicsError(err, 'Failed to delete mission'), variant: 'destructive' });
    }
  };

  const openPlayerAction = (kind: PlayerAction['kind'], mission: Mission) => {
    setIncrement(1);
    setPlayerAction({ kind, mission });
  };

  const handlePlayerAction = async (playerId: string) => {
    if (!playerAction) return;
    const { kind, mission } = playerAction;
    try {
      if (kind === 'start') {
        await startMutation.mutateAsync({ mission_id: mission.id, player_id: playerId });
        toast({ title: 'Mission started', description: `${mission.name} started for the player.` });
      } else if (kind === 'progress') {
        const result = await progressMutation.mutateAsync({ mission_id: mission.id, player_id: playerId, increment });
        const attempt = result.attempt;
        toast({
          title: result.duplicate ? 'Already applied' : result.completed ? 'Mission completed' : 'Progress added',
          description: attempt ? `Progress ${attempt.progress} / ${attempt.target}.` : undefined,
        });
      } else {
        await completeMutation.mutateAsync({ mission_id: mission.id, player_id: playerId });
        toast({ title: 'Mission completed', description: `${mission.name} completed; rewards are being granted.` });
      }
    } catch (err) {
      toast({
        title: 'Action failed',
        description: describeMechanicsError(err, 'Mission action failed', missionErrors),
        variant: 'destructive',
      });
      throw err;
    }
  };


  const isSaving = createMutation.isPending || updateMutation.isPending;
  const statusOptions: MissionStatus[] = editingMission
    ? [editingMission.status, ...transitions[editingMission.status]]
    : ['draft', 'active'];
  const playerActionPending = startMutation.isPending || progressMutation.isPending || completeMutation.isPending;
  const hasFilters = statusFilter !== ALL || typeFilter !== ALL;

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Missions</h1>
          <p className="text-muted-foreground mt-1">Create and manage player missions.</p>
        </div>
        {canManage && <div className="flex gap-2">
          <AIGenerateDialog
            trigger={
              <Button variant="outline" className="gap-2">
                <Sparkles className="w-4 h-4" />
                Generate with AI
              </Button>
            }
            title="Generate Mission Ideas"
            placeholder="E.g., Create a weekly mission that encourages social engagement..."
            context="Generate mission names, targets, and reward structures"
            kind="mission"
            onCreated={invalidateCreatedDraft}
          />
          <Button variant="glow" onClick={openCreate}>
            <Plus className="w-4 h-4" />
            Create Mission
          </Button>
        </div>}
      </div>

      {/* Create / Edit Mission Dialog */}
      <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
        <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto">
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>{editingMission ? 'Edit Mission' : 'Create New Mission'}</DialogTitle>
              <DialogDescription>
                {editingMission ? 'Update mission details.' : 'A mission counts progress towards a target.'}
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-6 py-4">
              {/* Basic Information */}
              <div className="space-y-4">
                <h3 className="font-semibold">Basic Information</h3>
                <div className="space-y-2">
                  <Label htmlFor="mission-name">Mission Name *</Label>
                  <Input
                    id="mission-name"
                    required
                    maxLength={255}
                    placeholder="e.g., Spring Challenge"
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="mission-description">Description</Label>
                  <Textarea
                    id="mission-description"
                    rows={3}
                    maxLength={1000}
                    placeholder="Describe what players need to do..."
                    value={formData.description}
                    onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="mission-type">Mission Type</Label>
                    <Select
                      value={formData.type}
                      onValueChange={(value) => setFormData({ ...formData, type: value as MissionType })}
                      disabled={!!editingMission && editingMission.status !== 'draft'}
                    >
                      <SelectTrigger id="mission-type"><SelectValue /></SelectTrigger>
                      <SelectContent>
                        {(Object.keys(typeLabels) as MissionType[]).map((t) => (
                          <SelectItem key={t} value={t}>{typeLabels[t]}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {editingMission && editingMission.status !== 'draft' && (
                      <p className="text-xs text-muted-foreground">Type can only change while the mission is a draft.</p>
                    )}
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="mission-status">Status</Label>
                    <Select
                      value={formData.status}
                      onValueChange={(value) => setFormData({ ...formData, status: value as MissionStatus })}
                      disabled={statusOptions.length <= 1}
                    >
                      <SelectTrigger id="mission-status"><SelectValue /></SelectTrigger>
                      <SelectContent>
                        {statusOptions.map((s) => (
                          <SelectItem key={s} value={s}>{statusLabels[s]}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>

              {/* Goal */}
              <div className="space-y-4">
                <h3 className="font-semibold">Goal</h3>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="mission-target">Target *</Label>
                    <Input
                      id="mission-target"
                      type="number"
                      min="1"
                      required
                      value={formData.target}
                      onChange={(e) => setFormData({ ...formData, target: Math.max(1, Number(e.target.value) || 1) })}
                    />
                    <p className="text-xs text-muted-foreground">Units of progress needed to complete.</p>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="mission-max">Max completions per player</Label>
                    <Input
                      id="mission-max"
                      type="number"
                      min="1"
                      placeholder={formData.type === 'one_time' ? '1' : 'Unlimited'}
                      value={formData.max_completions_per_player}
                      onChange={(e) => setFormData({ ...formData, max_completions_per_player: e.target.value })}
                    />
                  </div>
                </div>
                <MissionCriteriaEditor
                  value={criteria}
                  onChange={(next) => { setCriteria(next); setCriteriaErrors({}); }}
                  errors={criteriaErrors}
                  unsupported={criteriaUnsupported}
                />
              </div>

              {/* Rewards */}
              <div className="space-y-4">
                <h3 className="font-semibold">Rewards</h3>
                <div className="grid grid-cols-3 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="mission-xp">XP Reward</Label>
                    <Input
                      id="mission-xp"
                      type="number"
                      min="0"
                      value={formData.xp_reward}
                      onChange={(e) => setFormData({ ...formData, xp_reward: Math.max(0, Number(e.target.value) || 0) })}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="mission-points">Points Reward</Label>
                    <Input
                      id="mission-points"
                      type="number"
                      min="0"
                      value={formData.points_reward}
                      onChange={(e) => setFormData({ ...formData, points_reward: Math.max(0, Number(e.target.value) || 0) })}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="mission-badge">Badge Reward</Label>
                    <Select
                      value={formData.badge_reward_id || NO_BADGE}
                      onValueChange={(value) => setFormData({ ...formData, badge_reward_id: value === NO_BADGE ? '' : value })}
                    >
                      <SelectTrigger id="mission-badge"><SelectValue placeholder="None" /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NO_BADGE}>None</SelectItem>
                        {badges.map((badge) => (
                          <SelectItem key={badge.id} value={badge.id}>{badge.name}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>

              {/* Schedule */}
              <div className="space-y-4">
                <h3 className="font-semibold">Schedule</h3>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="mission-start">Starts</Label>
                    <Input
                      id="mission-start"
                      type="date"
                      value={formData.starts_at}
                      onChange={(e) => setFormData({ ...formData, starts_at: e.target.value })}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="mission-end">Ends</Label>
                    <Input
                      id="mission-end"
                      type="date"
                      value={formData.ends_at}
                      onChange={(e) => setFormData({ ...formData, ends_at: e.target.value })}
                    />
                  </div>
                </div>
              </div>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>Cancel</Button>
              <Button type="submit" variant="glow" disabled={isSaving}>
                {isSaving && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                {editingMission ? 'Update Mission' : 'Create Mission'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Tabs defaultValue="missions" className="space-y-6">
        <TabsList>
          <TabsTrigger value="missions">Missions</TabsTrigger>
          <TabsTrigger value="analytics">Analytics</TabsTrigger>
        </TabsList>

        <TabsContent value="analytics">
          <MissionAnalyticsPanel />
        </TabsContent>

        <TabsContent value="missions" className="space-y-6">
          {/* Filters (server-side: ?status&type) */}
          <div className="flex flex-wrap gap-3">
            <Select value={statusFilter} onValueChange={setFilter(setStatusFilter)}>
              <SelectTrigger className="w-40" aria-label="Filter by status"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All statuses</SelectItem>
                {(Object.keys(statusLabels) as MissionStatus[]).map((s) => (
                  <SelectItem key={s} value={s}>{statusLabels[s]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={typeFilter} onValueChange={setFilter(setTypeFilter)}>
              <SelectTrigger className="w-40" aria-label="Filter by type"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All types</SelectItem>
                {(Object.keys(typeLabels) as MissionType[]).map((t) => (
                  <SelectItem key={t} value={t}>{typeLabels[t]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Missions Grid */}
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
              <p className="text-destructive">Failed to load missions: {error.message}</p>
            </Card>
          ) : missions.length === 0 ? (
            <Card>
              <CardContent className="p-12 text-center">
                <Target className="w-16 h-16 mx-auto mb-4 text-muted-foreground" />
                <h3 className="text-lg font-medium mb-2">No missions found</h3>
                <p className="text-muted-foreground mb-4">
                  {hasFilters ? 'Try different filters.' : 'Create your first mission to get started.'}
                </p>
                {!hasFilters && canManage && (
                  <Button onClick={openCreate}>
                    <Plus className="w-4 h-4 mr-2" />
                    Create Mission
                  </Button>
                )}
              </CardContent>
            </Card>
          ) : (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              {missions.map((mission, index) => {
                const criteriaKeys = Object.keys(mission.criteria ?? {});
                const criteriaSummary = summarizeCriteria(mission.criteria);
                const reward = badgeName(mission.badge_reward_id);
                return (
                  <Card key={mission.id} className="stat-card overflow-hidden group" style={{ animationDelay: `${index * 100}ms` }}>
                    <CardHeader className="pb-3">
                      <div className="flex items-start justify-between">
                        <div className="flex items-center gap-3">
                          <div className={cn(
                            "w-12 h-12 rounded-xl flex items-center justify-center",
                            mission.status === 'active' ? "bg-green-500/10" : "bg-amber-500/10"
                          )}>
                            <Target className={cn(
                              "w-6 h-6",
                              mission.status === 'active' ? "text-green-500" : "text-amber-500"
                            )} />
                          </div>
                          <div>
                            <CardTitle className="text-lg">{mission.name}</CardTitle>
                            <CardDescription>{mission.description || 'No description'}</CardDescription>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <Badge
                            variant="outline"
                            className={cn(
                              "capitalize",
                              mission.status === 'active'
                                ? "border-green-500/50 text-green-500 bg-green-500/10"
                                : mission.status === 'draft' || mission.status === 'paused'
                                ? "border-amber-500/50 text-amber-500 bg-amber-500/10"
                                : "border-muted-foreground"
                            )}
                          >
                            {mission.status}
                          </Badge>
                          {canManage && <ItemActionsMenu
                            itemName={mission.name}
                            onEdit={() => openEdit(mission)}
                            onDelete={() => handleDelete(mission)}
                            actions={[
                              { label: 'Start for player', icon: Play, onClick: () => openPlayerAction('start', mission), disabled: mission.status !== 'active' },
                              { label: 'Add progress', icon: TrendingUp, onClick: () => openPlayerAction('progress', mission), disabled: mission.status !== 'active' },
                              { label: 'Complete for player', icon: CheckCircle2, onClick: () => openPlayerAction('complete', mission), disabled: mission.status !== 'active' },
                              { label: 'Stats & attempts', icon: ListChecks, onClick: () => openStats(mission) },
                            ]}
                            showInGroup
                          />}
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent className="space-y-4">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="secondary">{typeLabels[mission.type]}</Badge>
                        <Badge variant="outline">Target: {mission.target}</Badge>
                        {mission.max_completions_per_player !== null && (
                          <Badge variant="outline">Max {mission.max_completions_per_player}× per player</Badge>
                        )}
                      </div>

                      {criteriaSummary ? (
                        <div className="space-y-1 rounded-md bg-secondary/50 p-2">
                          <p className="text-sm font-medium flex items-center gap-1">
                            <Zap className="w-4 h-4 text-primary" /> Auto-progress on <code className="text-xs">{criteriaSummary.eventType}</code>
                          </p>
                          {criteriaSummary.conditions.length > 0 && (
                            <p className="text-xs text-muted-foreground">When {criteriaSummary.conditions.join(' and ')}</p>
                          )}
                          <p className="text-xs text-muted-foreground">{criteriaSummary.increment}</p>
                        </div>
                      ) : criteriaKeys.length > 0 ? (
                        <div className="space-y-1">
                          <p className="text-sm font-medium">Criteria</p>
                          <code className="block text-xs bg-secondary rounded px-2 py-1 overflow-x-auto">
                            {JSON.stringify(mission.criteria)}
                          </code>
                        </div>
                      ) : (
                        <p className="text-xs text-muted-foreground">Manual progress (rules or API).</p>
                      )}

                      <div className="flex items-center justify-between pt-2 border-t border-border">
                        <div className="flex items-center gap-4 text-sm text-muted-foreground">
                          <Button type="button" size="sm" variant="ghost" className="h-7 px-2 gap-1" onClick={() => openStats(mission)}>
                            <BarChart3 className="w-4 h-4" /> Stats
                          </Button>
                          {(mission.starts_at || mission.ends_at) && (
                            <div className="flex items-center gap-1">
                              <Calendar className="w-4 h-4" />
                              {mission.starts_at ? new Date(mission.starts_at).toLocaleDateString() : '…'}
                              {' – '}
                              {mission.ends_at ? new Date(mission.ends_at).toLocaleDateString() : '…'}
                            </div>
                          )}
                        </div>
                        <div className="flex flex-wrap items-center gap-2">
                          {mission.xp_reward > 0 && <Badge variant="secondary">+{mission.xp_reward} XP</Badge>}
                          {mission.points_reward > 0 && <Badge variant="secondary">+{mission.points_reward} Points</Badge>}
                          {reward && (
                            <Badge variant="outline" className="border-primary/50 text-primary">🏅 {reward}</Badge>
                          )}
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )}

          <CursorPager
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            nextCursor={missionsData?.next_cursor}
            onPrevious={pager.previous}
            onNext={pager.next}
            isFetching={isFetching}
          />
        </TabsContent>
      </Tabs>

      {/* Player actions: start / progress / complete */}
      <PlayerActionDialog
        open={!!playerAction}
        onOpenChange={(open) => !open && setPlayerAction(null)}
        title={
          playerAction?.kind === 'start'
            ? `Start "${playerAction.mission.name}"`
            : playerAction?.kind === 'progress'
            ? `Add progress to "${playerAction.mission.name}"`
            : `Complete "${playerAction?.mission.name ?? ''}"`
        }
        description={
          playerAction?.kind === 'progress'
            ? 'Adds progress; starts an attempt if none is open and completes it when the target is reached.'
            : playerAction?.kind === 'complete'
            ? 'Completes the player\'s open attempt once its target is reached.'
            : 'Opens an attempt for the current period.'
        }
        submitLabel={playerAction?.kind === 'start' ? 'Start' : playerAction?.kind === 'progress' ? 'Add Progress' : 'Complete'}
        isPending={playerActionPending}
        onSubmit={handlePlayerAction}
      >
        {playerAction?.kind === 'progress' && (
          <div className="space-y-2">
            <Label htmlFor="mission-increment">Increment</Label>
            <Input
              id="mission-increment"
              type="number"
              min={1}
              max={1000000}
              value={increment}
              onChange={(e) => setIncrement(Math.max(1, Number(e.target.value) || 1))}
            />
          </div>
        )}
      </PlayerActionDialog>

      {/* Attempts */}
      <Dialog open={!!attemptsMission} onOpenChange={(open) => !open && setAttemptsMission(null)}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{attemptsMission?.name}</DialogTitle>
            <DialogDescription>Completion analytics and each player's runs at this mission, newest first.</DialogDescription>
          </DialogHeader>
          {missionStatsQuery.isLoading ? (
            <Skeleton className="h-20 w-full" />
          ) : missionStatsQuery.error ? (
            <p className="text-sm text-destructive">Failed to load statistics: {missionStatsQuery.error.message}</p>
          ) : missionStatsQuery.data ? (
            <MissionStatsSummary stats={missionStatsQuery.data} />
          ) : null}
          <h4 className="font-semibold text-sm pt-2">Attempts</h4>
          {attemptsQuery.isLoading ? (
            <p className="text-sm text-muted-foreground py-4">Loading attempts...</p>
          ) : attemptsQuery.error ? (
            <p className="text-sm text-destructive py-4">{attemptsQuery.error.message}</p>
          ) : (attemptsQuery.data?.data ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground py-4">No attempts yet.</p>
          ) : (
            <div className="space-y-3">
              {attemptsQuery.data!.data.map((attempt) => (
                <div key={attempt.id} className="rounded-md border border-border p-3 space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <code className="text-xs text-muted-foreground truncate">{attempt.player_id}</code>
                    <Badge variant="outline" className={cn('capitalize', attemptStatusClass[attempt.status])}>
                      {attempt.status.replace('_', ' ')}
                    </Badge>
                  </div>
                  <Progress value={Math.min(100, (attempt.progress / Math.max(1, attempt.target)) * 100)} />
                  <div className="flex justify-between text-xs text-muted-foreground">
                    <span>{attempt.progress} / {attempt.target} · period {attempt.period_key}</span>
                    <span>
                      Started {new Date(attempt.started_at).toLocaleDateString()}
                      {attempt.completed_at && ` · completed ${new Date(attempt.completed_at).toLocaleDateString()}`}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
          <CursorPager
            page={attemptsPager.page}
            hasPrevious={attemptsPager.hasPrevious}
            nextCursor={attemptsQuery.data?.next_cursor}
            onPrevious={attemptsPager.previous}
            onNext={attemptsPager.next}
            isFetching={attemptsQuery.isFetching}
          />
        </DialogContent>
      </Dialog>
    </div>
  );
}

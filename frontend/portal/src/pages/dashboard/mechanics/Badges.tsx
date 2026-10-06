import React, { useState } from 'react';
import { Plus, Award, Sparkles, Loader2, UserPlus, UserMinus, Zap, Users, BarChart3 } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge as BadgeUI } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { useToast } from '@/hooks/use-toast';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import CursorPager from '@/components/CursorPager';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import { PlayerActionDialog } from '@/components/mechanics/PlayerActionDialog';
import { changedFields } from '@/components/mechanics/patch';
import { BadgeRequirementsEditor } from '@/components/mechanics/BadgeRequirementsEditor';
import { StatBarChart } from '@/components/mechanics/StatBarChart';
import {
  draftToRequirements,
  emptyRequirementsDraft,
  requirementsToDraft,
  summarizeRequirements,
  validateRequirementsDraft,
  type RequirementsDraft,
} from '@/components/mechanics/requirements';
import { StatCard } from '@/components/players/StatCard';
import { useAuth } from '@/contexts/AuthContext';
import {
  useBadgesQuery,
  useBadgeStatsQuery,
  MechanicsApiError,
  useCreateBadgeMutation,
  useUpdateBadgeMutation,
  useDeleteBadgeMutation,
  useAwardBadgeMutation,
  useRevokeBadgeMutation,
  useInvalidateCreatedDraft,
  describeMechanicsError,
} from '@/services/queries/mechanics';
import type { Badge, BadgeFilters, CreateBadgeData, UpdateBadgeData, BadgeTier, BadgeCategory } from '@/services/api/types';

const tierColors: Record<BadgeTier, string> = {
  bronze: 'bg-amber-700/20 text-amber-700 border-amber-700/30',
  silver: 'bg-slate-400/20 text-slate-400 border-slate-400/30',
  gold: 'bg-yellow-500/20 text-yellow-500 border-yellow-500/30',
  platinum: 'bg-cyan-400/20 text-cyan-400 border-cyan-400/30',
  diamond: 'bg-purple-400/20 text-purple-400 border-purple-400/30',
};

const tierIcons: Record<BadgeTier, string> = {
  bronze: '🥉',
  silver: '🥈',
  gold: '🥇',
  platinum: '💎',
  diamond: '👑',
};

const categoryLabels: Record<BadgeCategory, string> = {
  achievement: 'Achievement',
  milestone: 'Milestone',
  skill: 'Skill',
  social: 'Social',
  exploration: 'Exploration',
  collection: 'Collection',
  special: 'Special',
  seasonal: 'Seasonal',
};

/** Messages for the award endpoint's 409 codes. */
const awardErrors: Record<string, string> = {
  badge_already_earned: 'This player already has this badge (it is not stackable).',
  badge_max_awards_reached: 'This badge has reached its maximum number of awards.',
  badge_inactive: 'This badge is inactive. Activate it before awarding.',
  player_inactive: 'This player is inactive.',
  player_not_found: 'Player not found.',
};

const ALL = 'all';

/** "Auto-award" summary of a badge's requirements on its card. */
function RequirementsSummary({ requirements }: { requirements: Badge['requirements'] }) {
  const { all, any } = summarizeRequirements(requirements);
  if (all.length === 0 && any.length === 0) return null;
  return (
    <div className="mt-3 text-left rounded-md bg-secondary/50 p-2 space-y-1">
      <p className="text-xs font-medium flex items-center gap-1">
        <Zap className="w-3 h-3 text-primary" /> Auto-awarded when
      </p>
      {all.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {all.length > 1 ? 'All of: ' : ''}{all.join(' · ')}
        </p>
      )}
      {any.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {all.length > 0 ? 'and any of: ' : 'Any of: '}{any.join(' · ')}
        </p>
      )}
    </div>
  );
}

/** Stats table and chart for GET /badges/stats. */
function BadgeStatsPanel() {
  const { data: stats, isLoading, error } = useBadgeStatsQuery();

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          {[...Array(3)].map((_, i) => <Skeleton key={i} className="h-28" />)}
        </div>
        <Skeleton className="h-72" />
      </div>
    );
  }
  if (error || !stats) {
    return (
      <Card className="p-8 text-center border-destructive">
        <p className="text-destructive">Failed to load badge statistics: {error?.message ?? 'no data'}</p>
      </Card>
    );
  }

  const awardedBadges = stats.badges.filter((b) => b.awarded_count > 0).length;
  const ranked = [...stats.badges].sort((a, b) => b.awarded_count - a.awarded_count);
  const chartData = stats.awards_per_day.map((d) => ({ date: d.date, awards: d.count }));

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <StatCard icon={<Award className="w-6 h-6" />} iconClassName="text-amber-500" value={stats.total_awarded} label="Awards applied (all time)" />
        <StatCard icon={<Users className="w-6 h-6" />} iconClassName="text-violet-500" value={stats.unique_players} label="Players with a badge" />
        <StatCard
          icon={<BarChart3 className="w-6 h-6" />}
          iconClassName="text-emerald-500"
          value={`${awardedBadges} / ${stats.badges.filter((b) => !b.deleted).length}`}
          label="Badges awarded at least once"
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Awards per day</CardTitle>
          <CardDescription>Applied awards over the last 30 days (UTC).</CardDescription>
        </CardHeader>
        <CardContent>
          {chartData.every((d) => d.awards === 0) ? (
            <p className="text-sm text-muted-foreground py-8 text-center">No awards in the last 30 days.</p>
          ) : (
            <StatBarChart
              data={chartData}
              xKey="date"
              series={[{ key: 'awards', label: 'Awards', color: 'hsl(var(--primary))' }]}
              tickFormatter={(d) => d.slice(5)}
            />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Per badge</CardTitle>
          <CardDescription>From the award ledger: revokes do not subtract.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {ranked.length === 0 ? (
            <p className="text-sm text-muted-foreground p-6 text-center">No badges yet.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left p-3 text-sm font-medium text-muted-foreground">Badge</th>
                    <th className="text-left p-3 text-sm font-medium text-muted-foreground">Tier</th>
                    <th className="text-right p-3 text-sm font-medium text-muted-foreground">Awards</th>
                    <th className="text-right p-3 text-sm font-medium text-muted-foreground">Players</th>
                    <th className="text-left p-3 text-sm font-medium text-muted-foreground">Last awarded</th>
                  </tr>
                </thead>
                <tbody>
                  {ranked.map((b) => (
                    <tr key={b.badge_id} className="border-b border-border/50">
                      <td className="p-3">
                        <span className="font-medium">{b.name}</span>
                        {b.deleted && <BadgeUI variant="outline" className="ml-2 text-xs text-muted-foreground">Deleted</BadgeUI>}
                      </td>
                      <td className="p-3">
                        <BadgeUI variant="outline" className={`capitalize ${tierColors[b.tier] ?? ''}`}>
                          {tierIcons[b.tier] ?? ''} {b.tier}
                        </BadgeUI>
                      </td>
                      <td className="p-3 text-right font-mono">{b.awarded_count.toLocaleString()}</td>
                      <td className="p-3 text-right font-mono">{b.unique_players.toLocaleString()}</td>
                      <td className="p-3 text-sm text-muted-foreground">
                        {b.last_awarded_at ? new Date(b.last_awarded_at).toLocaleString() : '-'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

interface BadgeFormState {
  name: string;
  description: string;
  icon_url: string;
  tier: BadgeTier;
  category: BadgeCategory;
  points_value: number;
  is_stackable: boolean;
  max_awards: number | null;
  is_active: boolean;
  is_secret: boolean;
}

const defaultFormState: BadgeFormState = {
  name: '',
  description: '',
  icon_url: '',
  tier: 'bronze',
  category: 'achievement',
  points_value: 0,
  is_stackable: false,
  max_awards: null,
  is_active: true,
  is_secret: false,
};

export default function Badges() {
  const [tierFilter, setTierFilter] = useState<string>(ALL);
  const [categoryFilter, setCategoryFilter] = useState<string>(ALL);
  const [activeFilter, setActiveFilter] = useState<string>(ALL);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingBadge, setEditingBadge] = useState<Badge | null>(null);
  const [formState, setFormState] = useState<BadgeFormState>(defaultFormState);
  const [awardingBadge, setAwardingBadge] = useState<Badge | null>(null);
  const [revokingBadge, setRevokingBadge] = useState<Badge | null>(null);
  const [requirements, setRequirements] = useState<RequirementsDraft>(emptyRequirementsDraft);
  const [requirementsUnsupported, setRequirementsUnsupported] = useState(false);
  const [requirementsError, setRequirementsError] = useState<string | null>(null);
  const { toast } = useToast();
  const invalidateCreatedDraft = useInvalidateCreatedDraft();
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:mechanics');
  const pager = useCursorPagination(24);

  const filters: BadgeFilters = {
    limit: pager.limit,
    cursor: pager.cursor,
    tier: tierFilter === ALL ? undefined : (tierFilter as BadgeTier),
    category: categoryFilter === ALL ? undefined : (categoryFilter as BadgeCategory),
    active: activeFilter === ALL ? undefined : activeFilter === 'active',
  };
  const { data: badgesData, isLoading, isFetching, error } = useBadgesQuery(filters);
  const createMutation = useCreateBadgeMutation();
  const updateMutation = useUpdateBadgeMutation();
  const deleteMutation = useDeleteBadgeMutation();
  const awardMutation = useAwardBadgeMutation();
  const revokeMutation = useRevokeBadgeMutation();

  const badges = badgesData?.data ?? [];

  const setFilter = (setter: (v: string) => void) => (value: string) => {
    setter(value);
    pager.reset();
  };

  const handleOpenDialog = (badge?: Badge) => {
    if (badge) {
      setEditingBadge(badge);
      setFormState({
        name: badge.name,
        description: badge.description,
        icon_url: badge.icon_url,
        tier: badge.tier,
        category: badge.category,
        points_value: badge.points_value,
        is_stackable: badge.is_stackable,
        max_awards: badge.max_awards,
        is_active: badge.is_active,
        is_secret: badge.is_secret,
      });
      const { draft, unsupported } = requirementsToDraft(badge.requirements);
      setRequirements(draft);
      setRequirementsUnsupported(unsupported);
    } else {
      setEditingBadge(null);
      setFormState(defaultFormState);
      setRequirements(emptyRequirementsDraft);
      setRequirementsUnsupported(false);
    }
    setRequirementsError(null);
    setIsDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setIsDialogOpen(false);
    setEditingBadge(null);
    setFormState(defaultFormState);
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const invalid = validateRequirementsDraft(requirements);
    if (invalid) {
      setRequirementsError(invalid);
      return;
    }
    setRequirementsError(null);
    const requirementsPayload = draftToRequirements(requirements) as Record<string, unknown> | null;

    try {
      if (editingBadge) {
        const next: UpdateBadgeData = { ...formState };
        const patch = changedFields(editingBadge, next);
        // null clears the requirements; only send them when they changed.
        const before = editingBadge.requirements && Object.keys(editingBadge.requirements).length > 0 ? editingBadge.requirements : null;
        if (requirementsUnsupported ? requirementsPayload !== null : JSON.stringify(before) !== JSON.stringify(requirementsPayload)) {
          patch.requirements = requirementsPayload;
        }
        if (Object.keys(patch).length > 0) {
          await updateMutation.mutateAsync({ badgeId: editingBadge.id, data: patch });
        }
        toast({ title: 'Badge updated', description: 'Badge has been updated successfully.' });
      } else {
        const createData: CreateBadgeData = {
          name: formState.name,
          description: formState.description || undefined,
          icon_url: formState.icon_url || undefined,
          tier: formState.tier,
          category: formState.category,
          points_value: formState.points_value,
          is_stackable: formState.is_stackable,
          max_awards: formState.max_awards ?? undefined,
          is_active: formState.is_active,
          is_secret: formState.is_secret,
          requirements: requirementsPayload ?? undefined,
        };
        await createMutation.mutateAsync(createData);
        toast({ title: 'Badge created', description: 'New badge has been created successfully.' });
      }
      handleCloseDialog();
    } catch (err) {
      if (err instanceof MechanicsApiError && err.code === 'invalid_badge_requirements') {
        setRequirementsError(err.validationErrors?.requirements?.[0] ?? err.message);
      }
      toast({
        title: 'Error',
        description: describeMechanicsError(err, 'Something went wrong', {
          badge_slug_taken: 'A badge with this name/slug already exists.',
          version_conflict: 'The badge was changed by someone else. Reload and try again.',
          invalid_badge_requirements: 'The requirements are not valid: see the requirements section.',
        }),
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async (badge: Badge) => {
    try {
      await deleteMutation.mutateAsync(badge.id);
      toast({ title: 'Badge deleted', description: 'Badge has been removed successfully.' });
    } catch (err) {
      toast({
        title: 'Error',
        description: describeMechanicsError(err, 'Failed to delete badge'),
        variant: 'destructive',
      });
    }
  };

  const handleAward = async (playerId: string) => {
    if (!awardingBadge) return;
    try {
      const result = await awardMutation.mutateAsync({ badge_id: awardingBadge.id, player_id: playerId });
      toast({
        title: result.replay ? 'Already applied' : 'Badge awarded',
        description: `"${awardingBadge.name}" awarded (earned ${result.player_badge.earned_count}×).`,
      });
    } catch (err) {
      toast({
        title: 'Could not award badge',
        description: describeMechanicsError(err, 'Failed to award badge', awardErrors),
        variant: 'destructive',
      });
      throw err;
    }
  };

  const handleRevoke = async (playerId: string) => {
    if (!revokingBadge) return;
    try {
      await revokeMutation.mutateAsync({ playerId, badgeId: revokingBadge.id });
      toast({ title: 'Badge revoked', description: `"${revokingBadge.name}" was revoked from the player.` });
    } catch (err) {
      toast({
        title: 'Could not revoke badge',
        description: describeMechanicsError(err, 'Failed to revoke badge'),
        variant: 'destructive',
      });
      throw err;
    }
  };


  const isSubmitting = createMutation.isPending || updateMutation.isPending;
  const hasFilters = tierFilter !== ALL || categoryFilter !== ALL || activeFilter !== ALL;

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Badges</h1>
          <p className="text-muted-foreground mt-1">Create and manage achievement badges for your users.</p>
        </div>
        {canManage && <div className="flex gap-2">
          <AIGenerateDialog
            trigger={
              <Button variant="outline" className="gap-2">
                <Sparkles className="w-4 h-4" />
                Generate with AI
              </Button>
            }
            title="Generate Badge Ideas"
            placeholder="E.g., Create a badge for users who complete 100 purchases..."
            context="Generate badge names, descriptions, and tier suggestions"
            kind="badge"
            onCreated={invalidateCreatedDraft}
          />
          <Dialog open={isDialogOpen} onOpenChange={(open) => { if (!open) handleCloseDialog(); else setIsDialogOpen(true); }}>
            <DialogTrigger asChild>
              <Button variant="glow" onClick={() => handleOpenDialog()}>
                <Plus className="w-4 h-4" />
                Create Badge
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
              <form onSubmit={handleSubmit}>
                {/* Live Preview */}
                <div className="mb-6 p-4 rounded-lg bg-secondary/50 border border-border">
                  <p className="text-xs text-muted-foreground uppercase tracking-wide mb-3">Live Preview</p>
                  <div className="flex items-center gap-4">
                    <div className="w-16 h-16 rounded-full flex items-center justify-center text-3xl bg-background border-2 border-border transition-all shrink-0">
                      {formState.icon_url ? (
                        <img
                          src={formState.icon_url}
                          alt="Badge preview"
                          className="w-full h-full rounded-full object-cover"
                          onError={(e) => {
                            e.currentTarget.style.display = 'none';
                            e.currentTarget.nextElementSibling?.classList.remove('hidden');
                          }}
                        />
                      ) : null}
                      <span className={formState.icon_url ? 'hidden' : ''}>
                        {tierIcons[formState.tier]}
                      </span>
                    </div>
                    <div className="flex-1 min-w-0">
                      <h4 className="font-semibold text-lg truncate">
                        {formState.name || 'Badge Name'}
                      </h4>
                      <p className="text-sm text-muted-foreground line-clamp-2">
                        {formState.description || 'Badge description will appear here...'}
                      </p>
                      <div className="flex flex-wrap gap-1.5 mt-2">
                        <BadgeUI variant="outline" className={`text-xs ${tierColors[formState.tier]}`}>
                          {tierIcons[formState.tier]} {formState.tier}
                        </BadgeUI>
                        <BadgeUI variant="outline" className="text-xs bg-secondary">
                          {categoryLabels[formState.category]}
                        </BadgeUI>
                        {formState.points_value > 0 && (
                          <BadgeUI variant="outline" className="text-xs bg-secondary">
                            <Award className="w-3 h-3 mr-1" />
                            {formState.points_value} pts
                          </BadgeUI>
                        )}
                        {formState.is_secret && (
                          <BadgeUI variant="outline" className="text-xs bg-amber-500/20 text-amber-500 border-amber-500/30">
                            🔒 Secret
                          </BadgeUI>
                        )}
                        {!formState.is_active && (
                          <BadgeUI variant="outline" className="text-xs bg-muted text-muted-foreground">
                            Inactive
                          </BadgeUI>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
                <DialogHeader>
                  <DialogTitle>{editingBadge ? 'Edit Badge' : 'Create New Badge'}</DialogTitle>
                  <DialogDescription>
                    {editingBadge ? 'Update the badge details.' : 'Design a new achievement badge for your users.'}
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <Label htmlFor="name">Badge Name *</Label>
                    <Input
                      id="name"
                      placeholder="e.g., Super Achiever"
                      value={formState.name}
                      maxLength={255}
                      onChange={(e) => setFormState(prev => ({ ...prev, name: e.target.value }))}
                      required
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="description">Description</Label>
                    <Input
                      id="description"
                      placeholder="What does the user need to do?"
                      value={formState.description}
                      maxLength={1000}
                      onChange={(e) => setFormState(prev => ({ ...prev, description: e.target.value }))}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="icon_url">Icon URL</Label>
                    <Input
                      id="icon_url"
                      type="url"
                      placeholder="https://example.com/badge.png"
                      value={formState.icon_url}
                      onChange={(e) => setFormState(prev => ({ ...prev, icon_url: e.target.value }))}
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label>Tier</Label>
                      <Select
                        value={formState.tier}
                        onValueChange={(value: BadgeTier) => setFormState(prev => ({ ...prev, tier: value }))}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {(Object.keys(tierIcons) as BadgeTier[]).map((tier) => (
                            <SelectItem key={tier} value={tier} className="capitalize">
                              {tierIcons[tier]} {tier}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label>Category</Label>
                      <Select
                        value={formState.category}
                        onValueChange={(value: BadgeCategory) => setFormState(prev => ({ ...prev, category: value }))}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {Object.entries(categoryLabels).map(([key, label]) => (
                            <SelectItem key={key} value={key}>{label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="points_value">Points Value</Label>
                      <Input
                        id="points_value"
                        type="number"
                        min="0"
                        value={formState.points_value}
                        onChange={(e) => setFormState(prev => ({ ...prev, points_value: Math.max(0, Number(e.target.value) || 0) }))}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="max_awards">Max Awards</Label>
                      <Input
                        id="max_awards"
                        type="number"
                        min="1"
                        placeholder="Unlimited"
                        value={formState.max_awards ?? ''}
                        onChange={(e) => setFormState(prev => ({
                          ...prev,
                          max_awards: e.target.value ? Number(e.target.value) : null,
                        }))}
                      />
                    </div>
                  </div>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center space-x-2">
                      <Switch
                        id="is_stackable"
                        checked={formState.is_stackable}
                        onCheckedChange={(checked) => setFormState(prev => ({ ...prev, is_stackable: checked }))}
                      />
                      <Label htmlFor="is_stackable">Stackable</Label>
                    </div>
                    <div className="flex items-center space-x-2">
                      <Switch
                        id="is_secret"
                        checked={formState.is_secret}
                        onCheckedChange={(checked) => setFormState(prev => ({ ...prev, is_secret: checked }))}
                      />
                      <Label htmlFor="is_secret">Secret</Label>
                    </div>
                    <div className="flex items-center space-x-2">
                      <Switch
                        id="is_active"
                        checked={formState.is_active}
                        onCheckedChange={(checked) => setFormState(prev => ({ ...prev, is_active: checked }))}
                      />
                      <Label htmlFor="is_active">Active</Label>
                    </div>
                  </div>
                  <BadgeRequirementsEditor
                    value={requirements}
                    onChange={(next) => { setRequirements(next); setRequirementsError(null); }}
                    unsupported={requirementsUnsupported}
                    error={requirementsError}
                  />
                </div>
                <DialogFooter>
                  <Button type="button" variant="outline" onClick={handleCloseDialog}>
                    Cancel
                  </Button>
                  <Button type="submit" variant="glow" disabled={isSubmitting}>
                    {isSubmitting && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                    {editingBadge ? 'Save Changes' : 'Create Badge'}
                  </Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        </div>}
      </div>

      <Tabs defaultValue="badges" className="space-y-6">
        <TabsList>
          <TabsTrigger value="badges">Badges</TabsTrigger>
          <TabsTrigger value="stats">Statistics</TabsTrigger>
        </TabsList>

        <TabsContent value="stats">
          <BadgeStatsPanel />
        </TabsContent>

        <TabsContent value="badges" className="space-y-6">
          {/* Filters (server-side: ?tier&category&active) */}
          <div className="flex flex-wrap gap-3">
            <Select value={tierFilter} onValueChange={setFilter(setTierFilter)}>
              <SelectTrigger className="w-40" aria-label="Filter by tier"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All tiers</SelectItem>
                {(Object.keys(tierIcons) as BadgeTier[]).map((tier) => (
                  <SelectItem key={tier} value={tier} className="capitalize">{tierIcons[tier]} {tier}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={categoryFilter} onValueChange={setFilter(setCategoryFilter)}>
              <SelectTrigger className="w-44" aria-label="Filter by category"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All categories</SelectItem>
                {Object.entries(categoryLabels).map(([key, label]) => (
                  <SelectItem key={key} value={key}>{label}</SelectItem>
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

          {/* Loading State */}
          {isLoading && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
              {[...Array(8)].map((_, i) => (
                <Card key={i} className="stat-card">
                  <CardContent className="p-6 text-center">
                    <Skeleton className="w-20 h-20 rounded-full mx-auto mb-4" />
                    <Skeleton className="h-5 w-32 mx-auto mb-2" />
                    <Skeleton className="h-4 w-40 mx-auto mb-3" />
                    <Skeleton className="h-6 w-20 mx-auto" />
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {/* Error State */}
          {error && (
            <Card className="p-8 text-center border-destructive">
              <p className="text-destructive">Failed to load badges: {error.message}</p>
            </Card>
          )}

          {/* Badges Grid */}
          {!isLoading && !error && badges.length > 0 && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
              {badges.map((badge, index) => (
                <Card
                  key={badge.id}
                  className="stat-card group"
                  style={{ animationDelay: `${index * 50}ms` }}
                >
                  <CardContent className="p-6 text-center relative">
                    {canManage && <div className="absolute top-2 right-2">
                      <ItemActionsMenu
                        itemName={badge.name}
                        onEdit={() => handleOpenDialog(badge)}
                        onDelete={() => handleDelete(badge)}
                        actions={[
                          { label: 'Award to player', icon: UserPlus, onClick: () => setAwardingBadge(badge), disabled: !badge.is_active },
                          { label: 'Revoke from player', icon: UserMinus, onClick: () => setRevokingBadge(badge) },
                        ]}
                        showInGroup
                      />
                    </div>}
                    <div className="w-20 h-20 rounded-full mx-auto mb-4 flex items-center justify-center text-4xl transition-transform group-hover:scale-110 bg-secondary">
                      {badge.icon_url ? (
                        <img src={badge.icon_url} alt={badge.name} className="w-full h-full rounded-full object-cover" />
                      ) : (
                        tierIcons[badge.tier]
                      )}
                    </div>
                    <h3 className="font-semibold text-lg mb-1">{badge.name}</h3>
                    {badge.description && (
                      <p className="text-sm text-muted-foreground mb-3 line-clamp-2">{badge.description}</p>
                    )}
                    <div className="flex flex-wrap gap-2 justify-center">
                      <BadgeUI variant="outline" className={tierColors[badge.tier]}>
                        {tierIcons[badge.tier]} {badge.tier}
                      </BadgeUI>
                      <BadgeUI variant="outline" className="bg-secondary">
                        <Award className="w-3 h-3 mr-1" />
                        {badge.points_value} pts
                      </BadgeUI>
                      {badge.is_stackable && (
                        <BadgeUI variant="outline" className="bg-secondary">Stackable</BadgeUI>
                      )}
                      {badge.max_awards !== null && (
                        <BadgeUI variant="outline" className="bg-secondary">Max {badge.max_awards}</BadgeUI>
                      )}
                      {badge.is_secret && (
                        <BadgeUI variant="outline" className="bg-amber-500/20 text-amber-500 border-amber-500/30">🔒 Secret</BadgeUI>
                      )}
                    </div>
                    {!badge.is_active && (
                      <BadgeUI variant="outline" className="mt-2 bg-muted text-muted-foreground">
                        Inactive
                      </BadgeUI>
                    )}
                    <RequirementsSummary requirements={badge.requirements} />
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {/* Empty State */}
          {!isLoading && !error && badges.length === 0 && (
            <Card className="p-8 text-center">
              <div className="flex flex-col items-center gap-3">
                <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center">
                  <Award className="w-8 h-8 text-muted-foreground" />
                </div>
                <div>
                  <p className="font-medium">No badges found</p>
                  <p className="text-sm text-muted-foreground">
                    {hasFilters ? 'Try different filters.' : 'Create your first badge to reward your users.'}
                  </p>
                </div>
              </div>
            </Card>
          )}

          <CursorPager
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            nextCursor={badgesData?.next_cursor}
            onPrevious={pager.previous}
            onNext={pager.next}
            isFetching={isFetching}
          />
        </TabsContent>
      </Tabs>

      <PlayerActionDialog
        open={!!awardingBadge}
        onOpenChange={(open) => !open && setAwardingBadge(null)}
        title={`Award "${awardingBadge?.name ?? ''}"`}
        description="Grants the badge (and its points) to a player. Safe to retry: the request is idempotent."
        submitLabel="Award Badge"
        isPending={awardMutation.isPending}
        onSubmit={handleAward}
      />

      <PlayerActionDialog
        open={!!revokingBadge}
        onOpenChange={(open) => !open && setRevokingBadge(null)}
        title={`Revoke "${revokingBadge?.name ?? ''}"`}
        description="Removes the badge from a player."
        submitLabel="Revoke Badge"
        isPending={revokeMutation.isPending}
        onSubmit={handleRevoke}
        destructive
      />
    </div>
  );
}

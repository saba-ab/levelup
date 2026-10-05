import React, { useState } from 'react';
import { Plus, Gift, Clock, Sparkles, Loader2, ShoppingCart, RefreshCw, CheckCircle2, XCircle, Ticket } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
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
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { cn } from '@/lib/utils';
import CursorPager from '@/components/CursorPager';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import { PlayerActionDialog } from '@/components/mechanics/PlayerActionDialog';
import { PlayerPicker } from '@/components/mechanics/PlayerPicker';
import { changedFields, dateToRfc3339, optionalNumber, rfc3339ToDate } from '@/components/mechanics/patch';
import {
  useRewardsQuery,
  useRewardClaimQuery,
  usePlayerRewardsQuery,
  useAllBadgesQuery,
  useLevelsQuery,
  useCreateRewardMutation,
  useUpdateRewardMutation,
  useDeleteRewardMutation,
  useClaimRewardMutation,
  useRedeemRewardMutation,
  useCancelRewardClaimMutation,
  describeMechanicsError,
} from '@/services/queries/mechanics';
import type {
  Reward,
  RewardClaim,
  RewardClaimStatus,
  RewardFilters,
  RewardStatus,
  RewardType,
  RewardValueType,
  CreateRewardData,
  UpdateRewardData,
} from '@/services/api/types';

const typeLabels: Record<RewardType, { label: string; icon: string }> = {
  points: { label: 'Points', icon: '🪙' },
  discount: { label: 'Discount', icon: '🏷️' },
  item: { label: 'Item', icon: '📦' },
  badge: { label: 'Badge', icon: '🏅' },
  level: { label: 'Level', icon: '⭐' },
  custom: { label: 'Custom', icon: '🎁' },
};

const statusLabels: Record<RewardStatus, string> = {
  draft: 'Draft',
  active: 'Active',
  paused: 'Paused',
  expired: 'Expired',
  depleted: 'Depleted',
};

const claimStatusClass: Record<RewardClaimStatus, string> = {
  pending_payment: 'border-amber-500/50 text-amber-500 bg-amber-500/10',
  claimed: 'border-blue-500/50 text-blue-500 bg-blue-500/10',
  redeemed: 'border-green-500/50 text-green-500 bg-green-500/10',
  rejected: 'border-destructive/50 text-destructive bg-destructive/10',
  expired: 'border-muted-foreground text-muted-foreground',
  cancelled: 'border-muted-foreground text-muted-foreground',
  refund_pending: 'border-amber-500/50 text-amber-500 bg-amber-500/10',
  refunded: 'border-muted-foreground text-muted-foreground',
};

const rewardErrors: Record<string, string> = {
  slug_taken: 'A reward with this name/slug already exists.',
  invalid_value: 'Value must be a decimal number with at most 2 decimals (e.g. 10 or 12.50).',
  invalid_value_type: 'Value type must be percentage or fixed.',
  invalid_reward_window: 'The end date must not be before the start date.',
  max_redemptions_below_used: 'Max redemptions cannot be lower than the stock already used.',
  version_conflict: 'The reward was changed by someone else. Reload and try again.',
  reward_not_available: 'This reward is not available (inactive, paused or outside its window).',
  reward_depleted: 'This reward is out of stock.',
  player_limit_reached: 'The player has reached the maximum claims for this reward.',
  level_requirement_not_met: "The player's level is below this reward's requirement.",
  insufficient_balance: 'The player does not have enough points.',
  player_inactive: 'This player is inactive.',
  invalid_status_transition: 'This claim cannot change to that status.',
  claim_expired: 'This claim has expired and cannot be redeemed.',
};

const ALL = 'all';
const NONE = 'none';
const PENDING: RewardClaimStatus[] = ['pending_payment', 'refund_pending'];

interface RewardFormState {
  name: string;
  description: string;
  type: RewardType;
  status: RewardStatus;
  points_cost: number;
  value: string;
  value_type: RewardValueType | '';
  badge_reward_id: string;
  level_reward_id: string;
  max_redemptions: string;
  max_per_player: string;
  claim_ttl_days: string;
  level_requirement: string;
  start_at: string;
  end_at: string;
  is_active: boolean;
}

const initialForm: RewardFormState = {
  name: '',
  description: '',
  type: 'custom',
  status: 'active',
  points_cost: 0,
  value: '',
  value_type: '',
  badge_reward_id: '',
  level_reward_id: '',
  max_redemptions: '',
  max_per_player: '',
  claim_ttl_days: '',
  level_requirement: '',
  start_at: '',
  end_at: '',
  is_active: true,
};

/** Decimal string with at most 2 decimals and 8 integer digits (backend rule). */
const isValidDecimal = (value: string) => /^\d{1,8}(\.\d{1,2})?$/.test(value);

const formatDateTime = (value: string | null) => (value ? new Date(value).toLocaleString() : '-');

function ClaimStatusBadge({ status }: { status: RewardClaimStatus }) {
  return (
    <Badge variant="outline" className={cn('capitalize', claimStatusClass[status])}>
      {status.replace('_', ' ')}
    </Badge>
  );
}

export default function Rewards() {
  const { toast } = useToast();
  const [statusFilter, setStatusFilter] = useState<string>(ALL);
  const [typeFilter, setTypeFilter] = useState<string>(ALL);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingReward, setEditingReward] = useState<Reward | null>(null);
  const [form, setForm] = useState<RewardFormState>(initialForm);
  const [claimingReward, setClaimingReward] = useState<Reward | null>(null);
  const [trackedClaimId, setTrackedClaimId] = useState('');
  const [claimsPlayerId, setClaimsPlayerId] = useState('');
  const pager = useCursorPagination(24);
  const claimsPager = useCursorPagination(20);

  const filters: RewardFilters = {
    limit: pager.limit,
    cursor: pager.cursor,
    status: statusFilter === ALL ? undefined : (statusFilter as RewardStatus),
    type: typeFilter === ALL ? undefined : (typeFilter as RewardType),
  };
  const { data: rewardsData, isLoading, isFetching, error } = useRewardsQuery(filters);
  const { data: badges = [] } = useAllBadgesQuery();
  const { data: levels = [] } = useLevelsQuery();

  // A paid claim is accepted as pending_payment and settles asynchronously; the query polls while pending.
  const trackedClaim = useRewardClaimQuery(trackedClaimId);
  const trackedPending = !!trackedClaim.data && PENDING.includes(trackedClaim.data.status);

  const playerClaims = usePlayerRewardsQuery(claimsPlayerId, { limit: claimsPager.limit, cursor: claimsPager.cursor });

  const createMutation = useCreateRewardMutation();
  const updateMutation = useUpdateRewardMutation();
  const deleteMutation = useDeleteRewardMutation();
  const claimMutation = useClaimRewardMutation();
  const redeemMutation = useRedeemRewardMutation();
  const cancelMutation = useCancelRewardClaimMutation();

  const rewards = rewardsData?.data ?? [];
  const rewardName = (rewardId: string) => rewards.find((r) => r.id === rewardId)?.name;

  const setFilter = (setter: (v: string) => void) => (value: string) => {
    setter(value);
    pager.reset();
  };

  const openCreate = () => {
    setEditingReward(null);
    setForm(initialForm);
    setIsDialogOpen(true);
  };

  const openEdit = (reward: Reward) => {
    setEditingReward(reward);
    setForm({
      name: reward.name,
      description: reward.description,
      type: reward.type,
      status: reward.status,
      points_cost: reward.points_cost,
      value: reward.value ?? '',
      value_type: reward.value_type ?? '',
      badge_reward_id: reward.badge_reward_id ?? '',
      level_reward_id: reward.level_reward_id ?? '',
      max_redemptions: reward.max_redemptions?.toString() ?? '',
      max_per_player: reward.max_per_player?.toString() ?? '',
      claim_ttl_days: reward.claim_ttl_days?.toString() ?? '',
      level_requirement: reward.level_requirement?.toString() ?? '',
      start_at: rfc3339ToDate(reward.start_at),
      end_at: rfc3339ToDate(reward.end_at),
      is_active: reward.is_active,
    });
    setIsDialogOpen(true);
  };

  const formToData = (): CreateRewardData => ({
    name: form.name,
    description: form.description,
    type: form.type,
    status: form.status,
    points_cost: form.points_cost,
    value: form.value || undefined,
    value_type: form.value_type || undefined,
    badge_reward_id: form.type === 'badge' ? form.badge_reward_id || undefined : undefined,
    level_reward_id: form.type === 'level' ? form.level_reward_id || undefined : undefined,
    max_redemptions: optionalNumber(form.max_redemptions),
    max_per_player: optionalNumber(form.max_per_player),
    claim_ttl_days: optionalNumber(form.claim_ttl_days),
    level_requirement: optionalNumber(form.level_requirement),
    start_at: dateToRfc3339(form.start_at),
    end_at: dateToRfc3339(form.end_at),
    is_active: form.is_active,
  });

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (form.value && !isValidDecimal(form.value)) {
      toast({ title: 'Validation Error', description: rewardErrors.invalid_value, variant: 'destructive' });
      return;
    }
    try {
      if (editingReward) {
        const next: UpdateRewardData = formToData();
        const patch = changedFields(editingReward, next);
        // Decimal values come back normalised ("10" -> "10.00"): compare numerically.
        if (patch.value !== undefined && editingReward.value !== null && Number(patch.value) === Number(editingReward.value)) {
          delete patch.value;
        }
        if (form.start_at === rfc3339ToDate(editingReward.start_at)) delete patch.start_at;
        if (form.end_at === rfc3339ToDate(editingReward.end_at)) delete patch.end_at;
        if (Object.keys(patch).length > 0) {
          await updateMutation.mutateAsync({ rewardId: editingReward.id, data: patch });
        }
        toast({ title: 'Reward updated', description: 'Reward has been updated successfully.' });
      } else {
        const data = formToData();
        await createMutation.mutateAsync({ ...data, description: data.description || undefined });
        toast({ title: 'Reward created', description: 'New reward has been added to the catalog.' });
      }
      setIsDialogOpen(false);
    } catch (err) {
      toast({ title: 'Error', description: describeMechanicsError(err, 'Failed to save reward', rewardErrors), variant: 'destructive' });
    }
  };

  const handleDelete = async (reward: Reward) => {
    try {
      await deleteMutation.mutateAsync(reward.id);
      toast({ title: 'Reward deleted', description: 'Reward has been removed successfully.' });
    } catch (err) {
      toast({ title: 'Error', description: describeMechanicsError(err, 'Failed to delete reward'), variant: 'destructive' });
    }
  };

  const handleClaim = async (playerId: string) => {
    if (!claimingReward) return;
    try {
      const claim = await claimMutation.mutateAsync({ reward_id: claimingReward.id, player_id: playerId });
      setTrackedClaimId(claim.id);
      if (claim.status === 'pending_payment') {
        toast({
          title: 'Claim pending',
          description: `${claim.points_cost.toLocaleString()} points are being debited. The claim status below updates automatically.`,
        });
      } else {
        toast({ title: 'Reward claimed', description: claim.code ? `Code: ${claim.code}` : `${claimingReward.name} was claimed.` });
      }
    } catch (err) {
      toast({ title: 'Could not claim reward', description: describeMechanicsError(err, 'Failed to claim reward', rewardErrors), variant: 'destructive' });
      throw err;
    }
  };

  const handleRedeem = async (claim: RewardClaim) => {
    try {
      await redeemMutation.mutateAsync(claim.id);
      toast({ title: 'Claim redeemed', description: 'The reward has been marked as redeemed.' });
    } catch (err) {
      toast({ title: 'Error', description: describeMechanicsError(err, 'Failed to redeem claim', rewardErrors), variant: 'destructive' });
    }
  };

  const handleCancel = async (claim: RewardClaim) => {
    try {
      const updated = await cancelMutation.mutateAsync(claim.id);
      toast({
        title: updated.status === 'refund_pending' ? 'Refund pending' : 'Claim cancelled',
        description: updated.status === 'refund_pending' ? 'The points are being refunded.' : 'The claim was cancelled.',
      });
    } catch (err) {
      toast({ title: 'Error', description: describeMechanicsError(err, 'Failed to cancel claim', rewardErrors), variant: 'destructive' });
    }
  };

  const handleAIGenerate = async (prompt: string): Promise<string> => {
    await new Promise(resolve => setTimeout(resolve, 1500));
    return `Generated Reward Idea:\n\n"${prompt}"\n\nName: Exclusive Perk\nDescription: A unique reward that drives user engagement.\nSuggested cost: 2,500 points\nType: Custom`;
  };

  const isSaving = createMutation.isPending || updateMutation.isPending;
  const hasFilters = statusFilter !== ALL || typeFilter !== ALL;

  const claimActions = (claim: RewardClaim) => (
    <div className="flex justify-end gap-2">
      {claim.status === 'claimed' && (
        <Button size="sm" variant="outline" onClick={() => handleRedeem(claim)} disabled={redeemMutation.isPending}>
          <CheckCircle2 className="w-4 h-4 mr-1" /> Redeem
        </Button>
      )}
      {(claim.status === 'claimed' || claim.status === 'pending_payment') && (
        <Button size="sm" variant="outline" onClick={() => handleCancel(claim)} disabled={cancelMutation.isPending}>
          <XCircle className="w-4 h-4 mr-1" /> Cancel
        </Button>
      )}
    </div>
  );

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Rewards Catalog</h1>
          <p className="text-muted-foreground mt-1">Manage rewards and player claims.</p>
        </div>
        <div className="flex gap-2">
          <AIGenerateDialog
            trigger={
              <Button variant="outline" className="gap-2">
                <Sparkles className="w-4 h-4" />
                Generate with AI
              </Button>
            }
            title="Generate Reward Ideas"
            placeholder="E.g., Create a reward for loyal customers that feels exclusive..."
            context="Generate reward names, descriptions, pricing, and types"
            onGenerate={handleAIGenerate}
          />
          <Button variant="glow" onClick={openCreate}>
            <Plus className="w-4 h-4" />
            Add Reward
          </Button>
        </div>
      </div>

      {/* Create / Edit dialog */}
      <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>{editingReward ? 'Edit Reward' : 'Add New Reward'}</DialogTitle>
              <DialogDescription>
                {editingReward ? 'Update the reward details.' : 'Create a new reward for players to claim.'}
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="reward-name">Reward Name *</Label>
                <Input id="reward-name" required maxLength={255} placeholder="e.g., Free Shipping" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="reward-description">Description</Label>
                <Textarea id="reward-description" rows={2} maxLength={1000} placeholder="What does the player get?" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </div>
              <div className="grid grid-cols-3 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="reward-type">Type</Label>
                  <Select value={form.type} onValueChange={(v) => setForm({ ...form, type: v as RewardType })}>
                    <SelectTrigger id="reward-type"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {(Object.keys(typeLabels) as RewardType[]).map((t) => (
                        <SelectItem key={t} value={t}>{typeLabels[t].icon} {typeLabels[t].label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reward-status">Status</Label>
                  <Select value={form.status} onValueChange={(v) => setForm({ ...form, status: v as RewardStatus })}>
                    <SelectTrigger id="reward-status"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {(Object.keys(statusLabels) as RewardStatus[]).map((s) => (
                        <SelectItem key={s} value={s}>{statusLabels[s]}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reward-cost">Cost (points) *</Label>
                  <Input
                    id="reward-cost"
                    type="number"
                    min="0"
                    required
                    value={form.points_cost}
                    onChange={(e) => setForm({ ...form, points_cost: Math.max(0, Number(e.target.value) || 0) })}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="reward-value">Value</Label>
                  <Input
                    id="reward-value"
                    inputMode="decimal"
                    placeholder="e.g., 10 or 12.50"
                    value={form.value}
                    onChange={(e) => setForm({ ...form, value: e.target.value.trim() })}
                  />
                  {form.value && !isValidDecimal(form.value) && (
                    <p className="text-xs text-destructive">At most 2 decimals, e.g. 10 or 12.50.</p>
                  )}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reward-value-type">Value type</Label>
                  <Select value={form.value_type || NONE} onValueChange={(v) => setForm({ ...form, value_type: v === NONE ? '' : (v as RewardValueType) })}>
                    <SelectTrigger id="reward-value-type"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value={NONE}>None</SelectItem>
                      <SelectItem value="percentage">Percentage</SelectItem>
                      <SelectItem value="fixed">Fixed amount</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              {form.type === 'badge' && (
                <div className="space-y-2">
                  <Label htmlFor="reward-badge">Badge granted</Label>
                  <Select value={form.badge_reward_id || NONE} onValueChange={(v) => setForm({ ...form, badge_reward_id: v === NONE ? '' : v })}>
                    <SelectTrigger id="reward-badge"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value={NONE}>None</SelectItem>
                      {badges.map((b) => <SelectItem key={b.id} value={b.id}>{b.name}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </div>
              )}
              {form.type === 'level' && (
                <div className="space-y-2">
                  <Label htmlFor="reward-level">Level granted</Label>
                  <Select value={form.level_reward_id || NONE} onValueChange={(v) => setForm({ ...form, level_reward_id: v === NONE ? '' : v })}>
                    <SelectTrigger id="reward-level"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value={NONE}>None</SelectItem>
                      {levels.map((l) => <SelectItem key={l.id} value={l.id}>{l.name || `Level ${l.level_number}`}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </div>
              )}
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="reward-stock">Stock</Label>
                  <Input id="reward-stock" type="number" min="1" placeholder="Unlimited" value={form.max_redemptions} onChange={(e) => setForm({ ...form, max_redemptions: e.target.value })} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reward-per-player">Per player</Label>
                  <Input id="reward-per-player" type="number" min="1" placeholder="Unlimited" value={form.max_per_player} onChange={(e) => setForm({ ...form, max_per_player: e.target.value })} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reward-ttl">Claim valid (days)</Label>
                  <Input id="reward-ttl" type="number" min="1" placeholder="Forever" value={form.claim_ttl_days} onChange={(e) => setForm({ ...form, claim_ttl_days: e.target.value })} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reward-level-req">Min level</Label>
                  <Input id="reward-level-req" type="number" min="1" placeholder="None" value={form.level_requirement} onChange={(e) => setForm({ ...form, level_requirement: e.target.value })} />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="reward-start">Available from</Label>
                  <Input id="reward-start" type="date" value={form.start_at} onChange={(e) => setForm({ ...form, start_at: e.target.value })} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="reward-end">Available until</Label>
                  <Input id="reward-end" type="date" value={form.end_at} onChange={(e) => setForm({ ...form, end_at: e.target.value })} />
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Switch id="reward-active" checked={form.is_active} onCheckedChange={(c) => setForm({ ...form, is_active: c })} />
                <Label htmlFor="reward-active">Active</Label>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>Cancel</Button>
              <Button type="submit" variant="glow" disabled={isSaving}>
                {isSaving && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                {editingReward ? 'Save Changes' : 'Add Reward'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Tracked claim (latest claim made from this page) */}
      {trackedClaimId && trackedClaim.data && (
        <Card className={cn(trackedPending && 'border-amber-500/40')}>
          <CardContent className="p-4 flex flex-col sm:flex-row sm:items-center gap-4 justify-between">
            <div className="flex items-center gap-3">
              <Ticket className="w-5 h-5 text-muted-foreground" />
              <div>
                <p className="font-medium">
                  Latest claim: {rewardName(trackedClaim.data.reward_id) ?? trackedClaim.data.reward_slug}
                </p>
                <p className="text-sm text-muted-foreground">
                  {trackedPending
                    ? 'Points are being debited; this updates automatically.'
                    : trackedClaim.data.status === 'rejected'
                    ? `Rejected${trackedClaim.data.reject_reason ? `: ${trackedClaim.data.reject_reason}` : ''}`
                    : trackedClaim.data.code
                    ? `Code: ${trackedClaim.data.code}`
                    : `${trackedClaim.data.points_cost.toLocaleString()} points`}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <ClaimStatusBadge status={trackedClaim.data.status} />
              <Button size="sm" variant="outline" onClick={() => trackedClaim.refetch()} disabled={trackedClaim.isFetching}>
                <RefreshCw className={cn('w-4 h-4 mr-1', trackedClaim.isFetching && 'animate-spin')} /> Refresh
              </Button>
              {claimActions(trackedClaim.data)}
              <Button size="sm" variant="ghost" onClick={() => setTrackedClaimId('')}>Dismiss</Button>
            </div>
          </CardContent>
        </Card>
      )}

      <Tabs defaultValue="catalog" className="space-y-6">
        <TabsList>
          <TabsTrigger value="catalog">Catalog</TabsTrigger>
          <TabsTrigger value="claims">Player Claims</TabsTrigger>
        </TabsList>

        <TabsContent value="catalog" className="space-y-4">
          {/* Filters (server-side: ?status&type) */}
          <div className="flex flex-wrap gap-3">
            <Select value={statusFilter} onValueChange={setFilter(setStatusFilter)}>
              <SelectTrigger className="w-40" aria-label="Filter by status"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All statuses</SelectItem>
                {(Object.keys(statusLabels) as RewardStatus[]).map((s) => (
                  <SelectItem key={s} value={s}>{statusLabels[s]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={typeFilter} onValueChange={setFilter(setTypeFilter)}>
              <SelectTrigger className="w-40" aria-label="Filter by type"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All types</SelectItem>
                {(Object.keys(typeLabels) as RewardType[]).map((t) => (
                  <SelectItem key={t} value={t}>{typeLabels[t].label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {isLoading ? (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {[...Array(6)].map((_, i) => (
                <Card key={i}><CardContent className="p-6 space-y-3">
                  <Skeleton className="w-16 h-16 rounded-xl" />
                  <Skeleton className="h-5 w-32" />
                  <Skeleton className="h-4 w-full" />
                </CardContent></Card>
              ))}
            </div>
          ) : error ? (
            <Card className="p-8 text-center border-destructive">
              <p className="text-destructive">Failed to load rewards: {error.message}</p>
            </Card>
          ) : rewards.length === 0 ? (
            <Card className="p-8 text-center">
              <Gift className="w-12 h-12 mx-auto text-muted-foreground mb-3" />
              <p className="font-medium">No rewards found</p>
              <p className="text-sm text-muted-foreground">
                {hasFilters ? 'Try different filters.' : 'Add your first reward to the catalog.'}
              </p>
            </Card>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              {rewards.map((reward, index) => {
                const stockLeft = reward.max_redemptions !== null ? reward.max_redemptions - reward.stock_used : null;
                const claimable = reward.is_active && reward.status === 'active';
                return (
                  <Card key={reward.id} className="stat-card group" style={{ animationDelay: `${index * 50}ms` }}>
                    <CardContent className="p-6">
                      <div className="flex items-start justify-between mb-4">
                        <div className="w-16 h-16 rounded-xl bg-secondary flex items-center justify-center text-3xl">
                          {typeLabels[reward.type].icon}
                        </div>
                        <div className="flex items-center gap-1">
                          <Badge variant="outline" className="capitalize">{reward.status}</Badge>
                          <ItemActionsMenu
                            itemName={reward.name}
                            onEdit={() => openEdit(reward)}
                            onDelete={() => handleDelete(reward)}
                            actions={[{ label: 'Claim for player', icon: ShoppingCart, onClick: () => setClaimingReward(reward), disabled: !claimable }]}
                            showInGroup
                          />
                        </div>
                      </div>
                      <h3 className="font-semibold text-lg mb-1">{reward.name}</h3>
                      <p className="text-sm text-muted-foreground mb-4">{reward.description || typeLabels[reward.type].label}</p>
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge variant="secondary" className="font-mono">
                            {reward.points_cost > 0 ? `${reward.points_cost.toLocaleString()} pts` : 'Free'}
                          </Badge>
                          {reward.value !== null && (
                            <Badge variant="outline">
                              {reward.value_type === 'percentage' ? `${Number(reward.value)}% off` : reward.value}
                            </Badge>
                          )}
                          {stockLeft !== null && (
                            <Badge variant="outline" className="border-amber-500/50 text-amber-500">
                              {stockLeft} left
                            </Badge>
                          )}
                          {!reward.is_active && <Badge variant="outline" className="text-muted-foreground">Inactive</Badge>}
                        </div>
                        <span className="text-sm text-muted-foreground">{reward.stock_used} claimed</span>
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
            nextCursor={rewardsData?.next_cursor}
            onPrevious={pager.previous}
            onNext={pager.next}
            isFetching={isFetching}
          />
        </TabsContent>

        <TabsContent value="claims">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Clock className="w-5 h-5" />
                Player Claims
              </CardTitle>
              <CardDescription>Pick a player to see, redeem or cancel their reward claims.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="max-w-md">
                <PlayerPicker
                  id="claims-player"
                  value={claimsPlayerId}
                  onChange={(id) => { setClaimsPlayerId(id); claimsPager.reset(); }}
                />
              </div>
              {!claimsPlayerId ? null : playerClaims.isLoading ? (
                <p className="text-sm text-muted-foreground">Loading claims...</p>
              ) : playerClaims.error ? (
                <p className="text-sm text-destructive">{playerClaims.error.message}</p>
              ) : (playerClaims.data?.data ?? []).length === 0 ? (
                <p className="text-sm text-muted-foreground">This player has no claims.</p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-border">
                        <th className="text-left p-3 text-sm font-medium text-muted-foreground">Reward</th>
                        <th className="text-left p-3 text-sm font-medium text-muted-foreground">Cost</th>
                        <th className="text-left p-3 text-sm font-medium text-muted-foreground">Status</th>
                        <th className="text-left p-3 text-sm font-medium text-muted-foreground">Code</th>
                        <th className="text-left p-3 text-sm font-medium text-muted-foreground">Claimed</th>
                        <th className="text-right p-3 text-sm font-medium text-muted-foreground">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {playerClaims.data!.data.map((claim) => (
                        <tr key={claim.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors">
                          <td className="p-3 font-medium">{rewardName(claim.reward_id) ?? claim.reward_slug}</td>
                          <td className="p-3">
                            <Badge variant="secondary" className="font-mono">{claim.points_cost.toLocaleString()} pts</Badge>
                          </td>
                          <td className="p-3"><ClaimStatusBadge status={claim.status} /></td>
                          <td className="p-3"><code className="text-sm">{claim.code ?? '-'}</code></td>
                          <td className="p-3 text-sm text-muted-foreground">{formatDateTime(claim.claimed_at ?? claim.created_at)}</td>
                          <td className="p-3">{claimActions(claim)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              {claimsPlayerId && (
                <div className="flex items-center justify-between">
                  <Button size="sm" variant="outline" onClick={() => playerClaims.refetch()} disabled={playerClaims.isFetching}>
                    <RefreshCw className={cn('w-4 h-4 mr-1', playerClaims.isFetching && 'animate-spin')} /> Refresh
                  </Button>
                  <CursorPager
                    page={claimsPager.page}
                    hasPrevious={claimsPager.hasPrevious}
                    nextCursor={playerClaims.data?.next_cursor}
                    onPrevious={claimsPager.previous}
                    onNext={claimsPager.next}
                    isFetching={playerClaims.isFetching}
                  />
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <PlayerActionDialog
        open={!!claimingReward}
        onOpenChange={(open) => !open && setClaimingReward(null)}
        title={`Claim "${claimingReward?.name ?? ''}"`}
        description={
          claimingReward && claimingReward.points_cost > 0
            ? `Costs ${claimingReward.points_cost.toLocaleString()} points. The claim stays pending until the points are debited.`
            : 'Free reward: the claim is granted immediately.'
        }
        submitLabel="Claim Reward"
        isPending={claimMutation.isPending}
        onSubmit={handleClaim}
      />
    </div>
  );
}

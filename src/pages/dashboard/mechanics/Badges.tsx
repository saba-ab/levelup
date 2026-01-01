import React, { useState, useMemo } from 'react';
import { Plus, Search, Award, Sparkles, Loader2 } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
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
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { useToast } from '@/hooks/use-toast';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import {
  useBadgesQuery,
  useCreateBadgeMutation,
  useUpdateBadgeMutation,
  useDeleteBadgeMutation,
} from '@/services/queries/mechanics';
import type { Badge, CreateBadgeData, UpdateBadgeData, BadgeTier, BadgeCategory } from '@/services/api/types';

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

interface BadgeFormState {
  name: string;
  description: string;
  image_url: string;
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
  image_url: '',
  tier: 'bronze',
  category: 'achievement',
  points_value: 0,
  is_stackable: false,
  max_awards: null,
  is_active: true,
  is_secret: false,
};

export default function Badges() {
  const [searchQuery, setSearchQuery] = useState('');
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingBadge, setEditingBadge] = useState<Badge | null>(null);
  const [deletingBadge, setDeletingBadge] = useState<Badge | null>(null);
  const [formState, setFormState] = useState<BadgeFormState>(defaultFormState);
  const { toast } = useToast();

  // Queries and mutations
  const filters = useMemo(() => ({ search: searchQuery || undefined }), [searchQuery]);
  const { data: badgesData, isLoading, error } = useBadgesQuery(filters);
  const createMutation = useCreateBadgeMutation();
  const updateMutation = useUpdateBadgeMutation();
  const deleteMutation = useDeleteBadgeMutation();

  const badges = badgesData?.data || [];

  const handleOpenDialog = (badge?: Badge) => {
    if (badge) {
      setEditingBadge(badge);
      setFormState({
        name: badge.name,
        description: badge.description || '',
        image_url: badge.image_url || '',
        tier: badge.tier,
        category: badge.category,
        points_value: badge.points_value,
        is_stackable: badge.is_stackable,
        max_awards: badge.max_awards ?? null,
        is_active: badge.is_active,
        is_secret: badge.is_secret,
      });
    } else {
      setEditingBadge(null);
      setFormState(defaultFormState);
    }
    setIsDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setIsDialogOpen(false);
    setEditingBadge(null);
    setFormState(defaultFormState);
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    
    try {
      if (editingBadge) {
        const updateData: UpdateBadgeData = {
          name: formState.name,
          description: formState.description || undefined,
          image_url: formState.image_url || undefined,
          tier: formState.tier,
          category: formState.category,
          points_value: formState.points_value,
          is_stackable: formState.is_stackable,
          max_awards: formState.max_awards ?? undefined,
          is_active: formState.is_active,
          is_secret: formState.is_secret,
        };
        await updateMutation.mutateAsync({ badgeId: editingBadge.id, data: updateData });
        toast({
          title: 'Badge updated',
          description: 'Badge has been updated successfully.',
        });
      } else {
        const createData: CreateBadgeData = {
          name: formState.name,
          description: formState.description || undefined,
          image_url: formState.image_url || undefined,
          tier: formState.tier,
          category: formState.category,
          points_value: formState.points_value,
          is_stackable: formState.is_stackable,
          max_awards: formState.max_awards ?? undefined,
          is_active: formState.is_active,
          is_secret: formState.is_secret,
        };
        await createMutation.mutateAsync(createData);
        toast({
          title: 'Badge created',
          description: 'New badge has been created successfully.',
        });
      }
      handleCloseDialog();
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Something went wrong',
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async () => {
    if (!deletingBadge) return;
    
    try {
      await deleteMutation.mutateAsync(deletingBadge.id);
      toast({
        title: 'Badge deleted',
        description: 'Badge has been removed successfully.',
      });
      setDeletingBadge(null);
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to delete badge',
        variant: 'destructive',
      });
    }
  };

  const handleAIGenerate = async (prompt: string): Promise<string> => {
    await new Promise(resolve => setTimeout(resolve, 1500));
    return `Generated Badge Idea:\n\n"${prompt}"\n\nName: Achievement Unlocked\nDescription: Awarded to users who demonstrate exceptional dedication.\nTier: Gold\nCategory: Achievement`;
  };

  const isSubmitting = createMutation.isPending || updateMutation.isPending;

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Badges</h1>
          <p className="text-muted-foreground mt-1">Create and manage achievement badges for your users.</p>
        </div>
        <div className="flex gap-2">
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
            onGenerate={handleAIGenerate}
          />
          <Dialog open={isDialogOpen} onOpenChange={(open) => { if (!open) handleCloseDialog(); else setIsDialogOpen(true); }}>
            <DialogTrigger asChild>
              <Button variant="glow" onClick={() => handleOpenDialog()}>
                <Plus className="w-4 h-4" />
                Create Badge
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
              <form onSubmit={handleSubmit}>
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
                      onChange={(e) => setFormState(prev => ({ ...prev, description: e.target.value }))}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="image_url">Image URL</Label>
                    <Input
                      id="image_url"
                      placeholder="https://example.com/badge.png"
                      value={formState.image_url}
                      onChange={(e) => setFormState(prev => ({ ...prev, image_url: e.target.value }))}
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
                          <SelectItem value="bronze">{tierIcons.bronze} Bronze</SelectItem>
                          <SelectItem value="silver">{tierIcons.silver} Silver</SelectItem>
                          <SelectItem value="gold">{tierIcons.gold} Gold</SelectItem>
                          <SelectItem value="platinum">{tierIcons.platinum} Platinum</SelectItem>
                          <SelectItem value="diamond">{tierIcons.diamond} Diamond</SelectItem>
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
                        onChange={(e) => setFormState(prev => ({ ...prev, points_value: parseInt(e.target.value) || 0 }))}
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
                          max_awards: e.target.value ? parseInt(e.target.value) : null 
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
        </div>
      </div>

      {/* Search */}
      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <Input
          placeholder="Search badges..."
          className="pl-9"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
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
      {!isLoading && !error && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {badges.map((badge, index) => (
            <Card
              key={badge.id}
              className="stat-card group"
              style={{ animationDelay: `${index * 50}ms` }}
            >
              <CardContent className="p-6 text-center relative">
                <div className="absolute top-2 right-2">
                  <ItemActionsMenu
                    itemName={badge.name}
                    onEdit={() => handleOpenDialog(badge)}
                    onDelete={() => setDeletingBadge(badge)}
                    showInGroup
                  />
                </div>
                <div
                  className="w-20 h-20 rounded-full mx-auto mb-4 flex items-center justify-center text-4xl transition-transform group-hover:scale-110 bg-secondary"
                >
                  {badge.image_url ? (
                    <img src={badge.image_url} alt={badge.name} className="w-full h-full rounded-full object-cover" />
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
                </div>
                {!badge.is_active && (
                  <BadgeUI variant="outline" className="mt-2 bg-muted text-muted-foreground">
                    Inactive
                  </BadgeUI>
                )}
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
              <p className="text-sm text-muted-foreground">Create your first badge to reward your users.</p>
            </div>
          </div>
        </Card>
      )}

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={!!deletingBadge} onOpenChange={(open) => !open && setDeletingBadge(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Badge</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete "{deletingBadge?.name}"? This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteMutation.isPending ? (
                <Loader2 className="w-4 h-4 mr-2 animate-spin" />
              ) : null}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
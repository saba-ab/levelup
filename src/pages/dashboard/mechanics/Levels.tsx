import React, { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { TrendingUp, Sparkles, Plus, Award, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Textarea } from '@/components/ui/textarea';
import { useToast } from '@/hooks/use-toast';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useLevelsQuery,
  useCreateLevelMutation,
  useUpdateLevelMutation,
  useDeleteLevelMutation,
} from '@/services/queries/mechanics';
import { Level, CreateLevelData, UpdateLevelData } from '@/services/api/types';

// Color mappings based on level number ranges
const getLevelColor = (levelNumber: number) => {
  if (levelNumber >= 50) return { bg: 'bg-cyan-500', text: 'text-cyan-500', border: 'border-cyan-500' };
  if (levelNumber >= 30) return { bg: 'bg-slate-300', text: 'text-slate-300', border: 'border-slate-300' };
  if (levelNumber >= 20) return { bg: 'bg-yellow-500', text: 'text-yellow-500', border: 'border-yellow-500' };
  if (levelNumber >= 10) return { bg: 'bg-gray-400', text: 'text-gray-400', border: 'border-gray-400' };
  return { bg: 'bg-amber-700', text: 'text-amber-700', border: 'border-amber-700' };
};

const getTierName = (levelNumber: number) => {
  if (levelNumber >= 50) return 'Diamond';
  if (levelNumber >= 30) return 'Platinum';
  if (levelNumber >= 20) return 'Gold';
  if (levelNumber >= 10) return 'Silver';
  return 'Bronze';
};

const initialFormState = {
  number: 1,
  name: '',
  description: '',
  xp_required: 0,
  points_reward: 0,
  icon_url: '',
  color: '',
};

export default function Levels() {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingLevel, setEditingLevel] = useState<Level | null>(null);
  const [formState, setFormState] = useState(initialFormState);
  const { toast } = useToast();

  // API hooks
  const { data: levelsData, isLoading, error } = useLevelsQuery();
  const createMutation = useCreateLevelMutation();
  const updateMutation = useUpdateLevelMutation();
  const deleteMutation = useDeleteLevelMutation();

  const levels = levelsData ?? [];

  const handleOpenDialog = (level?: Level) => {
    if (level) {
      setEditingLevel(level);
      setFormState({
        number: level.number,
        name: level.name || '',
        description: level.description || '',
        xp_required: level.xp_required,
        points_reward: level.points_reward,
        icon_url: level.icon_url || '',
        color: level.color || '',
      });
    } else {
      setEditingLevel(null);
      setFormState({
        ...initialFormState,
        number: levels.length > 0 ? Math.max(...levels.map(l => l.number)) + 1 : 1,
      });
    }
    setIsDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setIsDialogOpen(false);
    setEditingLevel(null);
    setFormState(initialFormState);
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    try {
      if (editingLevel) {
        const updateData: UpdateLevelData = {
          number: formState.number,
          name: formState.name || undefined,
          description: formState.description || undefined,
          xp_required: formState.xp_required,
          points_reward: formState.points_reward,
          icon_url: formState.icon_url || undefined,
          color: formState.color || undefined,
        };
        await updateMutation.mutateAsync({ levelId: editingLevel.id, data: updateData });
        toast({
          title: 'Level updated',
          description: 'Level has been updated successfully.',
        });
      } else {
        const createData: CreateLevelData = {
          number: formState.number,
          name: formState.name || undefined,
          description: formState.description || undefined,
          xp_required: formState.xp_required,
          points_reward: formState.points_reward,
          icon_url: formState.icon_url || undefined,
          color: formState.color || undefined,
        };
        await createMutation.mutateAsync(createData);
        toast({
          title: 'Level created',
          description: 'New level has been added successfully.',
        });
      }
      handleCloseDialog();
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'An error occurred',
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async (level: Level) => {
    try {
      await deleteMutation.mutateAsync(level.id);
      toast({
        title: 'Level deleted',
        description: 'Level has been removed successfully.',
      });
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to delete level',
        variant: 'destructive',
      });
    }
  };

  const handleAIGenerate = async (prompt: string): Promise<string> => {
    await new Promise(resolve => setTimeout(resolve, 1500));
    return `Generated Level Description:\n\n"${prompt}"\n\nThis tier rewards dedicated players who have shown consistent engagement. Members enjoy exclusive perks including early access to new features, special badges, and priority support.`;
  };

  const isMutating = createMutation.isPending || updateMutation.isPending;

  if (error) {
    return (
      <div className="p-6 text-center">
        <p className="text-destructive">Failed to load levels: {error.message}</p>
      </div>
    );
  }
console.log('levels', levelsData);
  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Levels & Tiers</h1>
          <p className="text-muted-foreground mt-1">Define progression levels for your players.</p>
        </div>
        <div className="flex gap-2">
          <AIGenerateDialog
            trigger={
              <Button variant="outline" className="gap-2">
                <Sparkles className="w-4 h-4" />
                Generate with AI
              </Button>
            }
            title="Generate Level Content"
            placeholder="E.g., Create a description for a Diamond tier level that makes players feel elite..."
            context="Generate level descriptions, tier benefits, or progression milestones"
            onGenerate={handleAIGenerate}
          />
          <Dialog open={isDialogOpen} onOpenChange={(open) => { if (!open) handleCloseDialog(); else setIsDialogOpen(true); }}>
            <DialogTrigger asChild>
              <Button variant="glow" onClick={() => handleOpenDialog()}>
                <Plus className="w-4 h-4" />
                Create Level
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
              <form onSubmit={handleSubmit}>
                {/* Live Preview */}
                <div className="mb-6 p-4 rounded-lg bg-secondary/50 border border-border">
                  <p className="text-xs text-muted-foreground uppercase tracking-wide mb-3">Live Preview</p>
                  <div className="flex items-center gap-4">
                    <div
                      className={cn(
                        "w-16 h-16 rounded-full flex items-center justify-center shrink-0 border-2 transition-all",
                        getLevelColor(formState.number).bg
                      )}
                    >
                      {formState.icon_url ? (
                        <img
                          src={formState.icon_url}
                          alt="Level icon"
                          className="w-full h-full rounded-full object-cover"
                          onError={(e) => {
                            e.currentTarget.style.display = 'none';
                            e.currentTarget.nextElementSibling?.classList.remove('hidden');
                          }}
                        />
                      ) : null}
                      <TrendingUp className={cn("w-8 h-8 text-white", formState.icon_url ? 'hidden' : '')} />
                    </div>
                    <div className="flex-1 min-w-0">
                      <h4 className="font-semibold text-lg truncate">
                        {formState.name || `Level ${formState.number}`}
                      </h4>
                      <p className="text-sm text-muted-foreground line-clamp-2">
                        {formState.description || 'Level description will appear here...'}
                      </p>
                      <div className="flex flex-wrap gap-1.5 mt-2">
                        <Badge variant="outline" className={cn("text-xs", getLevelColor(formState.number).text, getLevelColor(formState.number).border)}>
                          {getTierName(formState.number)} Tier
                        </Badge>
                        <Badge variant="outline" className="text-xs bg-secondary">
                          {formState.xp_required.toLocaleString()} XP Required
                        </Badge>
                        {formState.points_reward > 0 && (
                          <Badge variant="outline" className="text-xs bg-secondary">
                            <Award className="w-3 h-3 mr-1" />
                            {formState.points_reward} pts reward
                          </Badge>
                        )}
                      </div>
                    </div>
                  </div>
                </div>

                <DialogHeader>
                  <DialogTitle>{editingLevel ? 'Edit Level' : 'Create New Level'}</DialogTitle>
                  <DialogDescription>
                    {editingLevel ? 'Update the level details.' : 'Define a new progression level for your players.'}
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Level Number *</label>
                      <Input
                        type="number"
                        min="1"
                        value={formState.number}
                        onChange={(e) => setFormState(prev => ({ ...prev, number: parseInt(e.target.value) || 1 }))}
                        required
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Level Name</label>
                      <Input
                        placeholder="e.g., Elite Champion"
                        value={formState.name}
                        onChange={(e) => setFormState(prev => ({ ...prev, name: e.target.value }))}
                      />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">XP Required *</label>
                      <Input
                        type="number"
                        min="0"
                        placeholder="5000"
                        value={formState.xp_required}
                        onChange={(e) => setFormState(prev => ({ ...prev, xp_required: parseInt(e.target.value) || 0 }))}
                        required
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Points Reward</label>
                      <Input
                        type="number"
                        min="0"
                        placeholder="100"
                        value={formState.points_reward}
                        onChange={(e) => setFormState(prev => ({ ...prev, points_reward: parseInt(e.target.value) || 0 }))}
                      />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Icon URL</label>
                      <Input
                        type="url"
                        placeholder="https://example.com/icon.png"
                        value={formState.icon_url}
                        onChange={(e) => setFormState(prev => ({ ...prev, icon_url: e.target.value }))}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Color</label>
                      <Input
                        placeholder="#10b981"
                        value={formState.color}
                        onChange={(e) => setFormState(prev => ({ ...prev, color: e.target.value }))}
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Description</label>
                    <Textarea
                      placeholder="What benefits does this level unlock?"
                      value={formState.description}
                      onChange={(e) => setFormState(prev => ({ ...prev, description: e.target.value }))}
                      rows={3}
                    />
                  </div>
                </div>
                <DialogFooter>
                  <Button type="button" variant="outline" onClick={handleCloseDialog} disabled={isMutating}>
                    Cancel
                  </Button>
                  <Button type="submit" variant="glow" disabled={isMutating}>
                    {isMutating && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                    {editingLevel ? 'Save Changes' : 'Create Level'}
                  </Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        </div>
      </div>

      {/* Level Progression Visualization */}
      {isLoading ? (
        <Card>
          <CardHeader>
            <CardTitle>Level Progression</CardTitle>
          </CardHeader>
          <CardContent>
            <Skeleton className="h-3 w-full mb-4" />
            <div className="flex justify-between">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="text-center">
                  <Skeleton className="w-12 h-12 rounded-full mx-auto mb-2" />
                  <Skeleton className="h-4 w-16 mx-auto mb-1" />
                  <Skeleton className="h-3 w-12 mx-auto" />
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      ) : levels.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>Level Progression</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="relative">
              {/* Progress Track */}
              <div className="h-3 bg-secondary rounded-full overflow-hidden">
                <div className="h-full flex">
                  {levels.slice(0, 10).map((level) => (
                    <div
                      key={level.id}
                      className={cn("h-full flex-1", getLevelColor(level.number).bg)}
                    />
                  ))}
                </div>
              </div>

              {/* Level Markers */}
              <div className="flex justify-between mt-4 overflow-x-auto pb-2">
                {levels.slice(0, 10).map((level) => (
                  <div key={level.id} className="text-center min-w-[80px]">
                    <div
                      className={cn(
                        "w-12 h-12 rounded-full mx-auto mb-2 flex items-center justify-center",
                        getLevelColor(level.number).bg
                      )}
                    >
                      <TrendingUp className="w-6 h-6 text-white" />
                    </div>
                    <p className="font-semibold text-sm">{level.name || `Level ${level.number}`}</p>
                    <p className="text-xs text-muted-foreground">{level.xp_required.toLocaleString()} XP</p>
                  </div>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {/* Levels Table */}
      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-6 space-y-4">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="flex items-center gap-4">
                  <Skeleton className="w-10 h-10 rounded-lg" />
                  <Skeleton className="h-4 w-32" />
                  <Skeleton className="h-4 w-20 ml-auto" />
                </div>
              ))}
            </div>
          ) : levels.length === 0 ? (
            <div className="p-12 text-center">
              <TrendingUp className="w-12 h-12 mx-auto text-muted-foreground mb-4" />
              <h3 className="text-lg font-semibold mb-2">No levels yet</h3>
              <p className="text-muted-foreground mb-4">Create your first progression level to get started.</p>
              <Button variant="glow" onClick={() => handleOpenDialog()}>
                <Plus className="w-4 h-4 mr-2" />
                Create Level
              </Button>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left p-4 text-sm font-medium text-muted-foreground">Level</th>
                    <th className="text-left p-4 text-sm font-medium text-muted-foreground">Tier</th>
                    <th className="text-left p-4 text-sm font-medium text-muted-foreground">XP Required</th>
                    <th className="text-left p-4 text-sm font-medium text-muted-foreground">Points Reward</th>
                    <th className="text-right p-4 text-sm font-medium text-muted-foreground">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {levels.map((level) => {
                    const colors = getLevelColor(level.number);
                    return (
                      <tr key={level.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors group">
                        <td className="p-4">
                          <div className="flex items-center gap-3">
                            <div
                              className={cn(
                                "w-10 h-10 rounded-lg flex items-center justify-center",
                                colors.bg
                              )}
                            >
                              <TrendingUp className="w-5 h-5 text-white" />
                            </div>
                            <div>
                              <span className="font-medium">{level.name || `Level ${level.number}`}</span>
                              <p className="text-xs text-muted-foreground">#{level.number}</p>
                            </div>
                          </div>
                        </td>
                        <td className="p-4">
                          <Badge
                            variant="outline"
                            className={cn("capitalize", colors.text, colors.border)}
                          >
                            {getTierName(level.number)}
                          </Badge>
                        </td>
                        <td className="p-4 font-mono">
                          {level.xp_required.toLocaleString()} XP
                        </td>
                        <td className="p-4">
                          <div className="flex items-center gap-2">
                            <Award className="w-4 h-4 text-muted-foreground" />
                            <span>{level.points_reward.toLocaleString()}</span>
                          </div>
                        </td>
                        <td className="p-4 text-right">
                          <ItemActionsMenu
                            itemName={level.name || `Level ${level.number}`}
                            onEdit={() => handleOpenDialog(level)}
                            onDelete={() => handleDelete(level)}
                            showInGroup
                          />
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

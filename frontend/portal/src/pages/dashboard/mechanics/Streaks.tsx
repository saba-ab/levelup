import React, { useState, useEffect } from 'react';
import { Plus, Flame, Clock, Gift, Users, Zap, Sparkles, Search, X } from 'lucide-react';
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
  DialogTrigger,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import {
  useStreaksQuery,
  useCreateStreakMutation,
  useUpdateStreakMutation,
  useDeleteStreakMutation,
} from '@/services/queries/mechanics';
import { useEventsQuery } from '@/services/queries/events';
import type { Streak, CreateStreakData } from '@/services/api/types';

interface StreakFormData {
  name: string;
  description: string;
  type: 'daily' | 'weekly' | 'monthly';
  activity_key: string;
  points_per_day: number;
  bonus_points: number;
  bonus_milestones: number[];
  max_streak_days: number | undefined;
  is_active: boolean;
}

const initialFormData: StreakFormData = {
  name: '',
  description: '',
  type: 'daily',
  activity_key: '',
  points_per_day: 0,
  bonus_points: 0,
  bonus_milestones: [],
  max_streak_days: undefined,
  is_active: true,
};

export default function Streaks() {
  const [searchQuery, setSearchQuery] = useState('');
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [editingStreak, setEditingStreak] = useState<Streak | null>(null);
  const [formData, setFormData] = useState<StreakFormData>(initialFormData);
  const [milestoneInput, setMilestoneInput] = useState('');
  const { toast } = useToast();

  const { data: streaksData, isLoading } = useStreaksQuery();
  const { data: eventsData } = useEventsQuery();
  const createMutation = useCreateStreakMutation();
  const updateMutation = useUpdateStreakMutation();
  const deleteMutation = useDeleteStreakMutation();

  const streaks = streaksData?.data || [];
  const events = eventsData || [];

  const filteredStreaks = streaks.filter((streak: Streak) =>
    streak.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    streak.description?.toLowerCase().includes(searchQuery.toLowerCase())
  );

  // Reset form when dialog closes
  useEffect(() => {
    if (!isDialogOpen && !isEditDialogOpen) {
      setFormData(initialFormData);
      setMilestoneInput('');
      setEditingStreak(null);
    }
  }, [isDialogOpen, isEditDialogOpen]);

  // Populate form when editing
  useEffect(() => {
    if (editingStreak) {
      setFormData({
        name: editingStreak.name,
        description: editingStreak.description || '',
        type: editingStreak.type,
        activity_key: editingStreak.activity_key,
        points_per_day: editingStreak.points_per_day,
        bonus_points: editingStreak.bonus_points,
        bonus_milestones: editingStreak.bonus_milestones || [],
        max_streak_days: editingStreak.max_streak_days,
        is_active: editingStreak.is_active,
      });
    }
  }, [editingStreak]);

  const addMilestone = () => {
    const milestone = parseInt(milestoneInput);
    if (milestone && milestone > 0 && !formData.bonus_milestones.includes(milestone)) {
      setFormData({
        ...formData,
        bonus_milestones: [...formData.bonus_milestones, milestone].sort((a, b) => a - b),
      });
      setMilestoneInput('');
    }
  };

  const removeMilestone = (milestone: number) => {
    setFormData({
      ...formData,
      bonus_milestones: formData.bonus_milestones.filter((m) => m !== milestone),
    });
  };

  const transformFormDataToAPI = (data: StreakFormData): CreateStreakData => {
    return {
      name: data.name,
      description: data.description || undefined,
      type: data.type,
      activity_key: data.activity_key,
      points_per_day: data.points_per_day || undefined,
      bonus_points: data.bonus_points || undefined,
      bonus_milestones: data.bonus_milestones.length > 0 ? data.bonus_milestones : undefined,
      max_streak_days: data.max_streak_days || undefined,
      is_active: data.is_active,
    };
  };

  const handleCreate = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    // Validation
    if (!formData.name.trim()) {
      toast({
        title: 'Validation Error',
        description: 'Streak name is required',
        variant: 'destructive',
      });
      return;
    }

    if (!formData.activity_key.trim()) {
      toast({
        title: 'Validation Error',
        description: 'Activity key is required',
        variant: 'destructive',
      });
      return;
    }

    try {
      const apiData = transformFormDataToAPI(formData);
      await createMutation.mutateAsync(apiData);
    toast({
      title: 'Streak created',
        description: 'New streak has been created successfully.',
    });
    setIsDialogOpen(false);
      setFormData(initialFormData);
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to create streak',
        variant: 'destructive',
      });
    }
  };

  const handleEdit = (streak: Streak) => {
    setEditingStreak(streak);
    setIsEditDialogOpen(true);
  };

  const handleUpdate = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!editingStreak) return;

    // Validation
    if (!formData.name.trim()) {
      toast({
        title: 'Validation Error',
        description: 'Streak name is required',
        variant: 'destructive',
      });
      return;
    }

    if (!formData.activity_key.trim()) {
      toast({
        title: 'Validation Error',
        description: 'Activity key is required',
        variant: 'destructive',
      });
      return;
    }

    try {
      const apiData = transformFormDataToAPI(formData);
      await updateMutation.mutateAsync({
        streakId: editingStreak.id,
        data: apiData,
      });
      toast({
        title: 'Streak updated',
        description: `${formData.name} has been updated successfully.`,
      });
      setIsEditDialogOpen(false);
      setEditingStreak(null);
      setFormData(initialFormData);
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to update streak',
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async (streak: Streak) => {
    try {
      await deleteMutation.mutateAsync(streak.id);
      toast({
        title: 'Streak deleted',
        description: `${streak.name} has been deleted.`,
      });
    } catch (error) {
      toast({
        title: 'Error',
        description: 'Failed to delete streak',
        variant: 'destructive',
      });
    }
  };

  // Connect this to your MySQL backend
  const handleAIGenerate = async (prompt: string): Promise<string> => {
    await new Promise(resolve => setTimeout(resolve, 1500));
    return `Generated Streak Idea:\n\n"${prompt}"\n\nName: Consistency Champion\nDescription: Reward users for maintaining consistent engagement.\n\nMilestones:\n- 7 days: 100 XP + "Getting Started" badge\n- 30 days: 500 XP + "Dedicated" badge\n- 90 days: 2000 XP + "Streak Master" exclusive badge\n\nGrace Period: 12 hours\nInterval: Daily`;
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Streaks</h1>
          <p className="text-muted-foreground mt-1">Configure streak mechanics and milestone rewards.</p>
        </div>
        <div className="flex gap-2">
          <AIGenerateDialog
            trigger={
              <Button variant="outline" className="gap-2">
                <Sparkles className="w-4 h-4" />
                Generate with AI
              </Button>
            }
            title="Generate Streak Ideas"
            placeholder="E.g., Create a streak mechanic that encourages daily app usage with escalating rewards..."
            context="Generate streak names, milestones, intervals, and reward structures"
            onGenerate={handleAIGenerate}
          />
          <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
            <DialogTrigger asChild>
              <Button variant="glow">
                <Plus className="w-4 h-4" />
                Create Streak
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
              <form onSubmit={handleCreate}>
                <DialogHeader>
                  <DialogTitle>Create New Streak</DialogTitle>
                  <DialogDescription>Define a new streak mechanic for your users.</DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <Label htmlFor="name">Streak Name *</Label>
                    <Input
                      id="name"
                      placeholder="e.g., Daily Challenge Streak"
                      value={formData.name}
                      onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                      required
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="description">Description</Label>
                    <Textarea
                      id="description"
                      placeholder="What action maintains the streak?"
                      rows={3}
                      value={formData.description}
                      onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="type">Interval *</Label>
                      <Select
                        value={formData.type}
                        onValueChange={(value) => setFormData({ ...formData, type: value as StreakFormData['type'] })}
                      >
                        <SelectTrigger id="type"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="daily">Daily</SelectItem>
                          <SelectItem value="weekly">Weekly</SelectItem>
                          <SelectItem value="monthly">Monthly</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="activity_key">Activity Key *</Label>
                      <Select
                        value={formData.activity_key}
                        onValueChange={(value) => setFormData({ ...formData, activity_key: value })}
                      >
                        <SelectTrigger id="activity_key">
                          <SelectValue placeholder="Select event" />
                        </SelectTrigger>
                        <SelectContent>
                          {events.length === 0 ? (
                            <SelectItem value="none" disabled>No events available</SelectItem>
                          ) : (
                            events.map((event) => (
                              <SelectItem key={event.id} value={event.slug}>
                                {event.name}
                              </SelectItem>
                            ))
                          )}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="points_per_day">Points Per Day</Label>
                      <Input
                        id="points_per_day"
                        type="number"
                        min="0"
                        placeholder="10"
                        value={formData.points_per_day || ''}
                        onChange={(e) => setFormData({ ...formData, points_per_day: parseInt(e.target.value) || 0 })}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="bonus_points">Bonus Points</Label>
                      <Input
                        id="bonus_points"
                        type="number"
                        min="0"
                        placeholder="100"
                        value={formData.bonus_points || ''}
                        onChange={(e) => setFormData({ ...formData, bonus_points: parseInt(e.target.value) || 0 })}
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="max_streak_days">Max Streak Days (Optional)</Label>
                    <Input
                      id="max_streak_days"
                      type="number"
                      min="1"
                      placeholder="Unlimited"
                      value={formData.max_streak_days || ''}
                      onChange={(e) => setFormData({ ...formData, max_streak_days: e.target.value ? parseInt(e.target.value) : undefined })}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>Bonus Milestones</Label>
                    <div className="flex gap-2">
                      <Input
                        type="number"
                        min="1"
                        placeholder="Enter milestone day (e.g., 7, 30, 100)"
                        value={milestoneInput}
                        onChange={(e) => setMilestoneInput(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault();
                            addMilestone();
                          }
                        }}
                      />
                      <Button type="button" variant="outline" onClick={addMilestone}>
                        <Plus className="w-4 h-4" />
                      </Button>
                    </div>
                    {formData.bonus_milestones.length > 0 && (
                      <div className="flex flex-wrap gap-2 mt-2">
                        {formData.bonus_milestones.map((milestone) => (
                          <Badge key={milestone} variant="secondary" className="gap-1">
                            {milestone} days
                            <button
                              type="button"
                              onClick={() => removeMilestone(milestone)}
                              className="ml-1 hover:text-destructive"
                            >
                              <X className="w-3 h-3" />
                            </button>
                          </Badge>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch
                      id="is_active"
                      checked={formData.is_active}
                      onCheckedChange={(checked) => setFormData({ ...formData, is_active: checked })}
                    />
                    <Label htmlFor="is_active">Active</Label>
                  </div>
                </div>
                <DialogFooter>
                  <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>Cancel</Button>
                  <Button type="submit" variant="glow" disabled={createMutation.isPending}>
                    {createMutation.isPending ? 'Creating...' : 'Create Streak'}
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
          placeholder="Search streaks..."
          className="pl-9"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
      </div>

      {/* Stats Overview */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Card className="stat-card">
          <CardContent className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-orange-500/10 flex items-center justify-center">
                <Flame className="w-6 h-6 text-orange-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{streaks.length}</p>
                <p className="text-sm text-muted-foreground">Total Streaks</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="stat-card">
          <CardContent className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-purple-500/10 flex items-center justify-center">
                <Zap className="w-6 h-6 text-purple-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{streaks.filter((s: Streak) => s.is_active).length}</p>
                <p className="text-sm text-muted-foreground">Active Streaks</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="stat-card">
          <CardContent className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-green-500/10 flex items-center justify-center">
                <Gift className="w-6 h-6 text-green-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">
                  {streaks.reduce((sum: number, s: Streak) => sum + (s.bonus_milestones?.length || 0), 0)}
                </p>
                <p className="text-sm text-muted-foreground">Total Milestones</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

        {/* Streak Definitions */}
      <div className="space-y-4">
          <h2 className="text-xl font-semibold">Streak Definitions</h2>
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
        ) : filteredStreaks.length === 0 ? (
          <Card>
            <CardContent className="p-12 text-center">
              <Flame className="w-16 h-16 mx-auto mb-4 text-muted-foreground" />
              <h3 className="text-lg font-medium mb-2">No streaks found</h3>
              <p className="text-muted-foreground mb-4">
                {searchQuery ? 'Try adjusting your search query.' : 'Create your first streak to get started.'}
              </p>
              {!searchQuery && (
                <Button onClick={() => setIsDialogOpen(true)}>
                  <Plus className="w-4 h-4 mr-2" />
                  Create Streak
                </Button>
              )}
            </CardContent>
          </Card>
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {filteredStreaks.map((streak: Streak, index: number) => (
            <Card key={streak.id} className="stat-card" style={{ animationDelay: `${index * 100}ms` }}>
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                      <div className={cn(
                        "w-12 h-12 rounded-xl flex items-center justify-center",
                        streak.is_active ? "bg-orange-500/10" : "bg-muted"
                      )}>
                        <Flame className={cn(
                          "w-6 h-6",
                          streak.is_active ? "text-orange-500" : "text-muted-foreground"
                        )} />
                    </div>
                    <div>
                      <CardTitle className="text-lg">{streak.name}</CardTitle>
                        <CardDescription>{streak.description || 'No description'}</CardDescription>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                      <Badge variant="outline" className="capitalize">{streak.type}</Badge>
                      {streak.is_active && (
                        <Badge variant="outline" className="border-green-500/50 text-green-500 bg-green-500/10">
                          Active
                        </Badge>
                      )}
                    <ItemActionsMenu
                      itemName={streak.name}
                        onEdit={() => handleEdit(streak)}
                        onDelete={() => handleDelete(streak)}
                      showInGroup
                    />
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center gap-6 text-sm">
                    <div className="flex items-center gap-2">
                      <Zap className="w-4 h-4 text-muted-foreground" />
                      <span className="text-muted-foreground">Activity: {streak.activity_key}</span>
                    </div>
                    {streak.max_streak_days && (
                  <div className="flex items-center gap-2">
                    <Clock className="w-4 h-4 text-muted-foreground" />
                        <span className="text-muted-foreground">Max: {streak.max_streak_days} days</span>
                      </div>
                    )}
                  </div>

                  <div className="grid grid-cols-2 gap-4 text-sm">
                    <div>
                      <p className="text-muted-foreground">Points/Day</p>
                      <p className="font-medium">{streak.points_per_day}</p>
                  </div>
                    <div>
                      <p className="text-muted-foreground">Bonus Points</p>
                      <p className="font-medium">{streak.bonus_points}</p>
                  </div>
                </div>

                  {streak.bonus_milestones && streak.bonus_milestones.length > 0 && (
                <div className="space-y-2">
                  <p className="text-sm font-medium">Milestone Rewards</p>
                  <div className="flex flex-wrap gap-2">
                        {streak.bonus_milestones.map((milestone, i) => (
                      <Badge key={i} variant="secondary" className="bg-secondary/80">
                            {milestone} {streak.type === 'daily' ? 'days' : streak.type === 'weekly' ? 'weeks' : 'months'}
                      </Badge>
                    ))}
                  </div>
                </div>
                  )}
              </CardContent>
            </Card>
          ))}
        </div>
        )}
      </div>

      {/* Edit Dialog */}
      <Dialog open={isEditDialogOpen} onOpenChange={setIsEditDialogOpen}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <form onSubmit={handleUpdate}>
            <DialogHeader>
              <DialogTitle>Edit Streak</DialogTitle>
              <DialogDescription>Update streak configuration</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="edit-name">Streak Name *</Label>
                <Input
                  id="edit-name"
                  placeholder="e.g., Daily Challenge Streak"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-description">Description</Label>
                <Textarea
                  id="edit-description"
                  placeholder="What action maintains the streak?"
                  rows={3}
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="edit-type">Interval *</Label>
                  <Select
                    value={formData.type}
                    onValueChange={(value) => setFormData({ ...formData, type: value as StreakFormData['type'] })}
                  >
                    <SelectTrigger id="edit-type"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="daily">Daily</SelectItem>
                      <SelectItem value="weekly">Weekly</SelectItem>
                      <SelectItem value="monthly">Monthly</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-activity_key">Activity Key *</Label>
                  <Select
                    value={formData.activity_key}
                    onValueChange={(value) => setFormData({ ...formData, activity_key: value })}
                  >
                    <SelectTrigger id="edit-activity_key">
                      <SelectValue placeholder="Select event" />
                    </SelectTrigger>
                    <SelectContent>
                      {events.length === 0 ? (
                        <SelectItem value="none" disabled>No events available</SelectItem>
                      ) : (
                        events.map((event) => (
                          <SelectItem key={event.id} value={event.slug}>
                            {event.name}
                          </SelectItem>
                        ))
                      )}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="edit-points_per_day">Points Per Day</Label>
                  <Input
                    id="edit-points_per_day"
                    type="number"
                    min="0"
                    placeholder="10"
                    value={formData.points_per_day || ''}
                    onChange={(e) => setFormData({ ...formData, points_per_day: parseInt(e.target.value) || 0 })}
                  />
                  </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-bonus_points">Bonus Points</Label>
                  <Input
                    id="edit-bonus_points"
                    type="number"
                    min="0"
                    placeholder="100"
                    value={formData.bonus_points || ''}
                    onChange={(e) => setFormData({ ...formData, bonus_points: parseInt(e.target.value) || 0 })}
                  />
                  </div>
                  </div>
              <div className="space-y-2">
                <Label htmlFor="edit-max_streak_days">Max Streak Days (Optional)</Label>
                <Input
                  id="edit-max_streak_days"
                  type="number"
                  min="1"
                  placeholder="Unlimited"
                  value={formData.max_streak_days || ''}
                  onChange={(e) => setFormData({ ...formData, max_streak_days: e.target.value ? parseInt(e.target.value) : undefined })}
                />
                  </div>
              <div className="space-y-2">
                <Label>Bonus Milestones</Label>
                <div className="flex gap-2">
                  <Input
                    type="number"
                    min="1"
                    placeholder="Enter milestone day (e.g., 7, 30, 100)"
                    value={milestoneInput}
                    onChange={(e) => setMilestoneInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        addMilestone();
                      }
                    }}
                  />
                  <Button type="button" variant="outline" onClick={addMilestone}>
                    <Plus className="w-4 h-4" />
                  </Button>
                </div>
                {formData.bonus_milestones.length > 0 && (
                  <div className="flex flex-wrap gap-2 mt-2">
                    {formData.bonus_milestones.map((milestone) => (
                      <Badge key={milestone} variant="secondary" className="gap-1">
                        {milestone} days
                        <button
                          type="button"
                          onClick={() => removeMilestone(milestone)}
                          className="ml-1 hover:text-destructive"
                        >
                          <X className="w-3 h-3" />
                        </button>
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Switch
                  id="edit-is_active"
                  checked={formData.is_active}
                  onCheckedChange={(checked) => setFormData({ ...formData, is_active: checked })}
                />
                <Label htmlFor="edit-is_active">Active</Label>
        </div>
      </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setIsEditDialogOpen(false)}>Cancel</Button>
              <Button type="submit" variant="glow" disabled={updateMutation.isPending}>
                {updateMutation.isPending ? 'Updating...' : 'Update Streak'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

import React, { useState, useEffect } from 'react';
import { Plus, Search, Target, Calendar, Users, ChevronRight, Check, Sparkles, X, Trash2 } from 'lucide-react';
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
import { cn } from '@/lib/utils';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useMissionsQuery,
  useCreateMissionMutation,
  useUpdateMissionMutation,
  useDeleteMissionMutation,
  useBadgesQuery,
} from '@/services/queries/mechanics';
import type { Mission, CreateMissionData, MissionObjective } from '@/services/api/types';

interface ObjectiveInput {
  key: string;
  label: string;
  target: number;
}

interface MissionFormData {
  name: string;
  description: string;
  type: 'one_time' | 'daily' | 'weekly' | 'monthly' | 'recurring' | 'event';
  status: 'draft' | 'active' | 'paused' | 'completed' | 'expired' | 'cancelled';
  objectives: ObjectiveInput[];
  points_reward: number;
  xp_reward: number;
  badge_reward_id: number | undefined;
  start_date: string;
  end_date: string;
  max_completions: number | undefined;
  cooldown_hours: number | undefined;
  is_active: boolean;
  is_secret: boolean;
}

const initialFormData: MissionFormData = {
  name: '',
  description: '',
  type: 'one_time',
  status: 'draft',
  objectives: [],
  points_reward: 0,
  xp_reward: 0,
  badge_reward_id: undefined,
  start_date: '',
  end_date: '',
  max_completions: undefined,
  cooldown_hours: undefined,
  is_active: true,
  is_secret: false,
};

export default function Missions() {
  const [searchQuery, setSearchQuery] = useState('');
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [editingMission, setEditingMission] = useState<Mission | null>(null);
  const [wizardStep, setWizardStep] = useState(1);
  const [formData, setFormData] = useState<MissionFormData>(initialFormData);
  const { toast } = useToast();

  const { data: missionsData, isLoading } = useMissionsQuery();
  const { data: badgesData } = useBadgesQuery();
  const createMutation = useCreateMissionMutation();
  const updateMutation = useUpdateMissionMutation();
  const deleteMutation = useDeleteMissionMutation();

  const missions = missionsData?.data || [];
  const badges = badgesData?.data || [];

  const filteredMissions = missions.filter((mission: Mission) =>
    mission.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    mission.description?.toLowerCase().includes(searchQuery.toLowerCase())
  );

  // Reset form when dialog closes
  useEffect(() => {
    if (!isDialogOpen && !isEditDialogOpen) {
      setFormData(initialFormData);
      setWizardStep(1);
      setEditingMission(null);
    }
  }, [isDialogOpen, isEditDialogOpen]);

  // Populate form when editing
  useEffect(() => {
    if (editingMission) {
      const objectives: ObjectiveInput[] = editingMission.objectives
        ? Object.entries(editingMission.objectives).map(([key, obj]) => ({
            key,
            label: obj.label,
            target: obj.target,
          }))
        : [];

      setFormData({
        name: editingMission.name,
        description: editingMission.description || '',
        type: editingMission.type,
        status: editingMission.status,
        objectives,
        points_reward: editingMission.points_reward,
        xp_reward: editingMission.xp_reward,
        badge_reward_id: editingMission.badge_reward_id,
        start_date: editingMission.start_date || '',
        end_date: editingMission.end_date || '',
        max_completions: editingMission.max_completions,
        cooldown_hours: editingMission.cooldown_hours,
        is_active: editingMission.is_active,
        is_secret: editingMission.is_secret,
      });
    }
  }, [editingMission]);

  const addObjective = () => {
    setFormData({
      ...formData,
      objectives: [
        ...formData.objectives,
        { key: `objective_${Date.now()}`, label: '', target: 1 },
      ],
    });
  };

  const removeObjective = (index: number) => {
    setFormData({
      ...formData,
      objectives: formData.objectives.filter((_, i) => i !== index),
    });
  };

  const updateObjective = (index: number, field: 'label' | 'target', value: string | number) => {
    const updated = [...formData.objectives];
    updated[index] = { ...updated[index], [field]: value };
    setFormData({ ...formData, objectives: updated });
  };

  const transformFormDataToAPI = (data: MissionFormData): CreateMissionData => {
    const objectives: Record<string, MissionObjective> = {};
    data.objectives.forEach((obj) => {
      if (obj.label.trim()) {
        objectives[obj.key] = {
          label: obj.label,
          target: obj.target,
        };
      }
    });

    return {
      name: data.name,
      description: data.description || undefined,
      type: data.type,
      status: data.status,
      objectives: Object.keys(objectives).length > 0 ? objectives : undefined,
      points_reward: data.points_reward || undefined,
      xp_reward: data.xp_reward || undefined,
      badge_reward_id: data.badge_reward_id || undefined,
      start_date: data.start_date || undefined,
      end_date: data.end_date || undefined,
      max_completions: data.max_completions || undefined,
      cooldown_hours: data.cooldown_hours || undefined,
      is_active: data.is_active,
      is_secret: data.is_secret,
    };
  };

  const handleCreate = async () => {
    if (wizardStep < 4) {
      setWizardStep(wizardStep + 1);
      return;
    }

    // Validation
    if (!formData.name.trim()) {
      toast({
        title: 'Validation Error',
        description: 'Mission name is required',
        variant: 'destructive',
      });
      return;
    }

    try {
      const apiData = transformFormDataToAPI(formData);
      await createMutation.mutateAsync(apiData);
      toast({
        title: 'Mission created',
        description: 'New mission has been created successfully.',
      });
      setIsDialogOpen(false);
      setFormData(initialFormData);
      setWizardStep(1);
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to create mission',
        variant: 'destructive',
      });
    }
  };

  const handleEdit = (mission: Mission) => {
    setEditingMission(mission);
    setIsEditDialogOpen(true);
  };

  const handleUpdate = async () => {
    if (!editingMission) return;

    // Validation
    if (!formData.name.trim()) {
      toast({
        title: 'Validation Error',
        description: 'Mission name is required',
        variant: 'destructive',
      });
      return;
    }

    try {
      const apiData = transformFormDataToAPI(formData);
      await updateMutation.mutateAsync({
        missionId: editingMission.id,
        data: apiData,
      });
      toast({
        title: 'Mission updated',
        description: `${formData.name} has been updated successfully.`,
      });
      setIsEditDialogOpen(false);
      setEditingMission(null);
      setFormData(initialFormData);
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to update mission',
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async (mission: Mission) => {
    try {
      await deleteMutation.mutateAsync(mission.id);
      toast({
        title: 'Mission deleted',
        description: `${mission.name} has been deleted.`,
      });
    } catch (error) {
      toast({
        title: 'Error',
        description: 'Failed to delete mission',
        variant: 'destructive',
      });
    }
  };

  const resetWizard = () => {
    setIsDialogOpen(false);
    setWizardStep(1);
  };

  // Connect this to your MySQL backend
  const handleAIGenerate = async (prompt: string): Promise<string> => {
    await new Promise(resolve => setTimeout(resolve, 1500));
    return `Generated Mission Idea:\n\n"${prompt}"\n\nName: Challenge Champion\nDescription: A multi-step mission that drives engagement.\n\nObjectives:\n1. Complete your daily check-in\n2. Engage with 3 community posts\n3. Refer a friend to the platform\n\nSuggested Rewards:\n- 750 XP\n- Exclusive "Champion" badge`;
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Missions</h1>
          <p className="text-muted-foreground mt-1">Create and manage user missions and objectives.</p>
        </div>
        <div className="flex gap-2">
          <AIGenerateDialog
            trigger={
              <Button variant="outline" className="gap-2">
                <Sparkles className="w-4 h-4" />
                Generate with AI
              </Button>
            }
            title="Generate Mission Ideas"
            placeholder="E.g., Create a weekly mission that encourages social engagement..."
            context="Generate mission names, objectives, and reward structures"
            onGenerate={handleAIGenerate}
          />
          <Dialog open={isDialogOpen} onOpenChange={(open) => { setIsDialogOpen(open); if (!open) setWizardStep(1); }}>
            <DialogTrigger asChild>
              <Button variant="glow">
                <Plus className="w-4 h-4" />
                Create Mission
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-2xl">
              <DialogHeader>
                <DialogTitle>Create New Mission</DialogTitle>
                <DialogDescription>Step {wizardStep} of 4</DialogDescription>
              </DialogHeader>

              {/* Progress Steps */}
              <div className="flex items-center justify-center gap-2 py-4">
                {[1, 2, 3, 4].map((step) => (
                  <React.Fragment key={step}>
                    <div className={cn(
                      "w-10 h-10 rounded-full flex items-center justify-center text-sm font-medium transition-all",
                      step < wizardStep ? "bg-primary text-primary-foreground" :
                      step === wizardStep ? "bg-primary text-primary-foreground" :
                      "bg-secondary text-muted-foreground"
                    )}>
                      {step < wizardStep ? <Check className="w-5 h-5" /> : step}
                    </div>
                    {step < 4 && (
                      <div className={cn(
                        "w-12 h-1 rounded-full transition-all",
                        step < wizardStep ? "bg-primary" : "bg-secondary"
                      )} />
                    )}
                  </React.Fragment>
                ))}
              </div>

              <div className="py-4">
                {wizardStep === 1 && (
                  <div className="space-y-4">
                    <h3 className="font-semibold">Basic Information</h3>
                    <div className="space-y-2">
                      <Label htmlFor="name">Mission Name *</Label>
                      <Input
                        id="name"
                        placeholder="e.g., Spring Challenge"
                        value={formData.name}
                        onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="description">Description</Label>
                      <Textarea
                        id="description"
                        placeholder="Describe what users need to do..."
                        rows={3}
                        value={formData.description}
                        onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                      />
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label htmlFor="type">Mission Type</Label>
                        <Select
                          value={formData.type}
                          onValueChange={(value) => setFormData({ ...formData, type: value as MissionFormData['type'] })}
                        >
                          <SelectTrigger id="type"><SelectValue /></SelectTrigger>
                          <SelectContent>
                            <SelectItem value="one_time">One-time</SelectItem>
                            <SelectItem value="daily">Daily</SelectItem>
                            <SelectItem value="weekly">Weekly</SelectItem>
                            <SelectItem value="monthly">Monthly</SelectItem>
                            <SelectItem value="recurring">Recurring</SelectItem>
                            <SelectItem value="event">Event</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="status">Status</Label>
                        <Select
                          value={formData.status}
                          onValueChange={(value) => setFormData({ ...formData, status: value as MissionFormData['status'] })}
                        >
                          <SelectTrigger id="status"><SelectValue /></SelectTrigger>
                          <SelectContent>
                            <SelectItem value="draft">Draft</SelectItem>
                            <SelectItem value="active">Active</SelectItem>
                            <SelectItem value="paused">Paused</SelectItem>
                            <SelectItem value="completed">Completed</SelectItem>
                            <SelectItem value="expired">Expired</SelectItem>
                            <SelectItem value="cancelled">Cancelled</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                    <div className="flex items-center gap-4 pt-2">
                      <div className="flex items-center gap-2">
                        <Switch
                          id="is_active"
                          checked={formData.is_active}
                          onCheckedChange={(checked) => setFormData({ ...formData, is_active: checked })}
                        />
                        <Label htmlFor="is_active">Active</Label>
                      </div>
                      <div className="flex items-center gap-2">
                        <Switch
                          id="is_secret"
                          checked={formData.is_secret}
                          onCheckedChange={(checked) => setFormData({ ...formData, is_secret: checked })}
                        />
                        <Label htmlFor="is_secret">Secret Mission</Label>
                      </div>
                    </div>
                  </div>
                )}

                {wizardStep === 2 && (
                  <div className="space-y-4">
                    <h3 className="font-semibold">Objectives</h3>
                    <p className="text-sm text-muted-foreground">Define what users need to complete.</p>
                    <div className="space-y-3 max-h-64 overflow-y-auto">
                      {formData.objectives.length === 0 ? (
                        <p className="text-sm text-muted-foreground text-center py-4">
                          No objectives yet. Click "Add Objective" to get started.
                        </p>
                      ) : (
                        formData.objectives.map((obj, index) => (
                          <div key={obj.key} className="flex gap-2 items-start">
                            <div className="flex-1 space-y-2">
                              <Input
                                placeholder={`Objective ${index + 1}: e.g., Complete your profile`}
                                value={obj.label}
                                onChange={(e) => updateObjective(index, 'label', e.target.value)}
                              />
                              <div className="flex items-center gap-2">
                                <Label className="text-xs text-muted-foreground">Target:</Label>
                                <Input
                                  type="number"
                                  min="1"
                                  className="w-20"
                                  value={obj.target}
                                  onChange={(e) => updateObjective(index, 'target', parseInt(e.target.value) || 1)}
                                />
                              </div>
                            </div>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => removeObjective(index)}
                              className="mt-1"
                            >
                              <Trash2 className="w-4 h-4 text-destructive" />
                            </Button>
                          </div>
                        ))
                      )}
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={addObjective}
                      className="w-full"
                    >
                      <Plus className="w-4 h-4 mr-2" />
                      Add Objective
                    </Button>
                  </div>
                )}

                {wizardStep === 3 && (
                  <div className="space-y-4">
                    <h3 className="font-semibold">Rewards</h3>
                    <p className="text-sm text-muted-foreground">Set rewards for completing the mission.</p>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label htmlFor="xp_reward">XP Reward</Label>
                        <Input
                          id="xp_reward"
                          type="number"
                          min="0"
                          placeholder="500"
                          value={formData.xp_reward || ''}
                          onChange={(e) => setFormData({ ...formData, xp_reward: parseInt(e.target.value) || 0 })}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="points_reward">Points Reward</Label>
                        <Input
                          id="points_reward"
                          type="number"
                          min="0"
                          placeholder="100"
                          value={formData.points_reward || ''}
                          onChange={(e) => setFormData({ ...formData, points_reward: parseInt(e.target.value) || 0 })}
                        />
                      </div>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="badge_reward">Badge Reward (Optional)</Label>
                      <Select
                        value={formData.badge_reward_id?.toString() || 'none'}
                        onValueChange={(value) => setFormData({ ...formData, badge_reward_id: value === 'none' ? undefined : parseInt(value) })}
                      >
                        <SelectTrigger id="badge_reward">
                          <SelectValue placeholder="Select a badge" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="none">None</SelectItem>
                          {badges.map((badge) => (
                            <SelectItem key={badge.id} value={badge.id.toString()}>
                              {badge.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                )}

                {wizardStep === 4 && (
                  <div className="space-y-4">
                    <h3 className="font-semibold">Schedule & Limits</h3>
                    <p className="text-sm text-muted-foreground">Set when the mission is available and completion limits.</p>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label htmlFor="start_date">Start Date</Label>
                        <Input
                          id="start_date"
                          type="date"
                          value={formData.start_date}
                          onChange={(e) => setFormData({ ...formData, start_date: e.target.value })}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="end_date">End Date</Label>
                        <Input
                          id="end_date"
                          type="date"
                          value={formData.end_date}
                          onChange={(e) => setFormData({ ...formData, end_date: e.target.value })}
                        />
                      </div>
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <Label htmlFor="max_completions">Max Completions (Optional)</Label>
                        <Input
                          id="max_completions"
                          type="number"
                          min="1"
                          placeholder="Unlimited"
                          value={formData.max_completions || ''}
                          onChange={(e) => setFormData({ ...formData, max_completions: e.target.value ? parseInt(e.target.value) : undefined })}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="cooldown_hours">Cooldown Hours (Optional)</Label>
                        <Input
                          id="cooldown_hours"
                          type="number"
                          min="0"
                          placeholder="No cooldown"
                          value={formData.cooldown_hours || ''}
                          onChange={(e) => setFormData({ ...formData, cooldown_hours: e.target.value ? parseInt(e.target.value) : undefined })}
                        />
                      </div>
                    </div>
                  </div>
                )}
              </div>

              <DialogFooter>
                {wizardStep > 1 && (
                  <Button type="button" variant="outline" onClick={() => setWizardStep(wizardStep - 1)}>
                    Back
                  </Button>
                )}
                <Button type="button" variant="outline" onClick={resetWizard}>Cancel</Button>
                <Button
                  type="button"
                  variant="glow"
                  onClick={handleCreate}
                  disabled={createMutation.isPending}
                >
                  {createMutation.isPending ? (
                    'Creating...'
                  ) : wizardStep === 4 ? (
                    'Create Mission'
                  ) : (
                    <>
                      Continue
                      <ChevronRight className="w-4 h-4 ml-1" />
                    </>
                  )}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>

          {/* Edit Mission Dialog */}
          <Dialog open={isEditDialogOpen} onOpenChange={setIsEditDialogOpen}>
            <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto">
              <DialogHeader>
                <DialogTitle>Edit Mission</DialogTitle>
                <DialogDescription>Update mission details</DialogDescription>
              </DialogHeader>

              <div className="space-y-6 py-4">
                {/* Basic Information */}
                <div className="space-y-4">
                  <h3 className="font-semibold">Basic Information</h3>
                  <div className="space-y-2">
                    <Label htmlFor="edit-name">Mission Name *</Label>
                    <Input
                      id="edit-name"
                      placeholder="e.g., Spring Challenge"
                      value={formData.name}
                      onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="edit-description">Description</Label>
                    <Textarea
                      id="edit-description"
                      placeholder="Describe what users need to do..."
                      rows={3}
                      value={formData.description}
                      onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                    />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="edit-type">Mission Type</Label>
                      <Select
                        value={formData.type}
                        onValueChange={(value) => setFormData({ ...formData, type: value as MissionFormData['type'] })}
                      >
                        <SelectTrigger id="edit-type"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="one_time">One-time</SelectItem>
                          <SelectItem value="daily">Daily</SelectItem>
                          <SelectItem value="weekly">Weekly</SelectItem>
                          <SelectItem value="monthly">Monthly</SelectItem>
                          <SelectItem value="recurring">Recurring</SelectItem>
                          <SelectItem value="event">Event</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="edit-status">Status</Label>
                      <Select
                        value={formData.status}
                        onValueChange={(value) => setFormData({ ...formData, status: value as MissionFormData['status'] })}
                      >
                        <SelectTrigger id="edit-status"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="draft">Draft</SelectItem>
                          <SelectItem value="active">Active</SelectItem>
                          <SelectItem value="paused">Paused</SelectItem>
                          <SelectItem value="completed">Completed</SelectItem>
                          <SelectItem value="expired">Expired</SelectItem>
                          <SelectItem value="cancelled">Cancelled</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className="flex items-center gap-4">
                    <div className="flex items-center gap-2">
                      <Switch
                        id="edit-is_active"
                        checked={formData.is_active}
                        onCheckedChange={(checked) => setFormData({ ...formData, is_active: checked })}
                      />
                      <Label htmlFor="edit-is_active">Active</Label>
                    </div>
                    <div className="flex items-center gap-2">
                      <Switch
                        id="edit-is_secret"
                        checked={formData.is_secret}
                        onCheckedChange={(checked) => setFormData({ ...formData, is_secret: checked })}
                      />
                      <Label htmlFor="edit-is_secret">Secret Mission</Label>
                    </div>
                  </div>
                </div>

                {/* Objectives */}
                <div className="space-y-4">
                  <h3 className="font-semibold">Objectives</h3>
                  <div className="space-y-3 max-h-48 overflow-y-auto">
                    {formData.objectives.length === 0 ? (
                      <p className="text-sm text-muted-foreground text-center py-4">
                        No objectives yet. Click "Add Objective" to get started.
                      </p>
                    ) : (
                      formData.objectives.map((obj, index) => (
                        <div key={obj.key} className="flex gap-2 items-start">
                          <div className="flex-1 space-y-2">
                            <Input
                              placeholder={`Objective ${index + 1}`}
                              value={obj.label}
                              onChange={(e) => updateObjective(index, 'label', e.target.value)}
                            />
                            <div className="flex items-center gap-2">
                              <Label className="text-xs text-muted-foreground">Target:</Label>
                              <Input
                                type="number"
                                min="1"
                                className="w-20"
                                value={obj.target}
                                onChange={(e) => updateObjective(index, 'target', parseInt(e.target.value) || 1)}
                              />
                            </div>
                          </div>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => removeObjective(index)}
                            className="mt-1"
                          >
                            <Trash2 className="w-4 h-4 text-destructive" />
                          </Button>
                        </div>
                      ))
                    )}
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={addObjective}
                    className="w-full"
                  >
                    <Plus className="w-4 h-4 mr-2" />
                    Add Objective
                  </Button>
                </div>

                {/* Rewards */}
                <div className="space-y-4">
                  <h3 className="font-semibold">Rewards</h3>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="edit-xp_reward">XP Reward</Label>
                      <Input
                        id="edit-xp_reward"
                        type="number"
                        min="0"
                        value={formData.xp_reward || ''}
                        onChange={(e) => setFormData({ ...formData, xp_reward: parseInt(e.target.value) || 0 })}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="edit-points_reward">Points Reward</Label>
                      <Input
                        id="edit-points_reward"
                        type="number"
                        min="0"
                        value={formData.points_reward || ''}
                        onChange={(e) => setFormData({ ...formData, points_reward: parseInt(e.target.value) || 0 })}
                      />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="edit-badge_reward">Badge Reward (Optional)</Label>
                    <Select
                      value={formData.badge_reward_id?.toString() || 'none'}
                      onValueChange={(value) => setFormData({ ...formData, badge_reward_id: value === 'none' ? undefined : parseInt(value) })}
                    >
                      <SelectTrigger id="edit-badge_reward">
                        <SelectValue placeholder="Select a badge" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">None</SelectItem>
                        {badges.map((badge) => (
                          <SelectItem key={badge.id} value={badge.id.toString()}>
                            {badge.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                {/* Schedule */}
                <div className="space-y-4">
                  <h3 className="font-semibold">Schedule & Limits</h3>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="edit-start_date">Start Date</Label>
                      <Input
                        id="edit-start_date"
                        type="date"
                        value={formData.start_date}
                        onChange={(e) => setFormData({ ...formData, start_date: e.target.value })}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="edit-end_date">End Date</Label>
                      <Input
                        id="edit-end_date"
                        type="date"
                        value={formData.end_date}
                        onChange={(e) => setFormData({ ...formData, end_date: e.target.value })}
                      />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label htmlFor="edit-max_completions">Max Completions (Optional)</Label>
                      <Input
                        id="edit-max_completions"
                        type="number"
                        min="1"
                        placeholder="Unlimited"
                        value={formData.max_completions || ''}
                        onChange={(e) => setFormData({ ...formData, max_completions: e.target.value ? parseInt(e.target.value) : undefined })}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="edit-cooldown_hours">Cooldown Hours (Optional)</Label>
                      <Input
                        id="edit-cooldown_hours"
                        type="number"
                        min="0"
                        placeholder="No cooldown"
                        value={formData.cooldown_hours || ''}
                        onChange={(e) => setFormData({ ...formData, cooldown_hours: e.target.value ? parseInt(e.target.value) : undefined })}
                      />
                    </div>
                  </div>
                </div>
              </div>

              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setIsEditDialogOpen(false)}
                >
                  Cancel
                </Button>
                <Button
                  type="button"
                  variant="glow"
                  onClick={handleUpdate}
                  disabled={updateMutation.isPending}
                >
                  {updateMutation.isPending ? 'Updating...' : 'Update Mission'}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </div>

      {/* Search */}
      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <Input
          placeholder="Search missions..."
          className="pl-9"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
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
      ) : filteredMissions.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <Target className="w-16 h-16 mx-auto mb-4 text-muted-foreground" />
            <h3 className="text-lg font-medium mb-2">No missions found</h3>
            <p className="text-muted-foreground mb-4">
              {searchQuery ? 'Try adjusting your search query.' : 'Create your first mission to get started.'}
            </p>
            {!searchQuery && (
              <Button onClick={() => setIsDialogOpen(true)}>
                <Plus className="w-4 h-4 mr-2" />
                Create Mission
              </Button>
            )}
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {filteredMissions.map((mission: Mission, index: number) => {
            const objectives = mission.objectives ? Object.entries(mission.objectives) : [];
            const objectiveLabels = objectives.map(([_, obj]) => obj.label);

            return (
              <Card key={mission.id} className="stat-card overflow-hidden" style={{ animationDelay: `${index * 100}ms` }}>
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
                            : mission.status === 'draft'
                            ? "border-amber-500/50 text-amber-500 bg-amber-500/10"
                            : "border-muted-foreground"
                        )}
                      >
                        {mission.status}
                      </Badge>
                      <ItemActionsMenu
                        itemName={mission.name}
                        onEdit={() => handleEdit(mission)}
                        onDelete={() => handleDelete(mission)}
                        showInGroup
                      />
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  {/* Mission Type */}
                  <div className="flex items-center gap-2">
                    <Badge variant="secondary" className="capitalize">
                      {mission.type.replace('_', '-')}
                    </Badge>
                    {mission.is_secret && (
                      <Badge variant="outline">Secret</Badge>
                    )}
                  </div>

                  {/* Objectives */}
                  {objectiveLabels.length > 0 && (
                    <div className="space-y-2">
                      <p className="text-sm font-medium">Objectives</p>
                      <div className="space-y-1">
                        {objectiveLabels.map((label, i) => (
                          <div key={i} className="flex items-center gap-2 text-sm text-muted-foreground">
                            <div className="w-4 h-4 rounded-full border border-muted-foreground flex items-center justify-center">
                              {i < Math.floor(objectiveLabels.length * 0.5) && (
                                <Check className="w-3 h-3 text-green-500" />
                              )}
                            </div>
                            {label}
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Meta Info */}
                  <div className="flex items-center justify-between pt-2 border-t border-border">
                    <div className="flex items-center gap-4 text-sm text-muted-foreground">
                      {mission.end_date && (
                        <div className="flex items-center gap-1">
                          <Calendar className="w-4 h-4" />
                          {new Date(mission.end_date).toLocaleDateString()}
                        </div>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      {mission.xp_reward > 0 && (
                        <Badge variant="secondary">+{mission.xp_reward} XP</Badge>
                      )}
                      {mission.points_reward > 0 && (
                        <Badge variant="secondary">+{mission.points_reward} Points</Badge>
                      )}
                      {mission.badge_reward && (
                        <Badge variant="outline" className="border-primary/50 text-primary">
                          🏅 {mission.badge_reward.name}
                        </Badge>
                      )}
                    </div>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}

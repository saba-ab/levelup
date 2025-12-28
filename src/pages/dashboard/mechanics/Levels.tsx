import React, { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { levels as initialLevels } from '@/lib/mockData';
import { TrendingUp, Users, Sparkles, Plus } from 'lucide-react';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';

export default function Levels() {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [levelsList, setLevelsList] = useState(initialLevels);
  const [editingLevel, setEditingLevel] = useState<typeof initialLevels[0] | null>(null);
  const { toast } = useToast();

  const getTierColor = (tier: string) => {
    switch (tier) {
      case 'bronze': return 'bg-amber-700';
      case 'silver': return 'bg-gray-400';
      case 'gold': return 'bg-yellow-500';
      case 'platinum': return 'bg-slate-300';
      case 'diamond': return 'bg-cyan-400';
      default: return 'bg-primary';
    }
  };

  const totalUsers = levelsList.reduce((sum, level) => sum + level.usersCount, 0);

  const handleCreate = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    toast({
      title: editingLevel ? 'Level updated' : 'Level created',
      description: editingLevel ? 'Level has been updated successfully.' : 'New level has been added successfully.',
    });
    setIsDialogOpen(false);
    setEditingLevel(null);
  };

  const handleEdit = (level: typeof initialLevels[0]) => {
    setEditingLevel(level);
    setIsDialogOpen(true);
  };

  const handleDelete = (id: string) => {
    setLevelsList(prev => prev.filter(l => l.id !== id));
    toast({
      title: 'Level deleted',
      description: 'Level has been removed successfully.',
    });
  };

  // Connect this to your MySQL backend
  const handleAIGenerate = async (prompt: string): Promise<string> => {
    await new Promise(resolve => setTimeout(resolve, 1500));
    return `Generated Level Description:\n\n"${prompt}"\n\nThis tier rewards dedicated players who have shown consistent engagement. Members enjoy exclusive perks including early access to new features, special badges, and priority support.`;
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Levels & Tiers</h1>
          <p className="text-muted-foreground mt-1">Define progression levels for your users.</p>
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
          <Dialog open={isDialogOpen} onOpenChange={(open) => { setIsDialogOpen(open); if (!open) setEditingLevel(null); }}>
            <DialogTrigger asChild>
              <Button variant="glow">
                <Plus className="w-4 h-4" />
                Create Level
              </Button>
            </DialogTrigger>
            <DialogContent>
              <form onSubmit={handleCreate}>
                <DialogHeader>
                  <DialogTitle>{editingLevel ? 'Edit Level' : 'Create New Level'}</DialogTitle>
                  <DialogDescription>
                    {editingLevel ? 'Update the level details.' : 'Define a new progression level for your users.'}
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Level Name</label>
                    <Input placeholder="e.g., Elite Champion" defaultValue={editingLevel?.name} required />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Tier</label>
                      <Select defaultValue={editingLevel?.tier}>
                        <SelectTrigger><SelectValue placeholder="Select tier" /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="bronze">Bronze</SelectItem>
                          <SelectItem value="silver">Silver</SelectItem>
                          <SelectItem value="gold">Gold</SelectItem>
                          <SelectItem value="platinum">Platinum</SelectItem>
                          <SelectItem value="diamond">Diamond</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">XP Threshold</label>
                      <Input type="number" placeholder="5000" defaultValue={editingLevel?.xpThreshold} required />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Description</label>
                    <Input placeholder="What benefits does this level unlock?" />
                  </div>
                </div>
                <DialogFooter>
                  <Button type="button" variant="outline" onClick={() => { setIsDialogOpen(false); setEditingLevel(null); }}>
                    Cancel
                  </Button>
                  <Button type="submit" variant="glow">{editingLevel ? 'Save Changes' : 'Create Level'}</Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        </div>
      </div>

      {/* Level Progression Visualization */}
      <Card>
        <CardHeader>
          <CardTitle>Level Progression</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="relative">
            {/* Progress Track */}
            <div className="h-3 bg-secondary rounded-full overflow-hidden">
              <div className="h-full flex">
                {levelsList.map((level, index) => (
                  <div
                    key={level.id}
                    className={cn("h-full", getTierColor(level.tier))}
                    style={{ width: `${(level.usersCount / totalUsers) * 100}%` }}
                  />
                ))}
              </div>
            </div>
            
            {/* Level Markers */}
            <div className="flex justify-between mt-4">
              {levelsList.map((level) => (
                <div key={level.id} className="text-center">
                  <div
                    className={cn(
                      "w-12 h-12 rounded-full mx-auto mb-2 flex items-center justify-center",
                      getTierColor(level.tier)
                    )}
                  >
                    <TrendingUp className="w-6 h-6 text-white" />
                  </div>
                  <p className="font-semibold text-sm">{level.name}</p>
                  <p className="text-xs text-muted-foreground">{level.xpThreshold.toLocaleString()} XP</p>
                </div>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Levels Table */}
      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Level</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Tier</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">XP Threshold</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Users</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Distribution</th>
                  <th className="text-right p-4 text-sm font-medium text-muted-foreground">Actions</th>
                </tr>
              </thead>
              <tbody>
                {levelsList.map((level) => (
                  <tr key={level.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors group">
                    <td className="p-4">
                      <div className="flex items-center gap-3">
                        <div
                          className={cn(
                            "w-10 h-10 rounded-lg flex items-center justify-center",
                            getTierColor(level.tier)
                          )}
                        >
                          <TrendingUp className="w-5 h-5 text-white" />
                        </div>
                        <span className="font-medium">{level.name}</span>
                      </div>
                    </td>
                    <td className="p-4">
                      <Badge
                        variant="outline"
                        className="capitalize"
                        style={{ borderColor: level.color, color: level.color }}
                      >
                        {level.tier}
                      </Badge>
                    </td>
                    <td className="p-4 font-mono">
                      {level.xpThreshold.toLocaleString()} XP
                    </td>
                    <td className="p-4">
                      <div className="flex items-center gap-2">
                        <Users className="w-4 h-4 text-muted-foreground" />
                        <span>{level.usersCount.toLocaleString()}</span>
                      </div>
                    </td>
                    <td className="p-4">
                      <div className="w-32 h-2 bg-secondary rounded-full overflow-hidden">
                        <div
                          className={cn("h-full", getTierColor(level.tier))}
                          style={{ width: `${(level.usersCount / totalUsers) * 100}%` }}
                        />
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {((level.usersCount / totalUsers) * 100).toFixed(1)}%
                      </span>
                    </td>
                    <td className="p-4 text-right">
                      <ItemActionsMenu
                        itemName={level.name}
                        onEdit={() => handleEdit(level)}
                        onDelete={() => handleDelete(level.id)}
                        showInGroup
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

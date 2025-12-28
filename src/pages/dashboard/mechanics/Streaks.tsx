import React, { useState } from 'react';
import { Plus, Flame, Clock, Gift, Users, Zap, Sparkles } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
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

const streaks = [
  { 
    id: '1', 
    name: 'Daily Login Streak', 
    description: 'Log in every day to maintain your streak',
    interval: 'daily', 
    gracePeriod: 12, 
    activeStreaks: 4521,
    longestStreak: 365,
    milestones: [
      { days: 7, reward: '50 XP' },
      { days: 30, reward: '250 XP + Badge' },
      { days: 100, reward: '1000 XP + Exclusive Badge' },
    ]
  },
  { 
    id: '2', 
    name: 'Weekly Purchase Streak', 
    description: 'Make at least one purchase every week',
    interval: 'weekly', 
    gracePeriod: 24, 
    activeStreaks: 1234,
    longestStreak: 52,
    milestones: [
      { days: 4, reward: '100 XP' },
      { days: 12, reward: '500 XP + Badge' },
      { days: 26, reward: '2000 XP + VIP Status' },
    ]
  },
  { 
    id: '3', 
    name: 'Workout Streak', 
    description: 'Log a workout session every day',
    interval: 'daily', 
    gracePeriod: 6, 
    activeStreaks: 890,
    longestStreak: 180,
    milestones: [
      { days: 7, reward: '75 XP' },
      { days: 30, reward: '300 XP + Fitness Badge' },
      { days: 90, reward: '1500 XP + Champion Badge' },
    ]
  },
];

const streakLeaders = [
  { userId: 'usr_001', name: 'Alex Chen', streak: 365, avatar: 'AC' },
  { userId: 'usr_089', name: 'Sarah Miller', streak: 287, avatar: 'SM' },
  { userId: 'usr_023', name: 'James Wilson', streak: 234, avatar: 'JW' },
  { userId: 'usr_045', name: 'Emily Brown', streak: 198, avatar: 'EB' },
  { userId: 'usr_112', name: 'Michael Davis', streak: 156, avatar: 'MD' },
];

export default function Streaks() {
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const { toast } = useToast();

  const handleCreate = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    toast({
      title: 'Streak created',
      description: 'New streak has been added successfully.',
    });
    setIsDialogOpen(false);
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
            <DialogContent>
              <form onSubmit={handleCreate}>
                <DialogHeader>
                  <DialogTitle>Create New Streak</DialogTitle>
                  <DialogDescription>Define a new streak mechanic for your users.</DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Streak Name</label>
                    <Input placeholder="e.g., Daily Challenge Streak" required />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Description</label>
                    <Input placeholder="What action maintains the streak?" />
                  </div>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Interval</label>
                      <Select defaultValue="daily">
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="daily">Daily</SelectItem>
                          <SelectItem value="weekly">Weekly</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Grace Period (hours)</label>
                      <Input type="number" placeholder="12" />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Trigger Event</label>
                    <Select>
                      <SelectTrigger><SelectValue placeholder="Select event" /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="user_login">User Login</SelectItem>
                        <SelectItem value="purchase_completed">Purchase Completed</SelectItem>
                        <SelectItem value="workout_logged">Workout Logged</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <DialogFooter>
                  <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>Cancel</Button>
                  <Button type="submit" variant="glow">Create Streak</Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        </div>
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
                <p className="text-2xl font-bold">6,645</p>
                <p className="text-sm text-muted-foreground">Active Streaks</p>
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
                <p className="text-2xl font-bold">365</p>
                <p className="text-sm text-muted-foreground">Longest Streak</p>
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
                <p className="text-2xl font-bold">12,430</p>
                <p className="text-sm text-muted-foreground">Milestones Hit</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Streak Definitions */}
        <div className="lg:col-span-2 space-y-4">
          <h2 className="text-xl font-semibold">Streak Definitions</h2>
          {streaks.map((streak, index) => (
            <Card key={streak.id} className="stat-card" style={{ animationDelay: `${index * 100}ms` }}>
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-12 h-12 rounded-xl bg-orange-500/10 flex items-center justify-center">
                      <Flame className="w-6 h-6 text-orange-500" />
                    </div>
                    <div>
                      <CardTitle className="text-lg">{streak.name}</CardTitle>
                      <CardDescription>{streak.description}</CardDescription>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className="capitalize">{streak.interval}</Badge>
                    <ItemActionsMenu
                      itemName={streak.name}
                      onEdit={() => {}}
                      onDelete={() => {}}
                      showInGroup
                    />
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center gap-6 text-sm">
                  <div className="flex items-center gap-2">
                    <Clock className="w-4 h-4 text-muted-foreground" />
                    <span className="text-muted-foreground">Grace: {streak.gracePeriod}h</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <Users className="w-4 h-4 text-muted-foreground" />
                    <span className="text-muted-foreground">{streak.activeStreaks.toLocaleString()} active</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <Flame className="w-4 h-4 text-orange-500" />
                    <span className="text-muted-foreground">Longest: {streak.longestStreak}</span>
                  </div>
                </div>

                <div className="space-y-2">
                  <p className="text-sm font-medium">Milestone Rewards</p>
                  <div className="flex flex-wrap gap-2">
                    {streak.milestones.map((milestone, i) => (
                      <Badge key={i} variant="secondary" className="bg-secondary/80">
                        {streak.interval === 'daily' ? `${milestone.days} days` : `${milestone.days} weeks`}: {milestone.reward}
                      </Badge>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>

        {/* Streak Leaders */}
        <div className="space-y-4">
          <h2 className="text-xl font-semibold">Streak Leaders</h2>
          <Card>
            <CardContent className="p-0">
              {streakLeaders.map((leader, index) => (
                <div
                  key={leader.userId}
                  className={cn(
                    "flex items-center gap-3 p-4 transition-colors hover:bg-secondary/30",
                    index < streakLeaders.length - 1 && "border-b border-border/50"
                  )}
                >
                  <div className="text-lg font-bold text-muted-foreground w-6">
                    {index + 1}
                  </div>
                  <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center">
                    <span className="text-sm font-medium">{leader.avatar}</span>
                  </div>
                  <div className="flex-1">
                    <p className="font-medium">{leader.name}</p>
                    <p className="text-xs text-muted-foreground">{leader.userId}</p>
                  </div>
                  <div className="flex items-center gap-1 text-orange-500">
                    <Flame className="w-4 h-4" />
                    <span className="font-bold">{leader.streak}</span>
                  </div>
                </div>
              ))}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

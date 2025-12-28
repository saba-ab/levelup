import React, { useState } from 'react';
import { Plus, Search, Target, Calendar, Users, ChevronRight, Check, Sparkles } from 'lucide-react';
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
import { missions } from '@/lib/mockData';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';

const missionDetails = [
  { 
    id: '1', 
    name: 'Welcome Journey', 
    description: 'Complete onboarding tasks to earn bonus XP',
    type: 'one-time', 
    status: 'active', 
    progress: 67,
    participants: 1250,
    objectives: ['Complete profile', 'Make first purchase', 'Invite a friend'],
    rewards: { xp: 500, badge: 'First Steps' },
    startDate: '2024-03-01', 
    endDate: '2024-04-01' 
  },
  { 
    id: '2', 
    name: 'Weekly Warrior', 
    description: 'Complete weekly challenges for bonus rewards',
    type: 'recurring', 
    status: 'active', 
    progress: 45,
    participants: 890,
    objectives: ['Log in 5 days', 'Make 3 purchases', 'Leave a review'],
    rewards: { xp: 250, coins: 100 },
    startDate: '2024-03-18', 
    endDate: '2024-03-25' 
  },
  { 
    id: '3', 
    name: 'Holiday Special', 
    description: 'Limited time holiday mission with exclusive rewards',
    type: 'one-time', 
    status: 'draft', 
    progress: 0,
    participants: 0,
    objectives: ['Visit 5 product pages', 'Add to wishlist', 'Share on social'],
    rewards: { xp: 1000, badge: 'Holiday Hero' },
    startDate: '2024-04-15', 
    endDate: '2024-04-30' 
  },
];

export default function Missions() {
  const [searchQuery, setSearchQuery] = useState('');
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [wizardStep, setWizardStep] = useState(1);
  const { toast } = useToast();

  const filteredMissions = missionDetails.filter(mission =>
    mission.name.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handleCreate = () => {
    if (wizardStep < 4) {
      setWizardStep(wizardStep + 1);
    } else {
      toast({
        title: 'Mission created',
        description: 'New mission has been created successfully.',
      });
      setIsDialogOpen(false);
      setWizardStep(1);
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
                      <label className="text-sm font-medium">Mission Name</label>
                      <Input placeholder="e.g., Spring Challenge" />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Description</label>
                      <Textarea placeholder="Describe what users need to do..." rows={3} />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Mission Type</label>
                      <Select defaultValue="one-time">
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="one-time">One-time</SelectItem>
                          <SelectItem value="recurring">Recurring</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                )}

                {wizardStep === 2 && (
                  <div className="space-y-4">
                    <h3 className="font-semibold">Objectives</h3>
                    <p className="text-sm text-muted-foreground">Define what users need to complete.</p>
                    <div className="space-y-3">
                      <div className="flex gap-2">
                        <Input placeholder="Objective 1: e.g., Complete your profile" className="flex-1" />
                        <Button variant="ghost" size="icon"><Plus className="w-4 h-4" /></Button>
                      </div>
                      <div className="flex gap-2">
                        <Input placeholder="Objective 2: e.g., Make a purchase" className="flex-1" />
                        <Button variant="ghost" size="icon"><Plus className="w-4 h-4" /></Button>
                      </div>
                      <div className="flex gap-2">
                        <Input placeholder="Objective 3: e.g., Invite a friend" className="flex-1" />
                        <Button variant="ghost" size="icon"><Plus className="w-4 h-4" /></Button>
                      </div>
                    </div>
                  </div>
                )}

                {wizardStep === 3 && (
                  <div className="space-y-4">
                    <h3 className="font-semibold">Rewards</h3>
                    <p className="text-sm text-muted-foreground">Set rewards for completing the mission.</p>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-sm font-medium">XP Reward</label>
                        <Input type="number" placeholder="500" />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">Coins Reward</label>
                        <Input type="number" placeholder="100" />
                      </div>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Badge Reward (Optional)</label>
                      <Select>
                        <SelectTrigger><SelectValue placeholder="Select a badge" /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="first_steps">First Steps</SelectItem>
                          <SelectItem value="power_user">Power User</SelectItem>
                          <SelectItem value="streak_master">Streak Master</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                )}

                {wizardStep === 4 && (
                  <div className="space-y-4">
                    <h3 className="font-semibold">Schedule</h3>
                    <p className="text-sm text-muted-foreground">Set when the mission is available.</p>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-2">
                        <label className="text-sm font-medium">Start Date</label>
                        <Input type="date" />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">End Date</label>
                        <Input type="date" />
                      </div>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Timezone</label>
                      <Select defaultValue="UTC">
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="UTC">UTC</SelectItem>
                          <SelectItem value="America/New_York">Eastern Time</SelectItem>
                          <SelectItem value="America/Los_Angeles">Pacific Time</SelectItem>
                        </SelectContent>
                      </Select>
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
                <Button type="button" variant="glow" onClick={handleCreate}>
                  {wizardStep === 4 ? 'Create Mission' : 'Continue'}
                  {wizardStep < 4 && <ChevronRight className="w-4 h-4 ml-1" />}
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
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {filteredMissions.map((mission, index) => (
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
                    <CardDescription>{mission.description}</CardDescription>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge
                    variant="outline"
                    className={cn(
                      "capitalize",
                      mission.status === 'active'
                        ? "border-green-500/50 text-green-500 bg-green-500/10"
                        : "border-amber-500/50 text-amber-500 bg-amber-500/10"
                    )}
                  >
                    {mission.status}
                  </Badge>
                  <ItemActionsMenu
                    itemName={mission.name}
                    onEdit={() => {}}
                    onDelete={() => {}}
                    showInGroup
                  />
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Progress */}
              <div className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">Progress</span>
                  <span className="font-medium">{mission.progress}%</span>
                </div>
                <Progress value={mission.progress} className="h-2" />
              </div>

              {/* Objectives */}
              <div className="space-y-2">
                <p className="text-sm font-medium">Objectives</p>
                <div className="space-y-1">
                  {mission.objectives.map((obj, i) => (
                    <div key={i} className="flex items-center gap-2 text-sm text-muted-foreground">
                      <div className={cn(
                        "w-4 h-4 rounded-full border flex items-center justify-center",
                        i < Math.floor(mission.objectives.length * mission.progress / 100)
                          ? "bg-green-500 border-green-500"
                          : "border-muted-foreground"
                      )}>
                        {i < Math.floor(mission.objectives.length * mission.progress / 100) && (
                          <Check className="w-3 h-3 text-white" />
                        )}
                      </div>
                      {obj}
                    </div>
                  ))}
                </div>
              </div>

              {/* Meta Info */}
              <div className="flex items-center justify-between pt-2 border-t border-border">
                <div className="flex items-center gap-4 text-sm text-muted-foreground">
                  <div className="flex items-center gap-1">
                    <Users className="w-4 h-4" />
                    {mission.participants.toLocaleString()}
                  </div>
                  <div className="flex items-center gap-1">
                    <Calendar className="w-4 h-4" />
                    {mission.endDate}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="secondary">+{mission.rewards.xp} XP</Badge>
                  {mission.rewards.badge && (
                    <Badge variant="outline" className="border-primary/50 text-primary">
                      🏅 {mission.rewards.badge}
                    </Badge>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowLeft, Plus, Trash2, Zap, GitBranch, Award, Target, Save, Play } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';

interface Condition {
  id: string;
  type: 'first_time' | 'count' | 'sum' | 'time_window';
  config: Record<string, any>;
}

interface Action {
  id: string;
  type: 'award_points' | 'unlock_badge' | 'advance_mission' | 'emit_webhook';
  config: Record<string, any>;
}

const eventTypes = [
  { value: 'purchase_completed', label: 'Purchase Completed', icon: '🛒' },
  { value: 'user_signup', label: 'User Signup', icon: '👋' },
  { value: 'user_login', label: 'User Login', icon: '🔐' },
  { value: 'referral_completed', label: 'Referral Completed', icon: '🔗' },
  { value: 'workout_logged', label: 'Workout Logged', icon: '💪' },
  { value: 'subscription_upgraded', label: 'Subscription Upgraded', icon: '⬆️' },
];

export default function RuleBuilder() {
  const navigate = useNavigate();
  const { toast } = useToast();
  const [ruleName, setRuleName] = useState('');
  const [description, setDescription] = useState('');
  const [triggerEvent, setTriggerEvent] = useState('');
  const [conditions, setConditions] = useState<Condition[]>([]);
  const [actions, setActions] = useState<Action[]>([]);

  const addCondition = (type: Condition['type']) => {
    setConditions([
      ...conditions,
      { id: crypto.randomUUID(), type, config: {} }
    ]);
  };

  const removeCondition = (id: string) => {
    setConditions(conditions.filter(c => c.id !== id));
  };

  const addAction = (type: Action['type']) => {
    setActions([
      ...actions,
      { id: crypto.randomUUID(), type, config: {} }
    ]);
  };

  const removeAction = (id: string) => {
    setActions(actions.filter(a => a.id !== id));
  };

  const handleSave = (publish: boolean) => {
    if (!ruleName || !triggerEvent) {
      toast({
        title: 'Missing required fields',
        description: 'Please provide a rule name and trigger event.',
        variant: 'destructive',
      });
      return;
    }

    toast({
      title: publish ? 'Rule published' : 'Rule saved as draft',
      description: `"${ruleName}" has been ${publish ? 'published' : 'saved'}.`,
    });
    navigate('/rules');
  };

  const getConditionLabel = (type: Condition['type']) => {
    switch (type) {
      case 'first_time': return 'First Time';
      case 'count': return 'Event Count';
      case 'sum': return 'Sum of Values';
      case 'time_window': return 'Time Window';
    }
  };

  const getActionLabel = (type: Action['type']) => {
    switch (type) {
      case 'award_points': return 'Award Points';
      case 'unlock_badge': return 'Unlock Badge';
      case 'advance_mission': return 'Advance Mission';
      case 'emit_webhook': return 'Emit Webhook';
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={() => navigate('/rules')}>
          <ArrowLeft className="w-5 h-5" />
        </Button>
        <div className="flex-1">
          <h1 className="text-3xl font-bold">Create New Rule</h1>
          <p className="text-muted-foreground mt-1">Define triggers, conditions, and actions for your gamification rule.</p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Main Builder */}
        <div className="lg:col-span-2 space-y-6">
          {/* Trigger Section */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Zap className="w-5 h-5 text-primary" />
                Trigger Event
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">When this event occurs:</label>
                <Select value={triggerEvent} onValueChange={setTriggerEvent}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select an event type" />
                  </SelectTrigger>
                  <SelectContent>
                    {eventTypes.map(event => (
                      <SelectItem key={event.value} value={event.value}>
                        <div className="flex items-center gap-2">
                          <span>{event.icon}</span>
                          <span>{event.label}</span>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {triggerEvent && (
                <div className="p-4 bg-secondary/50 rounded-lg">
                  <p className="text-sm text-muted-foreground mb-2">Property filters (optional):</p>
                  <div className="flex gap-2">
                    <Input placeholder="Property name" className="flex-1" />
                    <Select defaultValue="equals">
                      <SelectTrigger className="w-32">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="equals">=</SelectItem>
                        <SelectItem value="gt">&gt;</SelectItem>
                        <SelectItem value="lt">&lt;</SelectItem>
                        <SelectItem value="contains">contains</SelectItem>
                      </SelectContent>
                    </Select>
                    <Input placeholder="Value" className="flex-1" />
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Conditions Section */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <GitBranch className="w-5 h-5 text-blue-500" />
                Conditions
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {conditions.length === 0 && (
                <p className="text-sm text-muted-foreground text-center py-4">
                  No conditions added. The rule will trigger on every matching event.
                </p>
              )}
              
              {conditions.map(condition => (
                <div key={condition.id} className="p-4 bg-secondary/50 rounded-lg flex items-start justify-between gap-4">
                  <div className="flex-1">
                    <Badge variant="outline" className="mb-2">{getConditionLabel(condition.type)}</Badge>
                    <div className="grid grid-cols-2 gap-2 mt-2">
                      {condition.type === 'count' && (
                        <>
                          <Input placeholder="Event type" />
                          <Input placeholder="Minimum count" type="number" />
                        </>
                      )}
                      {condition.type === 'time_window' && (
                        <>
                          <Input placeholder="Duration" type="number" />
                          <Select defaultValue="days">
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="hours">Hours</SelectItem>
                              <SelectItem value="days">Days</SelectItem>
                              <SelectItem value="weeks">Weeks</SelectItem>
                            </SelectContent>
                          </Select>
                        </>
                      )}
                      {condition.type === 'first_time' && (
                        <p className="text-sm text-muted-foreground col-span-2">
                          This condition passes only the first time the event occurs for a user.
                        </p>
                      )}
                    </div>
                  </div>
                  <Button variant="ghost" size="icon" onClick={() => removeCondition(condition.id)}>
                    <Trash2 className="w-4 h-4 text-destructive" />
                  </Button>
                </div>
              ))}

              <div className="flex flex-wrap gap-2 pt-2">
                <Button variant="outline" size="sm" onClick={() => addCondition('first_time')}>
                  <Plus className="w-4 h-4 mr-1" />
                  First Time
                </Button>
                <Button variant="outline" size="sm" onClick={() => addCondition('count')}>
                  <Plus className="w-4 h-4 mr-1" />
                  Count
                </Button>
                <Button variant="outline" size="sm" onClick={() => addCondition('time_window')}>
                  <Plus className="w-4 h-4 mr-1" />
                  Time Window
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* Actions Section */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Award className="w-5 h-5 text-green-500" />
                Actions
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {actions.length === 0 && (
                <p className="text-sm text-muted-foreground text-center py-4">
                  Add at least one action to execute when conditions are met.
                </p>
              )}

              {actions.map(action => (
                <div key={action.id} className="p-4 bg-secondary/50 rounded-lg flex items-start justify-between gap-4">
                  <div className="flex-1">
                    <Badge variant="outline" className={cn(
                      "mb-2",
                      action.type === 'award_points' && "border-amber-500/50 text-amber-500",
                      action.type === 'unlock_badge' && "border-purple-500/50 text-purple-500",
                      action.type === 'advance_mission' && "border-blue-500/50 text-blue-500",
                    )}>{getActionLabel(action.type)}</Badge>
                    <div className="grid grid-cols-2 gap-2 mt-2">
                      {action.type === 'award_points' && (
                        <>
                          <Input placeholder="Amount" type="number" />
                          <Select defaultValue="XP">
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="XP">XP</SelectItem>
                              <SelectItem value="Coins">Coins</SelectItem>
                              <SelectItem value="Points">Points</SelectItem>
                            </SelectContent>
                          </Select>
                        </>
                      )}
                      {action.type === 'unlock_badge' && (
                        <Select>
                          <SelectTrigger className="col-span-2">
                            <SelectValue placeholder="Select badge" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="first_steps">First Steps</SelectItem>
                            <SelectItem value="early_bird">Early Bird</SelectItem>
                            <SelectItem value="shopaholic">Shopaholic</SelectItem>
                          </SelectContent>
                        </Select>
                      )}
                    </div>
                  </div>
                  <Button variant="ghost" size="icon" onClick={() => removeAction(action.id)}>
                    <Trash2 className="w-4 h-4 text-destructive" />
                  </Button>
                </div>
              ))}

              <div className="flex flex-wrap gap-2 pt-2">
                <Button variant="outline" size="sm" onClick={() => addAction('award_points')}>
                  <Plus className="w-4 h-4 mr-1" />
                  Award Points
                </Button>
                <Button variant="outline" size="sm" onClick={() => addAction('unlock_badge')}>
                  <Plus className="w-4 h-4 mr-1" />
                  Unlock Badge
                </Button>
                <Button variant="outline" size="sm" onClick={() => addAction('advance_mission')}>
                  <Plus className="w-4 h-4 mr-1" />
                  Advance Mission
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Sidebar */}
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Rule Settings</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Rule Name</label>
                <Input
                  placeholder="e.g., First Purchase Bonus"
                  value={ruleName}
                  onChange={(e) => setRuleName(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Description</label>
                <Textarea
                  placeholder="Describe what this rule does..."
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  rows={3}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Program</label>
                <Select defaultValue="onboarding">
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="onboarding">Onboarding XP</SelectItem>
                    <SelectItem value="loyalty">Loyalty Rewards</SelectItem>
                    <SelectItem value="weekly">Weekly Challenges</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Max Awards per User/Day</label>
                <Input type="number" placeholder="Leave empty for unlimited" />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Target className="w-5 h-5" />
                Test Rule
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground mb-4">
                Simulate this rule with sample data to see what effects it would produce.
              </p>
              <Button variant="outline" className="w-full">
                <Play className="w-4 h-4 mr-2" />
                Run Simulation
              </Button>
            </CardContent>
          </Card>

          <div className="flex flex-col gap-2">
            <Button variant="glow" className="w-full" onClick={() => handleSave(true)}>
              <Zap className="w-4 h-4" />
              Publish Rule
            </Button>
            <Button variant="outline" className="w-full" onClick={() => handleSave(false)}>
              <Save className="w-4 h-4" />
              Save as Draft
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

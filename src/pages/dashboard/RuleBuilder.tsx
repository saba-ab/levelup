import React, { useState, useMemo } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { ArrowLeft, Plus, Trash2, Zap, GitBranch, Award, Target, Save, Play, ExternalLink, Loader2, Code, Info, AlertCircle } from 'lucide-react';
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { useEventsQuery } from '@/services/queries/events';
import type { TriggerEventProperty } from '@/services/api/types';

interface PropertyFilter {
  id: string;
  property: string;
  operator: 'eq' | 'neq' | 'gt' | 'lt' | 'gte' | 'lte' | 'contains' | 'in';
  value: string;
}

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

const operatorLabels: Record<PropertyFilter['operator'], string> = {
  eq: '=',
  neq: '≠',
  gt: '>',
  lt: '<',
  gte: '≥',
  lte: '≤',
  contains: 'contains',
  in: 'in',
};

const operatorsForType: Record<string, PropertyFilter['operator'][]> = {
  string: ['eq', 'neq', 'contains', 'in'],
  number: ['eq', 'neq', 'gt', 'lt', 'gte', 'lte'],
  boolean: ['eq', 'neq'],
  array: ['contains', 'in'],
  object: ['eq', 'neq'],
};

const getTypeColor = (type: string) => {
  switch (type) {
    case 'string': return 'text-green-500 border-green-500/50';
    case 'number': return 'text-blue-500 border-blue-500/50';
    case 'boolean': return 'text-amber-500 border-amber-500/50';
    case 'array': return 'text-purple-500 border-purple-500/50';
    case 'object': return 'text-pink-500 border-pink-500/50';
    default: return 'text-muted-foreground';
  }
};

export default function RuleBuilder() {
  const navigate = useNavigate();
  const { toast } = useToast();
  const [ruleName, setRuleName] = useState('');
  const [description, setDescription] = useState('');
  const [triggerEvent, setTriggerEvent] = useState('');
  const [propertyFilters, setPropertyFilters] = useState<PropertyFilter[]>([]);
  const [conditions, setConditions] = useState<Condition[]>([]);
  const [actions, setActions] = useState<Action[]>([]);

  // Fetch events from API
  const { data: eventsData, isLoading: eventsLoading } = useEventsQuery({ is_active: true });
  const events = eventsData || [];

  // Get the selected event and its properties
  const selectedEvent = useMemo(() => {
    return events.find(e => e.key === triggerEvent);
  }, [events, triggerEvent]);

  const eventProperties = useMemo(() => {
    return selectedEvent?.properties || [];
  }, [selectedEvent]);

  // Clear filters when event changes
  const handleEventChange = (eventKey: string) => {
    setTriggerEvent(eventKey);
    setPropertyFilters([]);
  };

  // Property filter management
  const addPropertyFilter = () => {
    const firstProperty = eventProperties[0];
    setPropertyFilters([
      ...propertyFilters,
      {
        id: crypto.randomUUID(),
        property: firstProperty?.name || '',
        operator: 'eq',
        value: '',
      }
    ]);
  };

  const updatePropertyFilter = (id: string, updates: Partial<PropertyFilter>) => {
    setPropertyFilters(filters =>
      filters.map(f => f.id === id ? { ...f, ...updates } : f)
    );
  };

  const removePropertyFilter = (id: string) => {
    setPropertyFilters(filters => filters.filter(f => f.id !== id));
  };

  const getPropertyByName = (name: string): TriggerEventProperty | undefined => {
    return eventProperties.find(p => p.name === name);
  };

  const getAvailableOperators = (propertyName: string): PropertyFilter['operator'][] => {
    const property = getPropertyByName(propertyName);
    return operatorsForType[property?.type || 'string'] || operatorsForType.string;
  };

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

    // Validate property filters
    const invalidFilters = propertyFilters.filter(f => !f.property || !f.value);
    if (invalidFilters.length > 0) {
      toast({
        title: 'Incomplete property filters',
        description: 'Please fill in all property filter values or remove empty filters.',
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
                <div className="flex items-center justify-between">
                  <label className="text-sm font-medium">When this event occurs:</label>
                  <Link to="/events" className="text-xs text-primary hover:underline flex items-center gap-1">
                    Manage Events <ExternalLink className="w-3 h-3" />
                  </Link>
                </div>
                <Select value={triggerEvent} onValueChange={handleEventChange}>
                  <SelectTrigger>
                    <SelectValue placeholder={eventsLoading ? "Loading events..." : "Select an event type"} />
                  </SelectTrigger>
                  <SelectContent>
                    {eventsLoading ? (
                      <div className="flex items-center justify-center py-4">
                        <Loader2 className="w-4 h-4 animate-spin mr-2" />
                        <span className="text-sm text-muted-foreground">Loading events...</span>
                      </div>
                    ) : events.length === 0 ? (
                      <div className="py-4 px-2 text-center">
                        <p className="text-sm text-muted-foreground mb-2">No events found</p>
                        <Link to="/events">
                          <Button size="sm" variant="outline">
                            <Plus className="w-3 h-3 mr-1" />
                            Create Event
                          </Button>
                        </Link>
                      </div>
                    ) : (
                      events.map(event => (
                        <SelectItem key={event.key} value={event.key}>
                          <div className="flex items-center gap-2">
                            <span>{event.icon || '⚡'}</span>
                            <span>{event.name}</span>
                            {event.properties && event.properties.length > 0 && (
                              <Badge variant="secondary" className="text-[10px] ml-1">
                                {event.properties.length} props
                              </Badge>
                            )}
                          </div>
                        </SelectItem>
                      ))
                    )}
                  </SelectContent>
                </Select>
              </div>

              {/* Event Properties Display */}
              {selectedEvent && (
                <div className="space-y-4">
                  {/* Event Info */}
                  <div className="p-4 bg-primary/5 border border-primary/20 rounded-lg">
                    <div className="flex items-start gap-3">
                      <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center text-xl">
                        {selectedEvent.icon || '⚡'}
                      </div>
                      <div className="flex-1">
                        <div className="flex items-center gap-2">
                          <h4 className="font-medium">{selectedEvent.name}</h4>
                          <code className="text-xs bg-secondary px-1.5 py-0.5 rounded">{selectedEvent.key}</code>
                        </div>
                        {selectedEvent.description && (
                          <p className="text-sm text-muted-foreground mt-1">{selectedEvent.description}</p>
                        )}
                      </div>
                    </div>
                  </div>

                  {/* Event Properties Schema */}
                  {eventProperties.length > 0 && (
                    <div className="space-y-3">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <Code className="w-4 h-4 text-muted-foreground" />
                          <span className="text-sm font-medium">Event Properties</span>
                        </div>
                        <Badge variant="outline">{eventProperties.length} available</Badge>
                      </div>
                      <div className="grid gap-2">
                        {eventProperties.map((prop, index) => (
                          <div
                            key={index}
                            className="flex items-center gap-3 p-3 bg-secondary/30 rounded-lg border border-border/50"
                          >
                            <div className="flex-1 flex items-center gap-2">
                              <code className="text-sm font-mono font-medium">{prop.name}</code>
                              <Badge variant="outline" className={cn("text-xs", getTypeColor(prop.type))}>
                                {prop.type}
                              </Badge>
                              {prop.required && (
                                <Badge variant="destructive" className="text-[10px]">Required</Badge>
                              )}
                            </div>
                            {prop.description && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Info className="w-4 h-4 text-muted-foreground cursor-help" />
                                </TooltipTrigger>
                                <TooltipContent side="left" className="max-w-xs">
                                  <p className="text-sm">{prop.description}</p>
                                </TooltipContent>
                              </Tooltip>
                            )}
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {eventProperties.length === 0 && (
                    <div className="flex items-center gap-2 p-3 bg-amber-500/10 border border-amber-500/30 rounded-lg text-amber-500">
                      <AlertCircle className="w-4 h-4" />
                      <span className="text-sm">This event has no defined properties. Consider adding properties in the Events page.</span>
                    </div>
                  )}

                  {/* Property Filters */}
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium">Property Filters (optional)</span>
                      {eventProperties.length > 0 && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={addPropertyFilter}
                        >
                          <Plus className="w-3 h-3 mr-1" />
                          Add Filter
                        </Button>
                      )}
                    </div>
                    
                    {propertyFilters.length === 0 ? (
                      <p className="text-sm text-muted-foreground py-2">
                        No filters added. The rule will trigger for all events of this type.
                      </p>
                    ) : (
                      <div className="space-y-2">
                        {propertyFilters.map((filter) => {
                          const property = getPropertyByName(filter.property);
                          const availableOperators = getAvailableOperators(filter.property);
                          
                          return (
                            <div key={filter.id} className="flex items-center gap-2 p-3 bg-secondary/50 rounded-lg">
                              <Select
                                value={filter.property}
                                onValueChange={(value) => {
                                  const newOperators = getAvailableOperators(value);
                                  updatePropertyFilter(filter.id, {
                                    property: value,
                                    operator: newOperators[0],
                                  });
                                }}
                              >
                                <SelectTrigger className="w-40">
                                  <SelectValue placeholder="Property" />
                                </SelectTrigger>
                                <SelectContent>
                                  {eventProperties.map((prop) => (
                                    <SelectItem key={prop.name} value={prop.name}>
                                      <div className="flex items-center gap-2">
                                        <code className="text-xs">{prop.name}</code>
                                        <Badge variant="outline" className={cn("text-[10px]", getTypeColor(prop.type))}>
                                          {prop.type}
                                        </Badge>
                                      </div>
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>

                              <Select
                                value={filter.operator}
                                onValueChange={(value: PropertyFilter['operator']) =>
                                  updatePropertyFilter(filter.id, { operator: value })
                                }
                              >
                                <SelectTrigger className="w-28">
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                  {availableOperators.map((op) => (
                                    <SelectItem key={op} value={op}>
                                      {operatorLabels[op]}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>

                              <Input
                                placeholder={property?.type === 'number' ? 'Enter number' : 'Enter value'}
                                type={property?.type === 'number' ? 'number' : 'text'}
                                value={filter.value}
                                onChange={(e) => updatePropertyFilter(filter.id, { value: e.target.value })}
                                className="flex-1"
                              />

                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-8 w-8"
                                onClick={() => removePropertyFilter(filter.id)}
                              >
                                <Trash2 className="w-4 h-4 text-destructive" />
                              </Button>
                            </div>
                          );
                        })}
                      </div>
                    )}
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

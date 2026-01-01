import React, { useState } from 'react';
import { Plus, Search, MoreHorizontal, Pencil, Trash2, Zap, Tag, ToggleLeft, ToggleRight, Code } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Skeleton } from '@/components/ui/skeleton';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
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
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import {
  useEventsQuery,
  usePredefinedEventsQuery,
  useCustomEventsQuery,
  useCreateEventMutation,
  useUpdateEventMutation,
  useDeleteEventMutation,
} from '@/services/queries/events';
import type { TriggerEvent, CreateTriggerEventData, TriggerEventProperty } from '@/services/api/types';

const eventCategories = [
  { value: 'commerce', label: 'Commerce', icon: '🛒' },
  { value: 'user', label: 'User Activity', icon: '👤' },
  { value: 'social', label: 'Social', icon: '🔗' },
  { value: 'engagement', label: 'Engagement', icon: '💪' },
  { value: 'subscription', label: 'Subscription', icon: '📦' },
  { value: 'custom', label: 'Custom', icon: '⚡' },
];

const propertyTypes = ['string', 'number', 'boolean', 'array', 'object'] as const;

// Helper to get metadata fields
const getEventIcon = (event: TriggerEvent) => event.metadata?.icon || '⚡';
const getEventCategory = (event: TriggerEvent) => event.metadata?.category || 'custom';
const getEventProperties = (event: TriggerEvent) => event.metadata?.properties || [];

interface EventFormData {
  name: string;
  slug: string;
  description: string;
  icon: string;
  category: string;
  properties: TriggerEventProperty[];
  is_active: boolean;
}

interface EventFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  event?: TriggerEvent | null;
}

function EventFormDialog({ open, onOpenChange, event }: EventFormDialogProps) {
  const { toast } = useToast();
  const createMutation = useCreateEventMutation();
  const updateMutation = useUpdateEventMutation();
  const isEditing = !!event;

  const [formData, setFormData] = useState<EventFormData>({
    name: event?.name || '',
    slug: event?.slug || '',
    description: event?.description || '',
    icon: event ? getEventIcon(event) : '⚡',
    category: event ? getEventCategory(event) : 'custom',
    properties: event ? getEventProperties(event) : [],
    is_active: event?.is_active ?? true,
  });

  const [newProperty, setNewProperty] = useState<TriggerEventProperty>({
    name: '',
    type: 'string',
    required: false,
    description: '',
  });

  React.useEffect(() => {
    if (event) {
      setFormData({
        name: event.name,
        slug: event.slug,
        description: event.description || '',
        icon: getEventIcon(event),
        category: getEventCategory(event),
        properties: getEventProperties(event),
        is_active: event.is_active,
      });
    } else {
      setFormData({
        name: '',
        slug: '',
        description: '',
        icon: '⚡',
        category: 'custom',
        properties: [],
        is_active: true,
      });
    }
  }, [event, open]);

  const generateSlug = (name: string) => {
    return name.toLowerCase().replace(/\s+/g, '_').replace(/[^a-z0-9_]/g, '');
  };

  const handleNameChange = (name: string) => {
    setFormData(prev => ({
      ...prev,
      name,
      slug: !isEditing ? generateSlug(name) : prev.slug,
    }));
  };

  const addProperty = () => {
    if (!newProperty.name) return;
    setFormData(prev => ({
      ...prev,
      properties: [...prev.properties, newProperty],
    }));
    setNewProperty({ name: '', type: 'string', required: false, description: '' });
  };

  const removeProperty = (index: number) => {
    setFormData(prev => ({
      ...prev,
      properties: prev.properties.filter((_, i) => i !== index),
    }));
  };

  const handleSubmit = async () => {
    if (!formData.name) {
      toast({
        title: 'Missing required fields',
        description: 'Please provide a name for the event.',
        variant: 'destructive',
      });
      return;
    }

    const payload: CreateTriggerEventData = {
      name: formData.name,
      slug: formData.slug || undefined,
      description: formData.description || undefined,
      is_active: formData.is_active,
      metadata: {
        icon: formData.icon,
        category: formData.category,
        properties: formData.properties,
      },
    };

    try {
      if (isEditing && event) {
        await updateMutation.mutateAsync({ eventId: event.id, data: payload });
        toast({ title: 'Event updated successfully' });
      } else {
        await createMutation.mutateAsync(payload);
        toast({ title: 'Event created successfully' });
      }
      onOpenChange(false);
    } catch (error) {
      toast({
        title: 'Error',
        description: isEditing ? 'Failed to update event' : 'Failed to create event',
        variant: 'destructive',
      });
    }
  };

  const isPending = createMutation.isPending || updateMutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit Event' : 'Create New Event'}</DialogTitle>
          <DialogDescription>
            {isEditing ? 'Update the trigger event configuration.' : 'Define a new trigger event that can be used in rules.'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* Basic Info */}
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Event Name *</Label>
              <Input
                placeholder="e.g., Purchase Completed"
                value={formData.name}
                onChange={(e) => handleNameChange(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label>Event Slug</Label>
              <Input
                placeholder="e.g., purchase_completed"
                value={formData.slug}
                onChange={(e) => setFormData(prev => ({ ...prev, slug: e.target.value }))}
                disabled={isEditing}
                className={cn(isEditing && "opacity-50")}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label>Description</Label>
            <Textarea
              placeholder="Describe when this event is triggered..."
              value={formData.description}
              onChange={(e) => setFormData(prev => ({ ...prev, description: e.target.value }))}
              rows={2}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Category</Label>
              <Select
                value={formData.category}
                onValueChange={(value) => setFormData(prev => ({ ...prev, category: value }))}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {eventCategories.map(cat => (
                    <SelectItem key={cat.value} value={cat.value}>
                      <div className="flex items-center gap-2">
                        <span>{cat.icon}</span>
                        <span>{cat.label}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Icon</Label>
              <Input
                placeholder="e.g., 🛒"
                value={formData.icon}
                onChange={(e) => setFormData(prev => ({ ...prev, icon: e.target.value }))}
                className="text-center text-lg"
              />
            </div>
          </div>

          {/* Properties */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <Label>Event Properties</Label>
              <Badge variant="secondary">{formData.properties.length} properties</Badge>
            </div>
            
            {formData.properties.length > 0 && (
              <div className="space-y-2">
                {formData.properties.map((prop, index) => (
                  <div key={index} className="flex items-center gap-2 p-3 bg-secondary/50 rounded-lg">
                    <Code className="w-4 h-4 text-muted-foreground" />
                    <code className="text-sm font-medium">{prop.name}</code>
                    <Badge variant="outline" className="text-xs">{prop.type}</Badge>
                    {prop.required && <Badge variant="destructive" className="text-xs">Required</Badge>}
                    <span className="flex-1 text-sm text-muted-foreground truncate">{prop.description}</span>
                    <Button variant="ghost" size="icon" className="h-6 w-6" onClick={() => removeProperty(index)}>
                      <Trash2 className="w-3 h-3" />
                    </Button>
                  </div>
                ))}
              </div>
            )}

            <div className="p-4 border border-dashed rounded-lg space-y-3">
              <p className="text-sm font-medium">Add New Property</p>
              <div className="grid grid-cols-4 gap-2">
                <Input
                  placeholder="Property name"
                  value={newProperty.name}
                  onChange={(e) => setNewProperty(prev => ({ ...prev, name: e.target.value }))}
                />
                <Select
                  value={newProperty.type}
                  onValueChange={(value: typeof propertyTypes[number]) => setNewProperty(prev => ({ ...prev, type: value }))}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {propertyTypes.map(type => (
                      <SelectItem key={type} value={type}>{type}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <div className="flex items-center gap-2">
                  <Switch
                    checked={newProperty.required}
                    onCheckedChange={(checked) => setNewProperty(prev => ({ ...prev, required: checked }))}
                  />
                  <span className="text-sm">Required</span>
                </div>
                <Button onClick={addProperty} disabled={!newProperty.name}>
                  <Plus className="w-4 h-4 mr-1" />
                  Add
                </Button>
              </div>
              <Input
                placeholder="Property description (optional)"
                value={newProperty.description}
                onChange={(e) => setNewProperty(prev => ({ ...prev, description: e.target.value }))}
              />
            </div>
          </div>

          {/* Active Status */}
          <div className="flex items-center justify-between p-4 bg-secondary/30 rounded-lg">
            <div>
              <p className="font-medium">Active Status</p>
              <p className="text-sm text-muted-foreground">Enable or disable this event</p>
            </div>
            <Switch
              checked={formData.is_active}
              onCheckedChange={(checked) => setFormData(prev => ({ ...prev, is_active: checked }))}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={handleSubmit} disabled={isPending}>
            {isPending ? 'Saving...' : isEditing ? 'Update Event' : 'Create Event'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function Events() {
  const { toast } = useToast();
  const [searchQuery, setSearchQuery] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('all');
  const [activeTab, setActiveTab] = useState<'all' | 'predefined' | 'custom'>('all');
  const [formDialogOpen, setFormDialogOpen] = useState(false);
  const [selectedEvent, setSelectedEvent] = useState<TriggerEvent | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [eventToDelete, setEventToDelete] = useState<TriggerEvent | null>(null);

  // Fetch events based on active tab
  const { data: allEventsData, isLoading: allLoading } = useEventsQuery({
    search: searchQuery || undefined,
  });
  const { data: predefinedEventsData, isLoading: predefinedLoading } = usePredefinedEventsQuery();
  const { data: customEventsData, isLoading: customLoading } = useCustomEventsQuery();

  const updateMutation = useUpdateEventMutation();
  const deleteMutation = useDeleteEventMutation();

  // Get the right events based on tab
  const getEvents = () => {
    let events: TriggerEvent[] = [];
    switch (activeTab) {
      case 'predefined':
        events = predefinedEventsData || [];
        break;
      case 'custom':
        events = customEventsData || [];
        break;
      default:
        events = allEventsData || [];
    }
    
    // Apply category filter
    if (categoryFilter !== 'all') {
      events = events.filter(e => getEventCategory(e) === categoryFilter);
    }
    
    // Apply search filter for non-API search tabs
    if (searchQuery && activeTab !== 'all') {
      const query = searchQuery.toLowerCase();
      events = events.filter(e => 
        e.name.toLowerCase().includes(query) || 
        e.slug.toLowerCase().includes(query) ||
        e.description?.toLowerCase().includes(query)
      );
    }
    
    return events;
  };

  const events = getEvents();
  const isLoading = activeTab === 'all' ? allLoading : activeTab === 'predefined' ? predefinedLoading : customLoading;

  const handleEdit = (event: TriggerEvent) => {
    setSelectedEvent(event);
    setFormDialogOpen(true);
  };

  const handleCreate = () => {
    setSelectedEvent(null);
    setFormDialogOpen(true);
  };

  const handleToggleActive = async (event: TriggerEvent) => {
    try {
      await updateMutation.mutateAsync({
        eventId: event.id,
        data: { is_active: !event.is_active },
      });
      toast({
        title: event.is_active ? 'Event deactivated' : 'Event activated',
      });
    } catch (error) {
      toast({
        title: 'Error',
        description: 'Failed to update event status',
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async () => {
    if (!eventToDelete) return;
    try {
      await deleteMutation.mutateAsync(eventToDelete.id);
      toast({ title: 'Event deleted successfully' });
      setDeleteDialogOpen(false);
      setEventToDelete(null);
    } catch (error) {
      toast({
        title: 'Error',
        description: 'Failed to delete event',
        variant: 'destructive',
      });
    }
  };

  const getCategoryInfo = (category?: string) => {
    return eventCategories.find(c => c.value === category) || { value: 'custom', label: 'Custom', icon: '⚡' };
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Trigger Events</h1>
          <p className="text-muted-foreground mt-1">Define and manage events that trigger your gamification rules.</p>
        </div>
        <Button variant="glow" onClick={handleCreate}>
          <Plus className="w-4 h-4" />
          Create Event
        </Button>
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as typeof activeTab)}>
        <TabsList>
          <TabsTrigger value="all">All Events</TabsTrigger>
          <TabsTrigger value="predefined">Predefined</TabsTrigger>
          <TabsTrigger value="custom">Custom</TabsTrigger>
        </TabsList>
      </Tabs>

      {/* Search and Filter */}
      <Card>
        <CardContent className="p-4">
          <div className="flex flex-col sm:flex-row gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder="Search events..."
                className="pl-9"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
            <Select value={categoryFilter} onValueChange={setCategoryFilter}>
              <SelectTrigger className="w-full sm:w-[180px]">
                <SelectValue placeholder="Filter by category" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Categories</SelectItem>
                {eventCategories.map(cat => (
                  <SelectItem key={cat.value} value={cat.value}>
                    <div className="flex items-center gap-2">
                      <span>{cat.icon}</span>
                      <span>{cat.label}</span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Events Grid */}
      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(6)].map((_, i) => (
            <Card key={i}>
              <CardContent className="p-6">
                <Skeleton className="h-12 w-12 rounded-lg mb-4" />
                <Skeleton className="h-5 w-3/4 mb-2" />
                <Skeleton className="h-4 w-full mb-4" />
                <Skeleton className="h-4 w-1/2" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : events.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center mx-auto mb-4">
              <Zap className="w-8 h-8 text-muted-foreground" />
            </div>
            <h3 className="text-lg font-medium mb-2">No events found</h3>
            <p className="text-muted-foreground mb-4">Create your first trigger event to start building rules.</p>
            <Button onClick={handleCreate}>
              <Plus className="w-4 h-4 mr-2" />
              Create Event
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {events.map((event) => {
            const categoryInfo = getCategoryInfo(getEventCategory(event));
            const properties = getEventProperties(event);
            return (
              <Card key={event.id} className={cn(
                "relative transition-all hover:shadow-md",
                !event.is_active && "opacity-60"
              )}>
                <CardContent className="p-6">
                  <div className="flex items-start justify-between mb-4">
                    <div className="w-12 h-12 rounded-lg bg-primary/10 flex items-center justify-center text-2xl">
                      {getEventIcon(event)}
                    </div>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8">
                          <MoreHorizontal className="w-4 h-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => handleEdit(event)}>
                          <Pencil className="w-4 h-4 mr-2" />
                          Edit
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => handleToggleActive(event)}>
                          {event.is_active ? (
                            <>
                              <ToggleLeft className="w-4 h-4 mr-2" />
                              Deactivate
                            </>
                          ) : (
                            <>
                              <ToggleRight className="w-4 h-4 mr-2" />
                              Activate
                            </>
                          )}
                        </DropdownMenuItem>
                        {!event.is_predefined && (
                          <DropdownMenuItem
                            className="text-destructive"
                            onClick={() => {
                              setEventToDelete(event);
                              setDeleteDialogOpen(true);
                            }}
                          >
                            <Trash2 className="w-4 h-4 mr-2" />
                            Delete
                          </DropdownMenuItem>
                        )}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>

                  <div className="space-y-2">
                    <div className="flex items-center gap-2">
                      <h3 className="font-semibold">{event.name}</h3>
                      {event.is_predefined && (
                        <Badge variant="secondary" className="text-xs">System</Badge>
                      )}
                    </div>
                    <p className="text-sm text-muted-foreground line-clamp-2">
                      {event.description || 'No description provided'}
                    </p>
                  </div>

                  <div className="mt-4 pt-4 border-t flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Badge variant="outline" className="text-xs">
                        {categoryInfo.icon} {categoryInfo.label}
                      </Badge>
                      <code className="text-xs text-muted-foreground bg-secondary px-1.5 py-0.5 rounded">
                        {event.slug}
                      </code>
                    </div>
                    {properties.length > 0 && (
                      <Badge variant="secondary" className="text-xs">
                        {properties.length} props
                      </Badge>
                    )}
                  </div>

                  {!event.is_predefined && (
                    <div className="absolute top-3 left-3">
                      <div className={cn(
                        "w-2 h-2 rounded-full",
                        event.is_active ? "bg-green-500" : "bg-muted"
                      )} />
                    </div>
                  )}
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Form Dialog */}
      <EventFormDialog
        open={formDialogOpen}
        onOpenChange={setFormDialogOpen}
        event={selectedEvent}
      />

      {/* Delete Confirmation */}
      <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Event</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete "{eventToDelete?.name}"? This action cannot be undone.
              Any rules using this event will stop working.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

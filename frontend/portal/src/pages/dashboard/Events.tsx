import React, { useMemo, useState } from 'react';
import { Plus, Search, MoreHorizontal, Pencil, Trash2, Zap, ToggleLeft, ToggleRight, Lock, Globe } from 'lucide-react';
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
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
import { useAuth } from '@/contexts/AuthContext';
import { cn } from '@/lib/utils';
import {
  useEventsQuery,
  useEventCategoriesQuery,
  useCreateEventMutation,
  useUpdateEventMutation,
  useDeleteEventMutation,
} from '@/services/queries/events';
import { ApiRequestError } from '@/services/queries/rules';
import type {
  EventType,
  EventCategory,
  EventPropertySchema,
  CreateEventTypeData,
  UpdateEventTypeData,
} from '@/services/api/types';
import ActivityLog from '@/components/ActivityLog';

const categoryIcons: Record<string, string> = {
  account: '👤',
  commerce: '🛒',
  engagement: '💪',
  social: '🔗',
  gamification: '🏆',
};

const propertyCount = (event: EventType) => Object.keys(event.property_schema?.properties ?? {}).length;

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiRequestError) {
    if (err.code === 'global_event_type_read_only') return 'Platform event types are read-only.';
    if (err.validationErrors) {
      return Object.entries(err.validationErrors)
        .map(([field, msgs]) => `${field}: ${msgs.join(', ')}`)
        .join('\n');
    }
    return err.message;
  }
  return err instanceof Error ? err.message : fallback;
}

interface EventFormData {
  name: string;
  slug: string;
  description: string;
  category_id: string;
  is_active: boolean;
  property_schema: string;
}

interface EventFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  event?: EventType | null;
  categories: EventCategory[];
}

const NO_CATEGORY = 'none';

function toForm(event?: EventType | null): EventFormData {
  return {
    name: event?.name ?? '',
    slug: event?.slug ?? '',
    description: event?.description ?? '',
    category_id: event?.category_id ?? NO_CATEGORY,
    is_active: event?.is_active ?? true,
    property_schema: event?.property_schema ? JSON.stringify(event.property_schema, null, 2) : '',
  };
}

function EventFormDialog({ open, onOpenChange, event, categories }: EventFormDialogProps) {
  const { toast } = useToast();
  const createMutation = useCreateEventMutation();
  const updateMutation = useUpdateEventMutation();
  const isEditing = !!event;
  const [formData, setFormData] = useState<EventFormData>(toForm(event));
  const [error, setError] = useState<string | null>(null);

  React.useEffect(() => {
    setFormData(toForm(event));
    setError(null);
  }, [event, open]);

  const generateSlug = (name: string) => name.toLowerCase().trim().replace(/\s+/g, '_').replace(/[^a-z0-9_]/g, '');

  const handleNameChange = (name: string) => {
    setFormData(prev => ({ ...prev, name, slug: !isEditing ? generateSlug(name) : prev.slug }));
  };

  const handleSubmit = async () => {
    setError(null);
    if (!formData.name.trim()) {
      setError('Please provide a name for the event type.');
      return;
    }
    let schema: EventPropertySchema | null = null;
    if (formData.property_schema.trim()) {
      try {
        const parsed = JSON.parse(formData.property_schema);
        if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) throw new Error();
        schema = parsed;
      } catch {
        setError('Property schema must be a JSON object, e.g. {"type":"object","properties":{"amount":{"type":"number"}}}.');
        return;
      }
    }
    const categoryId = formData.category_id === NO_CATEGORY ? null : formData.category_id;

    try {
      if (isEditing && event) {
        const original = toForm(event);
        const patch: UpdateEventTypeData = {};
        if (formData.name !== original.name) patch.name = formData.name.trim();
        if (formData.description !== original.description) patch.description = formData.description;
        if (formData.category_id !== original.category_id) patch.category_id = categoryId;
        if (formData.is_active !== original.is_active) patch.is_active = formData.is_active;
        if (formData.property_schema.trim() !== original.property_schema.trim()) patch.property_schema = schema;
        if (Object.keys(patch).length > 0) {
          await updateMutation.mutateAsync({ eventId: event.id, data: patch });
        }
        toast({ title: 'Event type updated' });
      } else {
        const payload: CreateEventTypeData = {
          name: formData.name.trim(),
          slug: formData.slug || undefined,
          description: formData.description || undefined,
          category_id: categoryId ?? undefined,
          is_active: formData.is_active,
          property_schema: schema ?? undefined,
        };
        await createMutation.mutateAsync(payload);
        toast({ title: 'Event type created' });
      }
      onOpenChange(false);
    } catch (err) {
      setError(errorMessage(err, isEditing ? 'Failed to update event type' : 'Failed to create event type'));
    }
  };

  const isPending = createMutation.isPending || updateMutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit Event Type' : 'Create Event Type'}</DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Update this event type. The slug cannot change once created.'
              : 'Define an event type your systems report as activities and rules trigger on.'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Name *</Label>
              <Input
                placeholder="e.g., Purchase Completed"
                value={formData.name}
                onChange={e => handleNameChange(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label>Slug</Label>
              <Input
                placeholder="e.g., purchase_completed"
                value={formData.slug}
                onChange={e => setFormData(prev => ({ ...prev, slug: e.target.value }))}
                disabled={isEditing}
                className={cn(isEditing && 'opacity-50')}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label>Description</Label>
            <Textarea
              placeholder="Describe when this event is reported..."
              value={formData.description}
              onChange={e => setFormData(prev => ({ ...prev, description: e.target.value }))}
              rows={2}
            />
          </div>

          <div className="space-y-2">
            <Label>Category</Label>
            <Select value={formData.category_id} onValueChange={v => setFormData(prev => ({ ...prev, category_id: v }))}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_CATEGORY}>No category</SelectItem>
                {categories.map(c => (
                  <SelectItem key={c.id} value={c.id}>
                    {categoryIcons[c.slug] ?? '⚡'} {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>Property schema (optional JSON Schema)</Label>
            <Textarea
              placeholder='{"type":"object","properties":{"amount":{"type":"number"}},"required":["amount"]}'
              value={formData.property_schema}
              onChange={e => setFormData(prev => ({ ...prev, property_schema: e.target.value }))}
              rows={4}
              className="font-mono text-xs"
            />
            <p className="text-xs text-muted-foreground">
              Declared properties are suggested as condition fields in the rule builder.
            </p>
          </div>

          <div className="flex items-center justify-between p-4 bg-secondary/30 rounded-lg">
            <div>
              <p className="font-medium">Active</p>
              <p className="text-sm text-muted-foreground">Inactive event types are hidden from rule triggers.</p>
            </div>
            <Switch
              checked={formData.is_active}
              onCheckedChange={checked => setFormData(prev => ({ ...prev, is_active: checked }))}
            />
          </div>

          {error && <p className="text-sm text-destructive whitespace-pre-line">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={isPending}>
            {isPending ? 'Saving...' : isEditing ? 'Update Event Type' : 'Create Event Type'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function Events() {
  const { toast } = useToast();
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:rules');
  const [tab, setTab] = useState<'types' | 'activities'>('types');
  const [searchQuery, setSearchQuery] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('all');
  const [scope, setScope] = useState<'all' | 'global' | 'custom'>('all');
  const [formDialogOpen, setFormDialogOpen] = useState(false);
  const [selectedEvent, setSelectedEvent] = useState<EventType | null>(null);
  const [eventToDelete, setEventToDelete] = useState<EventType | null>(null);

  const { data: allEvents = [], isLoading, error } = useEventsQuery();
  const { data: categories = [] } = useEventCategoriesQuery();
  const updateMutation = useUpdateEventMutation();
  const deleteMutation = useDeleteEventMutation();

  const categoryById = useMemo(() => new Map(categories.map(c => [c.id, c])), [categories]);

  const events = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    return allEvents.filter(e => {
      if (scope === 'global' && !e.is_global) return false;
      if (scope === 'custom' && e.is_global) return false;
      if (categoryFilter !== 'all' && e.category_id !== categoryFilter) return false;
      if (!q) return true;
      return (
        e.name.toLowerCase().includes(q) ||
        e.slug.toLowerCase().includes(q) ||
        (e.description ?? '').toLowerCase().includes(q)
      );
    });
  }, [allEvents, scope, categoryFilter, searchQuery]);

  const handleEdit = (event: EventType) => {
    setSelectedEvent(event);
    setFormDialogOpen(true);
  };

  const handleCreate = () => {
    setSelectedEvent(null);
    setFormDialogOpen(true);
  };

  const handleToggleActive = async (event: EventType) => {
    try {
      await updateMutation.mutateAsync({ eventId: event.id, data: { is_active: !event.is_active } });
      toast({ title: event.is_active ? 'Event type deactivated' : 'Event type activated' });
    } catch (err) {
      toast({ title: 'Error', description: errorMessage(err, 'Failed to update event type'), variant: 'destructive' });
    }
  };

  const handleDelete = async () => {
    if (!eventToDelete) return;
    try {
      await deleteMutation.mutateAsync(eventToDelete.id);
      toast({ title: 'Event type deleted' });
      setEventToDelete(null);
    } catch (err) {
      toast({ title: 'Error', description: errorMessage(err, 'Failed to delete event type'), variant: 'destructive' });
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Events</h1>
          <p className="text-muted-foreground mt-1">
            The event types rules trigger on, and the activities your systems report.
          </p>
        </div>
        {tab === 'types' && canManage && (
          <Button variant="glow" onClick={handleCreate}>
            <Plus className="w-4 h-4" />
            Create Event Type
          </Button>
        )}
      </div>

      <Tabs value={tab} onValueChange={v => setTab(v as typeof tab)} className="space-y-6">
        <TabsList>
          <TabsTrigger value="types">Event Types</TabsTrigger>
          <TabsTrigger value="activities">Activity Log</TabsTrigger>
        </TabsList>

        <TabsContent value="types" className="space-y-6">
          <Card>
            <CardContent className="p-4">
              <div className="flex flex-col sm:flex-row gap-4">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                  <Input
                    placeholder="Search event types..."
                    className="pl-9"
                    value={searchQuery}
                    onChange={e => setSearchQuery(e.target.value)}
                  />
                </div>
                <Select value={scope} onValueChange={v => setScope(v as typeof scope)}>
                  <SelectTrigger className="w-full sm:w-[160px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All types</SelectItem>
                    <SelectItem value="global">Platform</SelectItem>
                    <SelectItem value="custom">Custom</SelectItem>
                  </SelectContent>
                </Select>
                <Select value={categoryFilter} onValueChange={setCategoryFilter}>
                  <SelectTrigger className="w-full sm:w-[180px]">
                    <SelectValue placeholder="Filter by category" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Categories</SelectItem>
                    {categories.map(cat => (
                      <SelectItem key={cat.id} value={cat.id}>
                        <div className="flex items-center gap-2">
                          <span>{categoryIcons[cat.slug] ?? '⚡'}</span>
                          <span>{cat.name}</span>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>

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
          ) : error ? (
            <Card>
              <CardContent className="p-12 text-center text-destructive">{error.message}</CardContent>
            </Card>
          ) : events.length === 0 ? (
            <Card>
              <CardContent className="p-12 text-center">
                <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center mx-auto mb-4">
                  <Zap className="w-8 h-8 text-muted-foreground" />
                </div>
                <h3 className="text-lg font-medium mb-2">No event types found</h3>
                <p className="text-muted-foreground mb-4">Create a custom event type to start building rules.</p>
                {canManage && (
                  <Button onClick={handleCreate}>
                    <Plus className="w-4 h-4 mr-2" />
                    Create Event Type
                  </Button>
                )}
              </CardContent>
            </Card>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {events.map(event => {
                const category = event.category_id ? categoryById.get(event.category_id) : undefined;
                const props = propertyCount(event);
                const editable = canManage && !event.is_global;
                return (
                  <Card
                    key={event.id}
                    className={cn('relative transition-all hover:shadow-md', !event.is_active && 'opacity-60')}
                  >
                    <CardContent className="p-6">
                      <div className="flex items-start justify-between mb-4">
                        <div className="w-12 h-12 rounded-lg bg-primary/10 flex items-center justify-center text-2xl">
                          {category ? (categoryIcons[category.slug] ?? '⚡') : '⚡'}
                        </div>
                        {editable ? (
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
                              <DropdownMenuItem className="text-destructive" onClick={() => setEventToDelete(event)}>
                                <Trash2 className="w-4 h-4 mr-2" />
                                Delete
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        ) : event.is_global ? (
                          <span title="Platform event types are read-only" className="text-muted-foreground p-2">
                            <Lock className="w-4 h-4" />
                          </span>
                        ) : null}
                      </div>

                      <div className="space-y-2">
                        <div className="flex items-center gap-2">
                          <h3 className="font-semibold">{event.name}</h3>
                          {event.is_global && (
                            <Badge variant="secondary" className="text-xs gap-1">
                              <Globe className="w-3 h-3" />
                              Platform
                            </Badge>
                          )}
                          {!event.is_active && (
                            <Badge variant="outline" className="text-xs">
                              Inactive
                            </Badge>
                          )}
                        </div>
                        <p className="text-sm text-muted-foreground line-clamp-2">
                          {event.description || 'No description provided'}
                        </p>
                      </div>

                      <div className="mt-4 pt-4 border-t flex items-center justify-between gap-2">
                        <div className="flex items-center gap-2 min-w-0">
                          {category && (
                            <Badge variant="outline" className="text-xs shrink-0">
                              {category.name}
                            </Badge>
                          )}
                          <code className="text-xs text-muted-foreground bg-secondary px-1.5 py-0.5 rounded truncate">
                            {event.slug}
                          </code>
                        </div>
                        {props > 0 && (
                          <Badge variant="secondary" className="text-xs shrink-0">
                            {props} props
                          </Badge>
                        )}
                      </div>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )}
        </TabsContent>

        <TabsContent value="activities">
          <ActivityLog canSend={canManage} />
        </TabsContent>
      </Tabs>

      <EventFormDialog
        open={formDialogOpen}
        onOpenChange={setFormDialogOpen}
        event={selectedEvent}
        categories={categories}
      />

      <AlertDialog open={!!eventToDelete} onOpenChange={o => !o && setEventToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Event Type</AlertDialogTitle>
            <AlertDialogDescription>
              Delete "{eventToDelete?.name}"? Rules triggered by <code>{eventToDelete?.slug}</code> will no longer match new
              activities.
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

import { useEffect, useMemo, useState } from 'react';
import { Plus, Search, MoreHorizontal, Pencil, Trash2, Zap, ToggleLeft, ToggleRight, Globe, Wand2 } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Skeleton } from '@/components/ui/skeleton';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
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
import CursorPager from '@/components/CursorPager';
import { useToast } from '@/hooks/use-toast';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { cn } from '@/lib/utils';
import {
  usePlatformEventTypesQuery,
  usePlatformEventCategoriesQuery,
  useCreatePlatformEventTypeMutation,
  useUpdatePlatformEventTypeMutation,
  useDeletePlatformEventTypeMutation,
} from '@/services/queries/platform';
import type {
  PlatformEventType,
  PlatformEventCategory,
  PlatformCreateEventTypeData,
  PlatformUpdateEventTypeData,
} from '@/services/api/models/platform';
import { FieldError, PlatformGuard } from './PlatformAccess';
import { apiErrorMessage, formatDate, parsePropertySchema, splitApiErrors, type FieldErrors } from './platform-utils';

const NO_CATEGORY = 'none';
const ALL = 'all';
const FORM_FIELDS = ['name', 'slug', 'description', 'category_id', 'property_schema', 'is_active'] as const;

const SCHEMA_TEMPLATE = JSON.stringify(
  {
    type: 'object',
    properties: { amount: { type: 'number', description: 'Order total' } },
    required: ['amount'],
  },
  null,
  2,
);

interface EventTypeForm {
  name: string;
  slug: string;
  description: string;
  category_id: string;
  is_active: boolean;
  property_schema: string;
}

function toForm(eventType?: PlatformEventType | null): EventTypeForm {
  return {
    name: eventType?.name ?? '',
    slug: eventType?.slug ?? '',
    description: eventType?.description ?? '',
    category_id: eventType?.category_id ?? NO_CATEGORY,
    is_active: eventType?.is_active ?? true,
    property_schema: eventType?.property_schema ? JSON.stringify(eventType.property_schema, null, 2) : '',
  };
}

const toSlug = (name: string) =>
  name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '');

const propertyCount = (et: PlatformEventType) => Object.keys(et.property_schema?.properties ?? {}).length;

interface EventTypeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  eventType: PlatformEventType | null;
  categories: PlatformEventCategory[];
}

function EventTypeDialog({ open, onOpenChange, eventType, categories }: EventTypeDialogProps) {
  const { toast } = useToast();
  const createMutation = useCreatePlatformEventTypeMutation();
  const updateMutation = useUpdatePlatformEventTypeMutation();
  const isEditing = !!eventType;
  const [form, setForm] = useState<EventTypeForm>(() => toForm(eventType));
  const [slugTouched, setSlugTouched] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setForm(toForm(eventType));
    setSlugTouched(false);
    setFieldErrors({});
    setFormError(null);
  }, [eventType, open]);

  const schemaCheck = useMemo(() => parsePropertySchema(form.property_schema), [form.property_schema]);
  const liveSchemaErrors = schemaCheck.ok ? undefined : schemaCheck.errors;

  const update = <K extends keyof EventTypeForm>(key: K, value: EventTypeForm[K]) => {
    setForm(prev => ({ ...prev, [key]: value }));
    setFieldErrors(prev => {
      if (!prev[key]) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  };

  const handleNameChange = (name: string) => {
    update('name', name);
    if (!isEditing && !slugTouched) setForm(prev => ({ ...prev, slug: toSlug(name) }));
  };

  const formatSchema = () => {
    if (schemaCheck.ok && schemaCheck.schema) update('property_schema', JSON.stringify(schemaCheck.schema, null, 2));
  };

  const handleSubmit = async () => {
    setFormError(null);
    const errors: FieldErrors = {};
    if (!form.name.trim()) errors.name = ['Name is required.'];
    if (form.name.length > 255) errors.name = ['Name must be at most 255 characters.'];
    if (!isEditing && form.slug.length > 100) errors.slug = ['Slug must be at most 100 characters.'];
    if (form.description.length > 1000) errors.description = ['Description must be at most 1000 characters.'];
    if (!schemaCheck.ok) errors.property_schema = schemaCheck.errors;
    if (Object.keys(errors).length) {
      setFieldErrors(errors);
      return;
    }
    const schema = schemaCheck.ok ? schemaCheck.schema : null;
    const categoryId = form.category_id === NO_CATEGORY ? null : form.category_id;

    try {
      if (isEditing && eventType) {
        const original = toForm(eventType);
        const patch: PlatformUpdateEventTypeData = {};
        if (form.name.trim() !== original.name) patch.name = form.name.trim();
        if (form.description !== original.description) patch.description = form.description;
        if (form.category_id !== original.category_id) patch.category_id = categoryId;
        if (form.is_active !== original.is_active) patch.is_active = form.is_active;
        if (JSON.stringify(schema) !== JSON.stringify(eventType.property_schema ?? null)) patch.property_schema = schema;
        if (Object.keys(patch).length > 0) {
          await updateMutation.mutateAsync({ eventTypeId: eventType.id, data: patch });
        }
        toast({ title: 'Global event type updated' });
      } else {
        const payload: PlatformCreateEventTypeData = {
          name: form.name.trim(),
          slug: form.slug.trim() || undefined,
          description: form.description || undefined,
          category_id: categoryId ?? undefined,
          is_active: form.is_active,
          property_schema: schema ?? undefined,
        };
        await createMutation.mutateAsync(payload);
        toast({ title: 'Global event type created' });
      }
      onOpenChange(false);
    } catch (err) {
      const split = splitApiErrors(err, FORM_FIELDS, isEditing ? 'Failed to update event type' : 'Failed to create event type');
      setFieldErrors(split.fields);
      setFormError(split.form);
    }
  };

  const isPending = createMutation.isPending || updateMutation.isPending;
  const schemaErrors = fieldErrors.property_schema ?? (form.property_schema.trim() ? liveSchemaErrors : undefined);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit Global Event Type' : 'Create Global Event Type'}</DialogTitle>
          <DialogDescription>
            Global event types are visible to every tenant and read-only for them.
            {isEditing && ' The slug cannot change once created.'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="pet-name">Name *</Label>
              <Input
                id="pet-name"
                placeholder="e.g., Purchase Completed"
                value={form.name}
                onChange={e => handleNameChange(e.target.value)}
                aria-invalid={!!fieldErrors.name}
                className={cn(fieldErrors.name && 'border-destructive')}
              />
              <FieldError messages={fieldErrors.name} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pet-slug">Slug</Label>
              <Input
                id="pet-slug"
                placeholder="e.g., purchase_completed"
                value={form.slug}
                onChange={e => {
                  setSlugTouched(true);
                  update('slug', e.target.value);
                }}
                disabled={isEditing}
                aria-invalid={!!fieldErrors.slug}
                className={cn('font-mono', isEditing && 'opacity-50', fieldErrors.slug && 'border-destructive')}
              />
              <FieldError messages={fieldErrors.slug} />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="pet-description">Description</Label>
            <Textarea
              id="pet-description"
              placeholder="Describe when this event is reported..."
              value={form.description}
              onChange={e => update('description', e.target.value)}
              rows={2}
              aria-invalid={!!fieldErrors.description}
              className={cn(fieldErrors.description && 'border-destructive')}
            />
            <FieldError messages={fieldErrors.description} />
          </div>

          <div className="space-y-2">
            <Label>Category</Label>
            <Select value={form.category_id} onValueChange={v => update('category_id', v)}>
              <SelectTrigger className={cn(fieldErrors.category_id && 'border-destructive')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_CATEGORY}>No category</SelectItem>
                {categories.map(c => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldError messages={fieldErrors.category_id} />
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="pet-schema">Property schema (JSON Schema subset)</Label>
              <div className="flex items-center gap-1">
                {!form.property_schema.trim() && (
                  <Button type="button" variant="ghost" size="sm" onClick={() => update('property_schema', SCHEMA_TEMPLATE)}>
                    Insert example
                  </Button>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={formatSchema}
                  disabled={!schemaCheck.ok || !schemaCheck.schema}
                >
                  <Wand2 className="w-3.5 h-3.5" />
                  Format
                </Button>
              </div>
            </div>
            <Textarea
              id="pet-schema"
              placeholder='{"type":"object","properties":{"amount":{"type":"number"}},"required":["amount"]}'
              value={form.property_schema}
              onChange={e => update('property_schema', e.target.value)}
              rows={10}
              spellCheck={false}
              aria-invalid={!!schemaErrors}
              className={cn('font-mono text-xs', schemaErrors && 'border-destructive')}
            />
            {schemaErrors ? (
              <FieldError messages={schemaErrors} />
            ) : (
              <p className="text-xs text-muted-foreground">
                Optional. <code>type</code> must be "object"; each property may declare a <code>type</code>; every name
                in <code>required</code> must be declared. Leave empty for free-form properties.
              </p>
            )}
          </div>

          <div className="flex items-center justify-between p-4 bg-secondary/30 rounded-lg">
            <div>
              <p className="font-medium">Active</p>
              <p className="text-sm text-muted-foreground">Inactive event types are hidden from rule triggers.</p>
            </div>
            <Switch checked={form.is_active} onCheckedChange={checked => update('is_active', checked)} />
          </div>
          <FieldError messages={fieldErrors.is_active} />

          {formError && <p className="text-sm text-destructive whitespace-pre-line">{formError}</p>}
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

function EventTypesContent() {
  const { toast } = useToast();
  const pager = useCursorPagination(24);
  const { reset: resetPager } = pager;
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [categoryFilter, setCategoryFilter] = useState(ALL);
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState<PlatformEventType | null>(null);
  const [toDelete, setToDelete] = useState<PlatformEventType | null>(null);

  useEffect(() => {
    const next = searchInput.trim();
    if (next === search) return;
    const t = setTimeout(() => {
      setSearch(next);
      resetPager();
    }, 300);
    return () => clearTimeout(t);
  }, [searchInput, search, resetPager]);

  const { data, isLoading, isFetching, error } = usePlatformEventTypesQuery({
    limit: pager.limit,
    cursor: pager.cursor,
    search: search || undefined,
    category: categoryFilter === ALL ? undefined : categoryFilter,
    active: statusFilter === 'all' ? undefined : statusFilter === 'active',
  });
  const { data: categories = [] } = usePlatformEventCategoriesQuery();
  const updateMutation = useUpdatePlatformEventTypeMutation();
  const deleteMutation = useDeletePlatformEventTypeMutation();

  const categoryById = useMemo(() => new Map(categories.map(c => [c.id, c])), [categories]);
  const eventTypes = data?.data ?? [];

  const openCreate = () => {
    setSelected(null);
    setDialogOpen(true);
  };

  const openEdit = (et: PlatformEventType) => {
    setSelected(et);
    setDialogOpen(true);
  };

  const handleToggleActive = async (et: PlatformEventType) => {
    try {
      await updateMutation.mutateAsync({ eventTypeId: et.id, data: { is_active: !et.is_active } });
      toast({ title: et.is_active ? 'Event type deactivated' : 'Event type activated' });
    } catch (err) {
      toast({ title: 'Error', description: apiErrorMessage(err, 'Failed to update event type'), variant: 'destructive' });
    }
  };

  const handleDelete = async () => {
    if (!toDelete) return;
    try {
      await deleteMutation.mutateAsync(toDelete.id);
      toast({ title: 'Event type deleted' });
      setToDelete(null);
    } catch (err) {
      toast({ title: 'Error', description: apiErrorMessage(err, 'Failed to delete event type'), variant: 'destructive' });
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Global Event Types</h1>
          <p className="text-muted-foreground mt-1">
            The platform catalogue every tenant can trigger rules on. Tenants see these as read-only.
          </p>
        </div>
        <Button variant="glow" onClick={openCreate}>
          <Plus className="w-4 h-4" />
          Create Event Type
        </Button>
      </div>

      <Card>
        <CardContent className="p-4">
          <div className="flex flex-col sm:flex-row gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder="Search name, slug or description..."
                className="pl-9"
                value={searchInput}
                onChange={e => setSearchInput(e.target.value)}
              />
            </div>
            <Select
              value={statusFilter}
              onValueChange={v => {
                setStatusFilter(v as typeof statusFilter);
                pager.reset();
              }}
            >
              <SelectTrigger className="w-full sm:w-[160px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All statuses</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="inactive">Inactive</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={categoryFilter}
              onValueChange={v => {
                setCategoryFilter(v);
                pager.reset();
              }}
            >
              <SelectTrigger className="w-full sm:w-[180px]">
                <SelectValue placeholder="Filter by category" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All Categories</SelectItem>
                {categories.map(c => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
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
          <CardContent className="p-12 text-center text-destructive">
            {apiErrorMessage(error, 'Failed to fetch event types')}
          </CardContent>
        </Card>
      ) : eventTypes.length === 0 ? (
        <Card>
          <CardContent className="p-12 text-center">
            <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center mx-auto mb-4">
              <Zap className="w-8 h-8 text-muted-foreground" />
            </div>
            <h3 className="text-lg font-medium mb-2">No global event types found</h3>
            <p className="text-muted-foreground mb-4">
              {search || categoryFilter !== ALL || statusFilter !== 'all'
                ? 'Try a different search or filter.'
                : 'Create the first platform-wide event type.'}
            </p>
            <Button onClick={openCreate}>
              <Plus className="w-4 h-4 mr-2" />
              Create Event Type
            </Button>
          </CardContent>
        </Card>
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {eventTypes.map(et => {
              const category = et.category_id ? categoryById.get(et.category_id) : undefined;
              const props = propertyCount(et);
              return (
                <Card key={et.id} className={cn('relative transition-all hover:shadow-md', !et.is_active && 'opacity-60')}>
                  <CardContent className="p-6">
                    <div className="flex items-start justify-between mb-4">
                      <div className="w-12 h-12 rounded-lg bg-primary/10 flex items-center justify-center">
                        <Zap className="w-6 h-6 text-primary" />
                      </div>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="h-8 w-8">
                            <MoreHorizontal className="w-4 h-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEdit(et)}>
                            <Pencil className="w-4 h-4 mr-2" />
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => handleToggleActive(et)}>
                            {et.is_active ? (
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
                          <DropdownMenuItem className="text-destructive" onClick={() => setToDelete(et)}>
                            <Trash2 className="w-4 h-4 mr-2" />
                            Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>

                    <div className="space-y-2">
                      <div className="flex items-center gap-2 flex-wrap">
                        <h3 className="font-semibold">{et.name}</h3>
                        <Badge variant="secondary" className="text-xs gap-1">
                          <Globe className="w-3 h-3" />
                          Global
                        </Badge>
                        {!et.is_active && (
                          <Badge variant="outline" className="text-xs">
                            Inactive
                          </Badge>
                        )}
                      </div>
                      <p className="text-sm text-muted-foreground line-clamp-2">
                        {et.description || 'No description provided'}
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
                          {et.slug}
                        </code>
                      </div>
                      <div className="flex items-center gap-2 shrink-0">
                        {props > 0 && (
                          <Badge variant="secondary" className="text-xs">
                            {props} props
                          </Badge>
                        )}
                        <span className="text-xs text-muted-foreground" title={et.updated_at}>
                          {formatDate(et.updated_at)}
                        </span>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
          </div>
          <CursorPager
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            nextCursor={data?.next_cursor}
            onPrevious={pager.previous}
            onNext={pager.next}
            isFetching={isFetching}
          />
        </>
      )}

      <EventTypeDialog open={dialogOpen} onOpenChange={setDialogOpen} eventType={selected} categories={categories} />

      <AlertDialog open={!!toDelete} onOpenChange={o => !o && setToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Global Event Type</AlertDialogTitle>
            <AlertDialogDescription>
              Delete "{toDelete?.name}"? It disappears from every tenant's catalogue, and rules triggered by{' '}
              <code>{toDelete?.slug}</code> will no longer match new activities.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={e => {
                e.preventDefault();
                void handleDelete();
              }}
              disabled={deleteMutation.isPending}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

export default function PlatformEventTypes() {
  return (
    <PlatformGuard>
      <EventTypesContent />
    </PlatformGuard>
  );
}

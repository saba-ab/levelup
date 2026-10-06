import { useEffect, useState } from 'react';
import { Plus, Pencil, Trash2, FolderTree, ArrowUp, ArrowDown } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
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
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import {
  usePlatformEventCategoriesQuery,
  useCreatePlatformEventCategoryMutation,
  useUpdatePlatformEventCategoryMutation,
  useDeletePlatformEventCategoryMutation,
} from '@/services/queries/platform';
import type {
  PlatformEventCategory,
  PlatformCreateEventCategoryData,
  PlatformUpdateEventCategoryData,
} from '@/services/api/models/platform';
import { FieldError, PlatformGuard } from './PlatformAccess';
import { apiErrorMessage, formatDate, splitApiErrors, type FieldErrors } from './platform-utils';

const FORM_FIELDS = ['name', 'slug', 'description', 'sort_order'] as const;

interface CategoryForm {
  name: string;
  slug: string;
  description: string;
  sort_order: string;
}

function toForm(category: PlatformEventCategory | null, nextSortOrder: number): CategoryForm {
  return {
    name: category?.name ?? '',
    slug: category?.slug ?? '',
    description: category?.description ?? '',
    sort_order: String(category?.sort_order ?? nextSortOrder),
  };
}

const toSlug = (name: string) =>
  name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '');

interface CategoryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  category: PlatformEventCategory | null;
  nextSortOrder: number;
}

function CategoryDialog({ open, onOpenChange, category, nextSortOrder }: CategoryDialogProps) {
  const { toast } = useToast();
  const createMutation = useCreatePlatformEventCategoryMutation();
  const updateMutation = useUpdatePlatformEventCategoryMutation();
  const isEditing = !!category;
  const [form, setForm] = useState<CategoryForm>(() => toForm(category, nextSortOrder));
  const [slugTouched, setSlugTouched] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setForm(toForm(category, nextSortOrder));
    setSlugTouched(false);
    setFieldErrors({});
    setFormError(null);
  }, [category, nextSortOrder, open]);

  const update = <K extends keyof CategoryForm>(key: K, value: CategoryForm[K]) => {
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

  const handleSubmit = async () => {
    setFormError(null);
    const errors: FieldErrors = {};
    const name = form.name.trim();
    const slug = form.slug.trim();
    const sortOrder = Number(form.sort_order);
    if (!name) errors.name = ['Name is required.'];
    else if (name.length > 255) errors.name = ['Name must be at most 255 characters.'];
    if (isEditing && !slug) errors.slug = ['Slug cannot be empty.'];
    else if (slug.length > 100) errors.slug = ['Slug must be at most 100 characters.'];
    if (form.description.length > 1000) errors.description = ['Description must be at most 1000 characters.'];
    if (form.sort_order.trim() === '' || !Number.isInteger(sortOrder)) errors.sort_order = ['Sort order must be a whole number.'];
    if (Object.keys(errors).length) {
      setFieldErrors(errors);
      return;
    }

    try {
      if (isEditing && category) {
        const patch: PlatformUpdateEventCategoryData = {};
        if (name !== category.name) patch.name = name;
        if (slug !== category.slug) patch.slug = slug;
        if (form.description !== (category.description ?? '')) patch.description = form.description;
        if (sortOrder !== category.sort_order) patch.sort_order = sortOrder;
        if (Object.keys(patch).length > 0) {
          await updateMutation.mutateAsync({ categoryId: category.id, data: patch });
        }
        toast({ title: 'Category updated' });
      } else {
        const payload: PlatformCreateEventCategoryData = {
          name,
          slug: slug || undefined,
          description: form.description || undefined,
          sort_order: sortOrder,
        };
        await createMutation.mutateAsync(payload);
        toast({ title: 'Category created' });
      }
      onOpenChange(false);
    } catch (err) {
      const split = splitApiErrors(err, FORM_FIELDS, isEditing ? 'Failed to update category' : 'Failed to create category');
      setFieldErrors(split.fields);
      setFormError(split.form);
    }
  };

  const isPending = createMutation.isPending || updateMutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit Global Category' : 'Create Global Category'}</DialogTitle>
          <DialogDescription>Global categories group platform event types for every tenant.</DialogDescription>
        </DialogHeader>

        <div className="space-y-5 py-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="pec-name">Name *</Label>
              <Input
                id="pec-name"
                placeholder="e.g., Commerce"
                value={form.name}
                onChange={e => handleNameChange(e.target.value)}
                aria-invalid={!!fieldErrors.name}
                className={cn(fieldErrors.name && 'border-destructive')}
              />
              <FieldError messages={fieldErrors.name} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pec-slug">Slug</Label>
              <Input
                id="pec-slug"
                placeholder="e.g., commerce"
                value={form.slug}
                onChange={e => {
                  setSlugTouched(true);
                  update('slug', e.target.value);
                }}
                aria-invalid={!!fieldErrors.slug}
                className={cn('font-mono', fieldErrors.slug && 'border-destructive')}
              />
              <FieldError messages={fieldErrors.slug} />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="pec-description">Description</Label>
            <Textarea
              id="pec-description"
              placeholder="What kind of events belong here..."
              value={form.description}
              onChange={e => update('description', e.target.value)}
              rows={2}
              aria-invalid={!!fieldErrors.description}
              className={cn(fieldErrors.description && 'border-destructive')}
            />
            <FieldError messages={fieldErrors.description} />
          </div>

          <div className="space-y-2">
            <Label htmlFor="pec-sort">Sort order</Label>
            <Input
              id="pec-sort"
              type="number"
              step={1}
              value={form.sort_order}
              onChange={e => update('sort_order', e.target.value)}
              aria-invalid={!!fieldErrors.sort_order}
              className={cn('w-32', fieldErrors.sort_order && 'border-destructive')}
            />
            {fieldErrors.sort_order ? (
              <FieldError messages={fieldErrors.sort_order} />
            ) : (
              <p className="text-xs text-muted-foreground">Lower numbers are listed first.</p>
            )}
          </div>

          {formError && <p className="text-sm text-destructive whitespace-pre-line">{formError}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={isPending}>
            {isPending ? 'Saving...' : isEditing ? 'Update Category' : 'Create Category'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CategoriesContent() {
  const { toast } = useToast();
  const { data: categories = [], isLoading, error } = usePlatformEventCategoriesQuery();
  const updateMutation = useUpdatePlatformEventCategoryMutation();
  const deleteMutation = useDeletePlatformEventCategoryMutation();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState<PlatformEventCategory | null>(null);
  const [toDelete, setToDelete] = useState<PlatformEventCategory | null>(null);
  const [reordering, setReordering] = useState(false);

  const nextSortOrder = categories.length ? Math.max(...categories.map(c => c.sort_order)) + 10 : 10;

  const openCreate = () => {
    setSelected(null);
    setDialogOpen(true);
  };

  const openEdit = (category: PlatformEventCategory) => {
    setSelected(category);
    setDialogOpen(true);
  };

  /**
   * Swaps a category with its neighbour. Equal sort orders cannot be swapped,
   * so the pair gets distinct values derived from their positions.
   */
  const move = async (index: number, direction: -1 | 1) => {
    const a = categories[index];
    const b = categories[index + direction];
    if (!a || !b) return;
    let aOrder = b.sort_order;
    let bOrder = a.sort_order;
    if (aOrder === bOrder) {
      const base = Math.min(a.sort_order, b.sort_order);
      aOrder = direction === -1 ? base : base + 1;
      bOrder = direction === -1 ? base + 1 : base;
    }
    setReordering(true);
    try {
      await updateMutation.mutateAsync({ categoryId: a.id, data: { sort_order: aOrder } });
      await updateMutation.mutateAsync({ categoryId: b.id, data: { sort_order: bOrder } });
    } catch (err) {
      toast({ title: 'Error', description: apiErrorMessage(err, 'Failed to reorder categories'), variant: 'destructive' });
    } finally {
      setReordering(false);
    }
  };

  const handleDelete = async () => {
    if (!toDelete) return;
    try {
      await deleteMutation.mutateAsync(toDelete.id);
      toast({ title: 'Category deleted' });
      setToDelete(null);
    } catch (err) {
      toast({ title: 'Error', description: apiErrorMessage(err, 'Failed to delete category'), variant: 'destructive' });
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Global Event Categories</h1>
          <p className="text-muted-foreground mt-1">
            Group the platform event catalogue. The order here is the order tenants see.
          </p>
        </div>
        <Button variant="glow" onClick={openCreate}>
          <Plus className="w-4 h-4" />
          Create Category
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-4 space-y-3">
              {[...Array(5)].map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : error ? (
            <div className="p-12 text-center text-destructive">{apiErrorMessage(error, 'Failed to fetch categories')}</div>
          ) : categories.length === 0 ? (
            <div className="p-12 text-center">
              <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center mx-auto mb-4">
                <FolderTree className="w-8 h-8 text-muted-foreground" />
              </div>
              <h3 className="text-lg font-medium mb-2">No global categories</h3>
              <p className="text-muted-foreground mb-4">Create a category to organise platform event types.</p>
              <Button onClick={openCreate}>
                <Plus className="w-4 h-4 mr-2" />
                Create Category
              </Button>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-28">Order</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Slug</TableHead>
                  <TableHead className="hidden md:table-cell">Description</TableHead>
                  <TableHead className="hidden sm:table-cell">Updated</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {categories.map((category, index) => (
                  <TableRow key={category.id}>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <span className="font-mono text-sm w-8 text-muted-foreground">{category.sort_order}</span>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          disabled={index === 0 || reordering}
                          onClick={() => move(index, -1)}
                          title="Move up"
                        >
                          <ArrowUp className="w-3.5 h-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          disabled={index === categories.length - 1 || reordering}
                          onClick={() => move(index, 1)}
                          title="Move down"
                        >
                          <ArrowDown className="w-3.5 h-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell className="font-medium">{category.name}</TableCell>
                    <TableCell>
                      <code className="text-xs text-muted-foreground bg-secondary px-1.5 py-0.5 rounded">{category.slug}</code>
                    </TableCell>
                    <TableCell className="hidden md:table-cell text-sm text-muted-foreground max-w-xs truncate">
                      {category.description || '—'}
                    </TableCell>
                    <TableCell className="hidden sm:table-cell text-sm text-muted-foreground" title={category.updated_at}>
                      {formatDate(category.updated_at)}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openEdit(category)} title="Edit">
                          <Pencil className="w-4 h-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-destructive hover:text-destructive"
                          onClick={() => setToDelete(category)}
                          title="Delete"
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <CategoryDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        category={selected}
        nextSortOrder={nextSortOrder}
      />

      <AlertDialog open={!!toDelete} onOpenChange={o => !o && setToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Global Category</AlertDialogTitle>
            <AlertDialogDescription>
              Delete "{toDelete?.name}"? Event types in it stay, but are no longer grouped under this category.
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

export default function PlatformEventCategories() {
  return (
    <PlatformGuard>
      <CategoriesContent />
    </PlatformGuard>
  );
}

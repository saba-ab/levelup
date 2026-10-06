import { useEffect, useMemo, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useToast } from '@/hooks/use-toast';
import { useAllBadgesQuery } from '@/services/queries/mechanics';
import {
  describeSegmentError,
  useCreateSegmentMutation,
  useUpdateSegmentMutation,
} from '@/services/queries/segments';
import type { Segment, UpdateSegmentData } from '@/services/api/models/segments';
import { ConditionBuilder } from './ConditionBuilder';
import { SegmentPreviewPanel } from './SegmentPreviewPanel';
import { fromConditions, newGroup, toConditions, type EditorGroup } from './conditionModel';

interface SegmentFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Edit this segment; create a new one when omitted. */
  segment?: Segment | null;
  onSaved?: (segment: Segment) => void;
}

export function SegmentFormDialog({ open, onOpenChange, segment, onSaved }: SegmentFormDialogProps) {
  const { toast } = useToast();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [tree, setTree] = useState<EditorGroup>(() => newGroup());
  const [submitError, setSubmitError] = useState<string | null>(null);
  const badgesQuery = useAllBadgesQuery();
  const createMutation = useCreateSegmentMutation();
  const updateMutation = useUpdateSegmentMutation();
  const saving = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (!open) return;
    setName(segment?.name ?? '');
    setDescription(segment?.description ?? '');
    setTree(segment ? fromConditions(segment.conditions) : newGroup());
    setSubmitError(null);
  }, [open, segment]);

  const { conditions, errors } = useMemo(() => toConditions(tree), [tree]);
  const badges = useMemo(
    () => (badgesQuery.data ?? []).map((b) => ({ id: b.id, name: b.name })),
    [badgesQuery.data],
  );

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!conditions) return;
    setSubmitError(null);
    try {
      let saved: Segment;
      if (segment) {
        const patch: UpdateSegmentData = {};
        if (name.trim() !== segment.name) patch.name = name.trim();
        if (description !== segment.description) patch.description = description;
        const original = toConditions(fromConditions(segment.conditions)).conditions;
        if (JSON.stringify(original) !== JSON.stringify(conditions)) patch.conditions = conditions;
        saved = Object.keys(patch).length
          ? await updateMutation.mutateAsync({ id: segment.id, data: patch })
          : segment;
        toast({
          title: 'Segment updated',
          description: patch.conditions ? 'Membership is being recalculated.' : `"${saved.name}" was saved.`,
        });
      } else {
        saved = await createMutation.mutateAsync({
          name: name.trim(),
          description: description.trim() || undefined,
          conditions,
        });
        toast({ title: 'Segment created', description: 'Membership is being calculated in the background.' });
      }
      onOpenChange(false);
      onSaved?.(saved);
    } catch (err) {
      setSubmitError(describeSegmentError(err, 'Failed to save the segment'));
    }
  };

  return (
    <Dialog open={open} onOpenChange={(v) => !saving && onOpenChange(v)}>
      <DialogContent className="max-h-[90vh] max-w-5xl overflow-y-auto">
        <form onSubmit={handleSubmit} className="flex flex-col gap-6">
          <DialogHeader>
            <DialogTitle>{segment ? 'Edit segment' : 'New segment'}</DialogTitle>
            <DialogDescription>
              Define which players belong to this segment. Membership is recalculated in the background after saving.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="flex flex-col gap-2">
              <Label htmlFor="segment-name">Name</Label>
              <Input
                id="segment-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={255}
                required
                placeholder="e.g. High-value players"
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="segment-description">Description</Label>
              <Textarea
                id="segment-description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                maxLength={1000}
                rows={1}
                placeholder="Optional"
              />
            </div>
          </div>

          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
            <div className="flex flex-col gap-2">
              <Label>Conditions</Label>
              <ConditionBuilder
                value={tree}
                onChange={setTree}
                badges={badges}
                badgesLoading={badgesQuery.isLoading}
                disabled={saving}
              />
            </div>
            <SegmentPreviewPanel conditions={conditions} problems={errors} />
          </div>

          {submitError && <p className="whitespace-pre-line text-sm text-destructive">{submitError}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              Cancel
            </Button>
            <Button type="submit" disabled={saving || !conditions || !name.trim()}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {segment ? 'Save changes' : 'Create segment'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

import { useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useToast } from '@/hooks/use-toast';
import type { ValidationErrors } from '@/hooks/useApi';
import { PlayerApiError, useCreatePlayerMutation, useUpdatePlayerMutation } from '@/services/queries/players';
import type { Player, UpdatePlayerData } from '@/services/api/types';

interface PlayerFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Edit this player; omit to create one. */
  player?: Player | null;
  onSaved?: (player: Player) => void;
}

function stringifyAttributes(attrs: Record<string, unknown> | undefined): string {
  return attrs && Object.keys(attrs).length > 0 ? JSON.stringify(attrs, null, 2) : '';
}

/** Create (POST /players) or edit (PATCH /players/{id}, changed fields only). */
export function PlayerFormDialog({ open, onOpenChange, player, onSaved }: PlayerFormDialogProps) {
  const isEdit = !!player;
  const { toast } = useToast();
  const createMutation = useCreatePlayerMutation();
  const updateMutation = useUpdatePlayerMutation();

  const [externalId, setExternalId] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [attributes, setAttributes] = useState('');
  const [errors, setErrors] = useState<ValidationErrors>({});

  useEffect(() => {
    if (!open) return;
    setExternalId(player?.external_id ?? '');
    setDisplayName(player?.display_name ?? '');
    setEmail(player?.email ?? '');
    setAttributes(stringifyAttributes(player?.attributes));
    setErrors({});
  }, [open, player]);

  const isPending = createMutation.isPending || updateMutation.isPending;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrors({});

    let parsedAttributes: Record<string, unknown> | undefined;
    if (attributes.trim()) {
      try {
        const value: unknown = JSON.parse(attributes);
        if (typeof value !== 'object' || value === null || Array.isArray(value)) throw new Error();
        parsedAttributes = value as Record<string, unknown>;
      } catch {
        setErrors({ attributes: ['Attributes must be a JSON object, e.g. {"tier": "vip"}'] });
        return;
      }
    }

    try {
      let saved: Player;
      if (isEdit && player) {
        const data: UpdatePlayerData = {};
        if (displayName !== (player.display_name ?? '')) data.display_name = displayName;
        if (email !== (player.email ?? '')) data.email = email;
        if (stringifyAttributes(parsedAttributes) !== stringifyAttributes(player.attributes)) {
          data.attributes = parsedAttributes ?? {};
        }
        if (Object.keys(data).length === 0) {
          onOpenChange(false);
          return;
        }
        saved = await updateMutation.mutateAsync({ playerId: player.id, data });
        toast({ title: 'Player updated' });
      } else {
        saved = await createMutation.mutateAsync({
          external_id: externalId.trim(),
          display_name: displayName.trim() || undefined,
          email: email.trim() || undefined,
          attributes: parsedAttributes,
        });
        toast({ title: 'Player created' });
      }
      onSaved?.(saved);
      onOpenChange(false);
    } catch (err) {
      if (err instanceof PlayerApiError && err.validationErrors) {
        setErrors(err.validationErrors);
        return;
      }
      toast({
        title: isEdit ? 'Could not update player' : 'Could not create player',
        description: err instanceof Error ? err.message : undefined,
        variant: 'destructive',
      });
    }
  };

  const fieldError = (field: string) =>
    errors[field] ? <p className="text-xs text-destructive">{errors[field].join(', ')}</p> : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{isEdit ? 'Edit Player' : 'New Player'}</DialogTitle>
            <DialogDescription>
              {isEdit ? 'Change the player\'s profile. Only changed fields are sent.' : 'Register a player by the ID your system uses for them.'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="player-external-id">External ID</Label>
              <Input
                id="player-external-id"
                value={externalId}
                onChange={(e) => setExternalId(e.target.value)}
                disabled={isEdit}
                required={!isEdit}
                maxLength={255}
                placeholder="user-123"
              />
              {fieldError('external_id')}
            </div>
            <div className="space-y-2">
              <Label htmlFor="player-display-name">Display name</Label>
              <Input
                id="player-display-name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                maxLength={255}
              />
              {fieldError('display_name')}
            </div>
            <div className="space-y-2">
              <Label htmlFor="player-email">Email (optional)</Label>
              <Input id="player-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} maxLength={255} />
              {fieldError('email')}
            </div>
            <div className="space-y-2">
              <Label htmlFor="player-attributes">Attributes (JSON, optional)</Label>
              <Textarea
                id="player-attributes"
                className="font-mono text-xs"
                rows={4}
                value={attributes}
                onChange={(e) => setAttributes(e.target.value)}
                placeholder='{"country": "GE"}'
              />
              {fieldError('attributes')}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isPending}>
              {isPending ? 'Saving...' : isEdit ? 'Save' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

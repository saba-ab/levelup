import { ReactNode, useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { PlayerPicker } from './PlayerPicker';

interface PlayerActionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: ReactNode;
  submitLabel: string;
  isPending?: boolean;
  /** Called with the chosen player id; throw to keep the dialog open. */
  onSubmit: (playerId: string) => Promise<void>;
  /** Extra fields under the player picker (e.g. an increment). */
  children?: ReactNode;
  destructive?: boolean;
}

/** A dialog that picks a player and runs one player-scoped action. */
export function PlayerActionDialog({
  open,
  onOpenChange,
  title,
  description,
  submitLabel,
  isPending,
  onSubmit,
  children,
  destructive,
}: PlayerActionDialogProps) {
  const [playerId, setPlayerId] = useState('');

  useEffect(() => {
    if (!open) setPlayerId('');
  }, [open]);

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!playerId) return;
    try {
      await onSubmit(playerId);
      onOpenChange(false);
    } catch {
      // the caller reports the error; keep the dialog open to retry
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description && <DialogDescription>{description}</DialogDescription>}
          </DialogHeader>
          <div className="space-y-4 py-4">
            <PlayerPicker value={playerId} onChange={setPlayerId} />
            {children}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" variant={destructive ? 'destructive' : 'glow'} disabled={!playerId || isPending}>
              {isPending && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

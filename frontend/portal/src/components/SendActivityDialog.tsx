import { useEffect, useState } from 'react';
import { Loader2, Send } from 'lucide-react';
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';
import { useEventsQuery } from '@/services/queries/events';
import { useIngestActivityMutation } from '@/services/queries/activities';

interface SendActivityDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultEventType?: string;
  defaultProperties?: string;
}

const newEventId = () =>
  `portal-test-${typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : Date.now()}`;

/**
 * Sends a real activity (POST /activities). Rules evaluate it asynchronously
 * and their effects are applied, so this changes player state.
 */
export default function SendActivityDialog({ open, onOpenChange, defaultEventType, defaultProperties }: SendActivityDialogProps) {
  const { toast } = useToast();
  const { data: eventTypes = [] } = useEventsQuery({ active: true });
  const ingest = useIngestActivityMutation();
  const [eventType, setEventType] = useState('');
  const [playerExternalId, setPlayerExternalId] = useState('');
  const [eventId, setEventId] = useState(newEventId);
  const [properties, setProperties] = useState('{}');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setEventType(defaultEventType ?? '');
      setProperties(defaultProperties ?? '{}');
      setEventId(newEventId());
      setError(null);
    }
  }, [open, defaultEventType, defaultProperties]);

  const submit = async () => {
    setError(null);
    let parsed: Record<string, unknown> = {};
    try {
      const v = properties.trim() ? JSON.parse(properties) : {};
      if (typeof v !== 'object' || v === null || Array.isArray(v)) throw new Error();
      parsed = v;
    } catch {
      setError('Properties must be a JSON object.');
      return;
    }
    try {
      const res = await ingest.mutateAsync({
        event_id: eventId.trim(),
        event_type: eventType,
        player_external_id: playerExternalId.trim(),
        properties: parsed,
      });
      toast({
        title: res.duplicate ? 'Duplicate activity' : 'Activity accepted',
        description: res.duplicate
          ? `event_id "${eventId}" was already ingested; nothing new was evaluated.`
          : `Activity ${res.activity_id} is ${res.status}. Rules evaluate it asynchronously.`,
      });
      onOpenChange(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to send activity');
    }
  };

  const canSubmit = !!eventType && !!playerExternalId.trim() && !!eventId.trim() && !ingest.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Send test activity</DialogTitle>
          <DialogDescription>
            Ingests a real activity. Live rules run on it and their effects (points, XP, badges…) are applied to the player.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label>Event type *</Label>
            <Select value={eventType} onValueChange={setEventType}>
              <SelectTrigger>
                <SelectValue placeholder="Select an event type" />
              </SelectTrigger>
              <SelectContent>
                {eventTypes.map(et => (
                  <SelectItem key={et.id} value={et.slug}>
                    {et.name} <code className="text-xs text-muted-foreground ml-1">{et.slug}</code>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Player external id *</Label>
            <Input value={playerExternalId} onChange={e => setPlayerExternalId(e.target.value)} placeholder="e.g. user-123" />
          </div>
          <div className="space-y-2">
            <Label>Event id (idempotency key)</Label>
            <Input value={eventId} onChange={e => setEventId(e.target.value)} className="font-mono text-xs" />
          </div>
          <div className="space-y-2">
            <Label>Properties (JSON)</Label>
            <Textarea value={properties} onChange={e => setProperties(e.target.value)} rows={4} className="font-mono text-xs" />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!canSubmit}>
            {ingest.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
            Send activity
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

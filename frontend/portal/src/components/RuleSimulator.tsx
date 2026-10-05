import { useEffect, useState } from 'react';
import { Loader2, Play, Send } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { cn } from '@/lib/utils';
import { useEventsQuery } from '@/services/queries/events';
import { useSimulateRulesMutation } from '@/services/queries/rules';
import type { SimulateRulesData } from '@/services/api/types';
import { ConditionTraceList, EffectList } from './RuleTrace';
import SendActivityDialog from './SendActivityDialog';

interface RuleSimulatorProps {
  defaultEventType?: string;
  /** Highlights this rule in the results. */
  focusRuleId?: string;
  /** Shows "Send test activity" (a real, state-changing ingest). */
  canSendActivity?: boolean;
  compact?: boolean;
}

/**
 * POST /rules/simulate: evaluates a hypothetical activity against the live
 * (published, active) ruleset and shows per-rule condition traces and the
 * effects it would request. Nothing is written and limits are not enforced.
 */
export default function RuleSimulator({ defaultEventType, focusRuleId, canSendActivity, compact }: RuleSimulatorProps) {
  const { data: eventTypes = [] } = useEventsQuery();
  const simulate = useSimulateRulesMutation();
  const [eventType, setEventType] = useState(defaultEventType ?? '');
  const [playerExternalId, setPlayerExternalId] = useState('');
  const [properties, setProperties] = useState('{}');
  const [error, setError] = useState<string | null>(null);
  const [sendOpen, setSendOpen] = useState(false);

  useEffect(() => {
    if (defaultEventType) setEventType(defaultEventType);
  }, [defaultEventType]);

  const run = async () => {
    setError(null);
    let parsed: Record<string, unknown>;
    try {
      const v = properties.trim() ? JSON.parse(properties) : {};
      if (typeof v !== 'object' || v === null || Array.isArray(v)) throw new Error();
      parsed = v;
    } catch {
      setError('Properties must be a JSON object, e.g. {"amount": 150}.');
      return;
    }
    const body: SimulateRulesData = { event_type: eventType, properties: parsed };
    if (playerExternalId.trim()) body.player_external_id = playerExternalId.trim();
    else body.player = { is_active: true };
    try {
      await simulate.mutateAsync(body);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Simulation failed');
    }
  };

  const result = simulate.data;

  return (
    <div className="space-y-4">
      <div className={cn('grid gap-3', !compact && 'md:grid-cols-2')}>
        <div className="space-y-2">
          <Label>Event type</Label>
          <Select value={eventType} onValueChange={setEventType}>
            <SelectTrigger>
              <SelectValue placeholder="Select an event type" />
            </SelectTrigger>
            <SelectContent>
              {eventTypes.map(et => (
                <SelectItem key={et.id} value={et.slug}>
                  {et.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label>Player external id (optional)</Label>
          <Input
            value={playerExternalId}
            onChange={e => setPlayerExternalId(e.target.value)}
            placeholder="Blank = anonymous active player"
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label>Activity properties (JSON)</Label>
        <Textarea
          value={properties}
          onChange={e => setProperties(e.target.value)}
          rows={compact ? 3 : 4}
          className="font-mono text-xs"
        />
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={run} disabled={!eventType || simulate.isPending} className={cn(compact && 'w-full')}>
          {simulate.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
          Run simulation
        </Button>
        {canSendActivity && (
          <Button variant="ghost" onClick={() => setSendOpen(true)} className={cn(compact && 'w-full')}>
            <Send className="w-4 h-4" />
            Send test activity
          </Button>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        Simulation runs against published, active rules only and writes nothing; limits are reported, not enforced.
      </p>

      {result && (
        <div className="space-y-3 border-t pt-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-medium">Outcome</span>
            <Badge variant="outline" className={cn(result.outcome === 'matched' && 'border-green-500/50 text-green-500')}>
              {result.outcome}
            </Badge>
            {result.reason && <span className="text-muted-foreground">{result.reason}</span>}
          </div>
          {result.rules.length === 0 ? (
            <p className="text-sm text-muted-foreground">No live rule is triggered by this event type.</p>
          ) : (
            result.rules.map(r => (
              <div
                key={r.rule_id}
                className={cn('rounded-lg border p-3 space-y-2', r.rule_id === focusRuleId && 'border-primary')}
              >
                <div className="flex items-center gap-2 text-sm">
                  <span className="font-medium">{r.name}</span>
                  <Badge variant={r.matched ? 'default' : 'secondary'}>{r.status}</Badge>
                  <span className="text-xs text-muted-foreground">priority {r.priority}</span>
                </div>
                {r.error && <p className="text-xs text-destructive">{r.error}</p>}
                <ConditionTraceList traces={r.condition_results} />
                {r.matched && <EffectList effects={r.effects} />}
              </div>
            ))
          )}
        </div>
      )}

      {canSendActivity && (
        <SendActivityDialog
          open={sendOpen}
          onOpenChange={setSendOpen}
          defaultEventType={eventType}
          defaultProperties={properties}
        />
      )}
    </div>
  );
}

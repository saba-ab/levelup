import { useEffect, useState } from 'react';
import { Loader2, Play, Send, OctagonX } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { cn } from '@/lib/utils';
import { useEventsQuery } from '@/services/queries/events';
import { ApiRequestError, useSimulateRulesMutation } from '@/services/queries/rules';
import { DRAFT_RULE_ID, type DraftRuleDefinition, type SimulateRulesDataV2 } from '@/services/api/models/rules';
import { ConditionTraceList, EffectList, RuleStatusBadge } from './RuleTrace';
import SendActivityDialog from './SendActivityDialog';

type SimulationTarget = 'draft' | 'live';

interface RuleSimulatorProps {
  defaultEventType?: string;
  /** Highlights this rule in the results. */
  focusRuleId?: string;
  /** Shows "Send test activity" (a real, state-changing ingest). */
  canSendActivity?: boolean;
  compact?: boolean;
  /**
   * Builds the unsaved rule body to simulate (null when the form is
   * incomplete: the caller reports why). When given, the simulator defaults to
   * simulating the draft alone instead of the live ruleset.
   */
  getDraft?: () => DraftRuleDefinition | null;
  /** Receives field errors of an invalid draft, keyed like the rule form ("conditions[0].value"). */
  onDraftErrors?: (errors: Record<string, string[]>) => void;
}

/** "2026-10-06T14:30" (datetime-local, local time) -> RFC 3339 UTC. */
function localInputToIso(v: string): string | undefined {
  if (!v) return undefined;
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

/**
 * POST /rules/simulate: evaluates a hypothetical activity against the live
 * (published, active) ruleset, or against an unsaved draft alone, and shows
 * per-rule condition traces and the effects it would request. Nothing is
 * written and limits are not enforced.
 */
export default function RuleSimulator({
  defaultEventType,
  focusRuleId,
  canSendActivity,
  compact,
  getDraft,
  onDraftErrors,
}: RuleSimulatorProps) {
  const { data: eventTypes = [] } = useEventsQuery();
  const simulate = useSimulateRulesMutation();
  const [target, setTarget] = useState<SimulationTarget>(getDraft ? 'draft' : 'live');
  const [eventType, setEventType] = useState(defaultEventType ?? '');
  const [playerExternalId, setPlayerExternalId] = useState('');
  const [occurredAt, setOccurredAt] = useState('');
  const [properties, setProperties] = useState('{}');
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<string[]>([]);
  const [sendOpen, setSendOpen] = useState(false);

  const simulatingDraft = !!getDraft && target === 'draft';

  useEffect(() => {
    if (defaultEventType) setEventType(defaultEventType);
  }, [defaultEventType]);

  const run = async () => {
    setError(null);
    setFieldErrors([]);
    onDraftErrors?.({});
    let parsed: Record<string, unknown>;
    try {
      const v = properties.trim() ? JSON.parse(properties) : {};
      if (typeof v !== 'object' || v === null || Array.isArray(v)) throw new Error();
      parsed = v;
    } catch {
      setError('Properties must be a JSON object, e.g. {"amount": 150}.');
      return;
    }
    const body: SimulateRulesDataV2 = { properties: parsed, occurred_at: localInputToIso(occurredAt) };
    if (simulatingDraft) {
      const definition = getDraft!();
      if (!definition) {
        setError('Complete the rule (trigger event, conditions and actions) to simulate it.');
        return;
      }
      body.definition = definition;
    } else {
      body.event_type = eventType;
    }
    if (playerExternalId.trim()) body.player_external_id = playerExternalId.trim();
    else body.player = { is_active: true };
    try {
      await simulate.mutateAsync(body);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Simulation failed');
      if (e instanceof ApiRequestError && e.validationErrors) {
        const entries = Object.entries(e.validationErrors);
        setFieldErrors(entries.flatMap(([k, msgs]) => msgs.map(m => `${k}: ${m}`)));
        if (simulatingDraft && onDraftErrors) {
          onDraftErrors(
            Object.fromEntries(
              entries
                .filter(([k]) => k.startsWith('definition.'))
                .map(([k, msgs]) => [k.slice('definition.'.length), msgs]),
            ),
          );
        }
      }
    }
  };

  const result = simulate.data;
  const usesHistory = result?.rules.some(r => r.condition_results.some(c => c.source === 'history')) ?? false;
  const nameOf = (ruleId: string) =>
    ruleId === DRAFT_RULE_ID ? 'this draft' : (result?.rules.find(r => r.rule_id === ruleId)?.name ?? ruleId);

  return (
    <div className="space-y-4">
      {getDraft && (
        <div className="space-y-2">
          <Label>Simulate</Label>
          <Select
            value={target}
            onValueChange={v => {
              setTarget(v as SimulationTarget);
              simulate.reset();
              setError(null);
              setFieldErrors([]);
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="draft">This rule as edited (unsaved)</SelectItem>
              <SelectItem value="live">The live ruleset</SelectItem>
            </SelectContent>
          </Select>
        </div>
      )}
      <div className={cn('grid gap-3', !compact && 'md:grid-cols-2')}>
        <div className="space-y-2">
          <Label>Event type</Label>
          <Select value={eventType} onValueChange={setEventType} disabled={simulatingDraft}>
            <SelectTrigger>
              <SelectValue placeholder="Select an event type" />
            </SelectTrigger>
            <SelectContent>
              {eventTypes.map(et => (
                <SelectItem key={et.id} value={et.slug}>
                  {et.name}
                </SelectItem>
              ))}
              {eventType && !eventTypes.some(et => et.slug === eventType) && (
                <SelectItem value={eventType}>{eventType}</SelectItem>
              )}
            </SelectContent>
          </Select>
          {simulatingDraft && <p className="text-xs text-muted-foreground">A draft is simulated with its trigger event.</p>}
        </div>
        <div className="space-y-2">
          <Label>Player external id (optional)</Label>
          <Input
            value={playerExternalId}
            onChange={e => setPlayerExternalId(e.target.value)}
            placeholder="Blank = anonymous active player"
          />
        </div>
        <div className="space-y-2">
          <Label>Occurred at (optional)</Label>
          <Input type="datetime-local" value={occurredAt} onChange={e => setOccurredAt(e.target.value)} />
          <p className="text-xs text-muted-foreground">Drives schedules and history windows; blank = now.</p>
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
      {error && (
        <div className="text-sm text-destructive space-y-1">
          <p>{error}</p>
          {fieldErrors.map(f => (
            <p key={f} className="text-xs font-mono">
              {f}
            </p>
          ))}
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          onClick={run}
          disabled={(!simulatingDraft && !eventType) || simulate.isPending}
          className={cn(compact && 'w-full')}
        >
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
        {simulatingDraft
          ? 'Only this rule, as currently edited, is evaluated. Nothing is saved or written; limits are reported, not enforced.'
          : 'Simulation runs against published, active rules only and writes nothing; limits are reported, not enforced.'}
      </p>

      {result && (
        <div className="space-y-3 border-t pt-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-medium">Outcome</span>
            <Badge variant="outline" className={cn(result.outcome === 'matched' && 'border-green-500/50 text-green-500')}>
              {result.outcome}
            </Badge>
            {result.draft && <Badge variant="secondary">draft</Badge>}
            {result.reason && <span className="text-muted-foreground">{result.reason}</span>}
            {result.occurred_at && (
              <span className="text-xs text-muted-foreground">at {new Date(result.occurred_at).toLocaleString()}</span>
            )}
          </div>
          {usesHistory && !result.history_loaded && (
            <p className="text-xs text-amber-500">
              History conditions had no data: enter a stored player's external id to evaluate them against real activity.
            </p>
          )}
          {result.rules.length === 0 ? (
            <p className="text-sm text-muted-foreground">No live rule is triggered by this event type.</p>
          ) : (
            result.rules.map(r => (
              <div
                key={r.rule_id}
                className={cn(
                  'rounded-lg border p-3 space-y-2',
                  (r.rule_id === focusRuleId || r.rule_id === DRAFT_RULE_ID) && 'border-primary',
                )}
              >
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <span className="font-medium">{r.rule_id === DRAFT_RULE_ID ? 'Draft (unsaved)' : r.name}</span>
                  <RuleStatusBadge status={r.status} />
                  <span className="text-xs text-muted-foreground">priority {r.priority}</span>
                  {r.stop_processing && (
                    <Badge variant="outline" className="gap-1 text-[10px] font-normal">
                      <OctagonX className="w-3 h-3" /> stops processing
                    </Badge>
                  )}
                </div>
                {r.stopped_by && (
                  <p className="text-xs text-muted-foreground">Skipped because "{nameOf(r.stopped_by)}" fired first.</p>
                )}
                {r.error && <p className="text-xs text-destructive">{r.error}</p>}
                {r.status !== 'out_of_schedule' && r.status !== 'skipped_by_stop' && (
                  <ConditionTraceList traces={r.condition_results} />
                )}
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

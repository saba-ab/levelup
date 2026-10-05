import { CheckCircle2, XCircle, Loader2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { useRuleDecisionQuery } from '@/services/queries/rules';
import type { ConditionTrace, ID } from '@/services/api/types';

const fmt = (v: unknown) => (v === undefined ? '—' : JSON.stringify(v));

const effectLabels: Record<string, string> = {
  credit_points: 'Credit points',
  grant_xp: 'Grant XP',
  award_badge: 'Award badge',
  record_streak: 'Record streak',
  progress_mission: 'Progress mission',
  grant_reward: 'Grant reward',
};

function effectLabel(type: string): string {
  return effectLabels[type] ?? type;
}

/** Per-condition evaluation trace (simulation or a recorded execution). */
export function ConditionTraceList({ traces }: { traces: ConditionTrace[] }) {
  if (traces.length === 0) {
    return <p className="text-xs text-muted-foreground">No conditions: the rule matches every activity of its trigger.</p>;
  }
  return (
    <ul className="space-y-1">
      {traces.map(t => (
        <li key={t.path} className="flex items-center gap-2 text-xs font-mono">
          {t.result ? (
            <CheckCircle2 className="w-3.5 h-3.5 text-green-500 shrink-0" />
          ) : (
            <XCircle className="w-3.5 h-3.5 text-destructive shrink-0" />
          )}
          <span className="text-muted-foreground">{t.path}</span>
          <span>
            {t.source}.{t.field} {t.operator} {fmt(t.expected)}
          </span>
          <span className="text-muted-foreground">(actual: {t.present ? fmt(t.actual) : 'missing'})</span>
        </li>
      ))}
    </ul>
  );
}

/** Action effects as "type {params}" chips. */
export function EffectList({ effects }: { effects: { type: string; params: Record<string, unknown>; status?: string }[] }) {
  if (effects.length === 0) return <p className="text-xs text-muted-foreground">No effects.</p>;
  return (
    <div className="flex flex-wrap gap-2">
      {effects.map((e, i) => (
        <Badge key={i} variant="outline" className="font-normal gap-1">
          <span className="font-medium">{effectLabel(e.type)}</span>
          <code className="text-[11px] text-muted-foreground">{JSON.stringify(e.params)}</code>
          {e.status && <span className="text-[11px] text-muted-foreground">· {e.status}</span>}
        </Badge>
      ))}
    </div>
  );
}

/** A recorded rules decision with its rule executions and effects. */
export function DecisionDetail({ decisionId }: { decisionId: ID }) {
  const { data, isLoading, error } = useRuleDecisionQuery(decisionId);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="w-4 h-4 animate-spin" /> Loading decision…
      </div>
    );
  }
  if (error || !data) return <p className="text-sm text-destructive">{error?.message ?? 'Decision not found'}</p>;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="outline" className={cn(data.outcome === 'matched' && 'border-green-500/50 text-green-500')}>
          {data.outcome}
        </Badge>
        {data.reason && <span className="text-muted-foreground">{data.reason}</span>}
        <span className="text-muted-foreground">
          evaluated {new Date(data.evaluated_at).toLocaleString()} in {data.duration_us} µs
        </span>
      </div>
      {data.executions.length === 0 ? (
        <p className="text-sm text-muted-foreground">No rule was evaluated for this activity.</p>
      ) : (
        data.executions.map(ex => (
          <div key={ex.id} className="rounded-lg border p-3 space-y-2">
            <div className="flex items-center gap-2 text-sm">
              <code className="text-xs">{ex.rule_id}</code>
              <Badge variant={ex.matched ? 'default' : 'secondary'}>{ex.status}</Badge>
            </div>
            <ConditionTraceList traces={ex.condition_results} />
            <EffectList effects={data.effects.filter(e => e.execution_id === ex.id)} />
          </div>
        ))
      )}
    </div>
  );
}

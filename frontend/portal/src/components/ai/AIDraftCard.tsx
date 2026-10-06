import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Check, ChevronDown, Copy, ExternalLink, Loader2, Plus } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import type { AIDraft, AIDraftKind } from '@/services/api/models/ai';
import type { CreatedFromDraft } from '@/services/queries/ai';
import { describeAIError } from '@/services/queries/ai';
import { AI_KIND_META } from './kinds';

export type DraftCreateState =
  | { status: 'idle' }
  | { status: 'pending' }
  | { status: 'created'; created: CreatedFromDraft }
  | { status: 'error'; error: unknown };

interface AIDraftCardProps {
  kind: AIDraftKind;
  draft: AIDraft;
  index: number;
  state: DraftCreateState;
  /** Omit to hide the Create action (no permission). */
  onCreate?: () => void;
}

const SUMMARY_SKIP = new Set(['name', 'description', 'slug']);

function formatValue(value: unknown): string {
  if (typeof value === 'boolean') return value ? 'yes' : 'no';
  if (typeof value === 'number') return value.toLocaleString();
  return String(value);
}

/** Primitive top-level fields as chips; nested objects stay in the JSON view. */
function summaryFields(draft: AIDraft): [string, string][] {
  return Object.entries(draft)
    .filter(([k, v]) => !SUMMARY_SKIP.has(k) && v !== null && v !== '' && ['string', 'number', 'boolean'].includes(typeof v))
    .map(([k, v]) => [k.replace(/_/g, ' '), formatValue(v)]);
}

/** One validated draft: summary, the exact create body, and a Create action. */
export function AIDraftCard({ kind, draft, index, state, onCreate }: AIDraftCardProps) {
  const [copied, setCopied] = useState(false);
  const meta = AI_KIND_META[kind];
  const Icon = meta.icon;
  const name = typeof draft.name === 'string' && draft.name ? draft.name : `${meta.label} draft ${index + 1}`;
  const description = typeof draft.description === 'string' ? draft.description : '';
  const json = JSON.stringify(draft, null, 2);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(json);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard unavailable (insecure context): nothing to do
    }
  };

  return (
    <Card>
      <CardContent className="space-y-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-start gap-3">
            <div className="mt-0.5 rounded-md bg-primary/10 p-2 text-primary">
              <Icon className="h-4 w-4" />
            </div>
            <div className="min-w-0">
              <p className="truncate font-semibold">{name}</p>
              {description && <p className="text-sm text-muted-foreground">{description}</p>}
            </div>
          </div>
          {state.status === 'created' ? (
            <Button asChild variant="outline" size="sm" className="shrink-0 gap-1">
              <Link to={meta.path(state.created.id)}>
                <Check className="h-3.5 w-3.5 text-green-600" />
                Created
                <ExternalLink className="h-3 w-3" />
              </Link>
            </Button>
          ) : onCreate ? (
            <Button size="sm" className="shrink-0 gap-1" onClick={onCreate} disabled={state.status === 'pending'}>
              {state.status === 'pending' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
              Create {meta.label.toLowerCase()}
            </Button>
          ) : null}
        </div>

        {summaryFields(draft).length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {summaryFields(draft).map(([k, v]) => (
              <Badge key={k} variant="outline" className="bg-secondary text-xs font-normal">
                <span className="mr-1 text-muted-foreground">{k}:</span>
                {v}
              </Badge>
            ))}
          </div>
        )}

        {state.status === 'error' && (
          <ErrorLine error={state.error} />
        )}

        <Collapsible>
          <div className="flex items-center justify-between">
            <CollapsibleTrigger asChild>
              <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs">
                <ChevronDown className="h-3 w-3" />
                Request body
              </Button>
            </CollapsibleTrigger>
            <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs" onClick={copy}>
              {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
              {copied ? 'Copied' : 'Copy JSON'}
            </Button>
          </div>
          <CollapsibleContent>
            <pre className="mt-2 max-h-64 overflow-auto rounded-md border bg-muted/40 p-3 text-xs">{json}</pre>
          </CollapsibleContent>
        </Collapsible>
      </CardContent>
    </Card>
  );
}

function ErrorLine({ error }: { error: unknown }) {
  const described = describeAIError(error);
  const fields = described.fieldErrors ? Object.entries(described.fieldErrors) : [];
  return (
    <div className="rounded-md border border-destructive/40 bg-destructive/10 p-2 text-xs text-destructive">
      <p>{described.message}</p>
      {fields.map(([f, m]) => (
        <p key={f}>
          <span className="font-mono">{f}</span>: {m.join(', ')}
        </p>
      ))}
    </div>
  );
}

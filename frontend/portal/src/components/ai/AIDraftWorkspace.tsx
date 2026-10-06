import { useState } from 'react';
import { AlertTriangle, Info, Loader2, Sparkles, XCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';
import { useAuth } from '@/contexts/AuthContext';
import { cn } from '@/lib/utils';
import { AI_MAX_COUNT, AI_MAX_PROMPT_LENGTH, AI_MIN_COUNT, type AIDraftKind, type AIDraftResponse } from '@/services/api/models/ai';
import { useAIDraftContext, useAIDraftsMutation, useAIUsageQuery, useCreateFromDraft, type CreatedFromDraft } from '@/services/queries/ai';
import { AIDraftCard, type DraftCreateState } from './AIDraftCard';
import { AIErrorAlert, AINotConfiguredAlert } from './AIErrorAlert';
import { AI_KIND_META } from './kinds';

interface AIDraftWorkspaceProps {
  kind: AIDraftKind;
  prompt: string;
  onPromptChange: (prompt: string) => void;
  count: number;
  onCountChange: (count: number) => void;
  placeholder?: string;
  /** Called after a draft was created through its module's endpoint. */
  onCreated?: (kind: AIDraftKind, created: CreatedFromDraft) => void;
  /** Hide the "not configured" alert when the page already shows one. */
  showNotConfiguredAlert?: boolean;
  className?: string;
}

const COUNTS = Array.from({ length: AI_MAX_COUNT - AI_MIN_COUNT + 1 }, (_, i) => AI_MIN_COUNT + i);

/**
 * Prompt → POST /ai/drafts (with the tenant's events, badges, missions,
 * rewards and levels as context) → validated drafts, each creatable through
 * the owning module, plus the rejected ones with reasons.
 */
export function AIDraftWorkspace({ kind, prompt, onPromptChange, count, onCountChange, placeholder, onCreated, showNotConfiguredAlert = true, className }: AIDraftWorkspaceProps) {
  const { toast } = useToast();
  const { hasPermission } = useAuth();
  const { context, isLoading: contextLoading, isPartial } = useAIDraftContext();
  const draftsMutation = useAIDraftsMutation();
  const usage = useAIUsageQuery(1);
  const createFromDraft = useCreateFromDraft();
  const [result, setResult] = useState<AIDraftResponse | null>(null);
  const [createStates, setCreateStates] = useState<Record<number, DraftCreateState>>({});

  const meta = AI_KIND_META[kind];
  const canCreate = hasPermission(meta.createPermission);
  const tooLong = prompt.length > AI_MAX_PROMPT_LENGTH;
  const notConfigured = usage.data?.enabled === false;

  const generate = () => {
    if (!prompt.trim() || tooLong) return;
    setResult(null);
    setCreateStates({});
    draftsMutation.mutate(
      { kind, prompt: prompt.trim(), count, context },
      { onSuccess: (res) => setResult(res) },
    );
  };

  const create = async (index: number) => {
    if (!result) return;
    setCreateStates((s) => ({ ...s, [index]: { status: 'pending' } }));
    try {
      const created = await createFromDraft(result.kind, result.drafts[index]);
      setCreateStates((s) => ({ ...s, [index]: { status: 'created', created } }));
      toast({ title: `${AI_KIND_META[result.kind].label} created`, description: created.name });
      onCreated?.(result.kind, created);
    } catch (error) {
      setCreateStates((s) => ({ ...s, [index]: { status: 'error', error } }));
    }
  };

  return (
    <div className={cn('space-y-4', className)}>
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <Label htmlFor="ai-prompt">Describe the {meta.plural.toLowerCase()} you want</Label>
          <span className={cn('text-xs', tooLong ? 'text-destructive' : 'text-muted-foreground')}>
            {prompt.length}/{AI_MAX_PROMPT_LENGTH}
          </span>
        </div>
        <Textarea
          id="ai-prompt"
          placeholder={placeholder ?? `E.g., ${meta.plural.toLowerCase()} for a fitness app that reward weekly consistency...`}
          value={prompt}
          onChange={(e) => onPromptChange(e.target.value)}
          className="min-h-[110px] resize-y"
        />
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <Label className="text-xs text-muted-foreground">Drafts</Label>
          <Select value={String(count)} onValueChange={(v) => onCountChange(Number(v))}>
            <SelectTrigger className="w-24">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {COUNTS.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button
          className="flex-1 gap-2"
          onClick={generate}
          disabled={notConfigured || draftsMutation.isPending || contextLoading || !prompt.trim() || tooLong}
        >
          {draftsMutation.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}
          {draftsMutation.isPending ? 'Generating...' : contextLoading ? 'Loading your data...' : `Generate ${meta.label.toLowerCase()} drafts`}
        </Button>
      </div>

      {notConfigured && showNotConfiguredAlert && <AINotConfiguredAlert />}

      {isPartial && (
        <p className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-400">
          <AlertTriangle className="h-3.5 w-3.5" />
          Some of your existing data could not be loaded; drafts may not reference it.
        </p>
      )}

      {draftsMutation.isError && (
        <AIErrorAlert
          error={draftsMutation.error}
          quota={usage.data ? { daily_limit: usage.data.daily_limit, used: usage.data.today.requests } : null}
        />
      )}

      {result && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
            <span>
              {result.drafts.length} draft{result.drafts.length === 1 ? '' : 's'}
              {result.rejected.length > 0 && `, ${result.rejected.length} rejected`} · {result.model}
            </span>
            <span>
              {(result.usage.input_tokens + result.usage.output_tokens).toLocaleString()} tokens
              {result.quota.daily_limit > 0 && ` · ${Math.max(result.quota.remaining, 0).toLocaleString()} of ${result.quota.daily_limit.toLocaleString()} requests left today`}
            </span>
          </div>

          {!canCreate && result.drafts.length > 0 && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Info className="h-3.5 w-3.5" />
              Your role cannot create {meta.plural.toLowerCase()}; copy the JSON to share it.
            </p>
          )}

          {result.drafts.length === 0 && result.rejected.length === 0 && (
            <p className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
              The AI returned no drafts. Try a more specific prompt.
            </p>
          )}

          {result.drafts.map((draft, i) => (
            <AIDraftCard
              key={i}
              kind={result.kind}
              draft={draft}
              index={i}
              state={createStates[i] ?? { status: 'idle' }}
              onCreate={canCreate ? () => create(i) : undefined}
            />
          ))}

          {result.rejected.length > 0 && (
            <div className="space-y-2 rounded-md border border-amber-500/40 bg-amber-500/5 p-3">
              <p className="flex items-center gap-1.5 text-sm font-medium text-amber-600 dark:text-amber-400">
                <XCircle className="h-4 w-4" />
                Rejected drafts
              </p>
              <ul className="space-y-1.5 text-xs">
                {result.rejected.map((r) => (
                  <li key={r.index}>
                    <span className="font-medium">Draft #{r.index + 1}:</span>
                    <ul className="list-disc pl-5 text-muted-foreground">
                      {r.reasons.map((reason, j) => (
                        <li key={j}>{reason}</li>
                      ))}
                    </ul>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

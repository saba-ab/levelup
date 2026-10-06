import { useState } from 'react';
import { Lightbulb, RefreshCw, Sparkles, Wand2 } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { cn } from '@/lib/utils';
import { AIDraftWorkspace } from '@/components/ai/AIDraftWorkspace';
import { AINotConfiguredAlert } from '@/components/ai/AIErrorAlert';
import { AIUsagePanel } from '@/components/ai/AIUsagePanel';
import { AI_KIND_META } from '@/components/ai/kinds';
import { AI_DEFAULT_COUNT, AI_DRAFT_KINDS, type AIDraftKind, type AIPromptTemplate } from '@/services/api/models/ai';
import { useAITemplatesQuery, useAIUsageQuery } from '@/services/queries/ai';

const USAGE_DAYS = 30;

export default function AIHub() {
  const [kind, setKind] = useState<AIDraftKind>('badge');
  const [prompt, setPrompt] = useState('');
  const [count, setCount] = useState(AI_DEFAULT_COUNT);
  const [activeTemplateId, setActiveTemplateId] = useState<string | null>(null);

  const usage = useAIUsageQuery(USAGE_DAYS);
  const templates = useAITemplatesQuery();
  const kindTemplates = (templates.data ?? []).filter((t) => t.kind === kind);
  const notConfigured = usage.data?.enabled === false;

  const changeKind = (next: AIDraftKind) => {
    setKind(next);
    setActiveTemplateId(null);
  };

  const applyTemplate = (t: AIPromptTemplate) => {
    setKind(t.kind);
    setPrompt(t.example_prompt);
    setCount(t.default_count || AI_DEFAULT_COUNT);
    setActiveTemplateId(t.id);
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="flex items-center gap-3 text-3xl font-bold">
            <Sparkles className="h-8 w-8 text-primary" />
            AI Hub
          </h1>
          <p className="mt-1 text-muted-foreground">
            Draft badges, levels, missions, rewards, rules and segments from a prompt, grounded in your existing data.
          </p>
        </div>
        {usage.data?.model && (
          <Badge variant="outline" className="w-fit gap-1.5">
            <Wand2 className="h-3.5 w-3.5" />
            {usage.data.model}
          </Badge>
        )}
      </div>

      {usage.isError ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-8 text-center">
            <p className="text-sm text-muted-foreground">Could not load AI usage: {usage.error.message}</p>
            <Button variant="outline" size="sm" className="gap-2" onClick={() => usage.refetch()}>
              <RefreshCw className="h-4 w-4" />
              Retry
            </Button>
          </CardContent>
        </Card>
      ) : notConfigured ? (
        <AINotConfiguredAlert />
      ) : (
        <AIUsagePanel usage={usage.data} isLoading={usage.isLoading} days={USAGE_DAYS} />
      )}

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-5">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg">
              <Lightbulb className="h-5 w-5 text-primary" />
              Prompt templates
            </CardTitle>
            <CardDescription>Start from an example prompt, then adapt it to your program.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <Tabs value={kind} onValueChange={(v) => changeKind(v as AIDraftKind)}>
              <TabsList className="grid h-auto w-full grid-cols-3">
                {AI_DRAFT_KINDS.map((k) => {
                  const Icon = AI_KIND_META[k].icon;
                  return (
                    <TabsTrigger key={k} value={k} className="gap-1.5 text-xs">
                      <Icon className="h-3.5 w-3.5" />
                      {AI_KIND_META[k].plural}
                    </TabsTrigger>
                  );
                })}
              </TabsList>
            </Tabs>

            {templates.isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-20" />
                <Skeleton className="h-20" />
              </div>
            ) : templates.isError ? (
              <div className="flex flex-col items-center gap-2 py-6 text-center text-sm text-muted-foreground">
                <p>Could not load templates: {templates.error.message}</p>
                <Button variant="outline" size="sm" onClick={() => templates.refetch()}>Retry</Button>
              </div>
            ) : kindTemplates.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">
                No templates for {AI_KIND_META[kind].plural.toLowerCase()}; write your own prompt.
              </p>
            ) : (
              <div className="space-y-2">
                {kindTemplates.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => applyTemplate(t)}
                    className={cn(
                      'w-full rounded-lg border p-3 text-left transition-colors hover:border-primary/50 hover:bg-secondary/50',
                      activeTemplateId === t.id ? 'border-primary bg-primary/5' : 'border-border',
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-sm font-medium">{t.name}</p>
                      <Badge variant="secondary" className="text-xs">
                        {t.default_count} draft{t.default_count === 1 ? '' : 's'}
                      </Badge>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">{t.description}</p>
                  </button>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="lg:col-span-3">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg">
              <Wand2 className="h-5 w-5 text-primary" />
              Generate {AI_KIND_META[kind].label.toLowerCase()} drafts
            </CardTitle>
            <CardDescription>
              Each draft is validated against your event types, badges, missions, rewards and levels, and can be created as-is.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <AIDraftWorkspace
              key={kind}
              kind={kind}
              prompt={prompt}
              onPromptChange={setPrompt}
              count={count}
              onCountChange={setCount}
              showNotConfiguredAlert={false}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

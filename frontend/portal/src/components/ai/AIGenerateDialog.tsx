import React, { useState } from 'react';
import { Sparkles } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { useAuth } from '@/contexts/AuthContext';
import { AI_DEFAULT_COUNT, type AIDraftKind } from '@/services/api/models/ai';
import type { CreatedFromDraft } from '@/services/queries/ai';
import { AIDraftWorkspace } from './AIDraftWorkspace';
import { AI_KIND_META, inferAIKind } from './kinds';

interface AIGenerateDialogProps {
  trigger?: React.ReactNode;
  title?: string;
  placeholder?: string;
  /** Short hint shown above the prompt. */
  context?: string;
  /**
   * What to draft (POST /ai/drafts kind). When omitted it is inferred from
   * `title` ("Generate Badge Ideas" → badge); with no match the dialog is not
   * rendered, since only these kinds can be drafted.
   */
  kind?: AIDraftKind;
  /** Drafts per request, 1..5 (default 3). */
  defaultCount?: number;
  /** Called after a draft was created through its module's endpoint. */
  onCreated?: (kind: AIDraftKind, created: CreatedFromDraft) => void;
  /**
   * @deprecated Ignored: drafts come from POST /ai/drafts. Kept so existing
   * call sites compile.
   */
  onGenerate?: (prompt: string) => Promise<string>;
  /** @deprecated Ignored, see onGenerate. */
  onUseResult?: (result: string) => void;
}

/** "Generate with AI" for one entity kind: drafts from the AI module, each creatable in place. */
export function AIGenerateDialog({
  trigger,
  title,
  placeholder,
  context,
  kind: kindProp,
  defaultCount = AI_DEFAULT_COUNT,
  onCreated,
}: AIGenerateDialogProps) {
  const { hasPermission } = useAuth();
  const [open, setOpen] = useState(false);
  const [prompt, setPrompt] = useState('');
  const [count, setCount] = useState(defaultCount);
  const kind = kindProp ?? inferAIKind(title);

  if (!kind || !hasPermission('view:ai-hub')) return null;
  const meta = AI_KIND_META[kind];

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {trigger || (
          <Button variant="outline" size="sm" className="gap-2">
            <Sparkles className="w-4 h-4" />
            Generate with AI
          </Button>
        )}
      </DialogTrigger>
      <DialogContent className="sm:max-w-[680px] max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles className="w-5 h-5 text-primary" />
            {title ?? `Generate ${meta.label.toLowerCase()} drafts`}
          </DialogTitle>
          <DialogDescription>
            {context ?? 'Drafts are checked against your existing data and can be created directly.'}
          </DialogDescription>
        </DialogHeader>
        <AIDraftWorkspace
          kind={kind}
          prompt={prompt}
          onPromptChange={setPrompt}
          count={count}
          onCountChange={setCount}
          placeholder={placeholder}
          onCreated={onCreated}
        />
      </DialogContent>
    </Dialog>
  );
}

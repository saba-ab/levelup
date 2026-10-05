import React, { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';
import { Sparkles, Loader2, Copy, Check } from 'lucide-react';
import { cn } from '@/lib/utils';

interface AIGenerateDialogProps {
  trigger?: React.ReactNode;
  title?: string;
  placeholder?: string;
  context?: string;
  onGenerate?: (prompt: string) => Promise<string>;
  onUseResult?: (result: string) => void;
}

export function AIGenerateDialog({
  trigger,
  title = "Generate with AI",
  placeholder = "Describe what you want to generate...",
  context,
  onGenerate,
  onUseResult,
}: AIGenerateDialogProps) {
  const [open, setOpen] = useState(false);
  const [prompt, setPrompt] = useState('');
  const [result, setResult] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  const handleGenerate = async () => {
    if (!prompt.trim() || !onGenerate) return;

    setIsLoading(true);
    setResult('');

    try {
      setResult(await onGenerate(prompt));
    } catch {
      setResult('Error generating content. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  const handleCopy = async () => {
    await navigator.clipboard.writeText(result);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleUse = () => {
    if (onUseResult && result) {
      onUseResult(result);
      setOpen(false);
      setPrompt('');
      setResult('');
    }
  };

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
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles className="w-5 h-5 text-primary" />
            {title}
          </DialogTitle>
        </DialogHeader>
        
        <div className="space-y-4 py-4">
          {!onGenerate && (
            <div role="status" className="p-3 rounded-lg border border-amber-500/40 bg-amber-500/10 text-sm text-amber-600 dark:text-amber-400">
              AI generation is coming soon: it is not available in this API version.
            </div>
          )}

          {context && (
            <div className="p-3 bg-secondary/50 rounded-lg text-sm text-muted-foreground">
              <span className="font-medium text-foreground">Context:</span> {context}
            </div>
          )}
          
          <div className="space-y-2">
            <Label htmlFor="prompt">Your Prompt</Label>
            <Textarea
              id="prompt"
              placeholder={placeholder}
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              className="min-h-[100px] resize-none"
            />
          </div>
          
          <Button 
            onClick={handleGenerate} 
            disabled={isLoading || !prompt.trim() || !onGenerate}
            className="w-full gap-2"
          >
            {isLoading ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                Generating...
              </>
            ) : (
              <>
                <Sparkles className="w-4 h-4" />
                Generate
              </>
            )}
          </Button>
          
          {result && (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Generated Result</Label>
                <Button 
                  variant="ghost" 
                  size="sm" 
                  onClick={handleCopy}
                  className="gap-1 h-7"
                >
                  {copied ? (
                    <>
                      <Check className="w-3 h-3" />
                      Copied
                    </>
                  ) : (
                    <>
                      <Copy className="w-3 h-3" />
                      Copy
                    </>
                  )}
                </Button>
              </div>
              <div 
                className={cn(
                  "p-4 rounded-lg border bg-card min-h-[120px] max-h-[200px] overflow-y-auto",
                  "whitespace-pre-wrap text-sm"
                )}
              >
                {result}
              </div>
              
              {onUseResult && (
                <Button 
                  onClick={handleUse}
                  variant="secondary"
                  className="w-full"
                >
                  Use This Result
                </Button>
              )}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

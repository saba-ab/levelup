import { useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

/** Copies text to the clipboard and flips to a check mark for two seconds. */
export function CopyButton({ text, label = 'Copy', className }: { text: string; label?: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard blocked (insecure context): the text stays selectable
    }
  };
  return (
    <Button type="button" variant="ghost" size="icon" aria-label={label} className={cn('h-7 w-7', className)} onClick={copy}>
      {copied ? <Check className="h-3.5 w-3.5 text-green-500" /> : <Copy className="h-3.5 w-3.5" />}
    </Button>
  );
}

/** A monospace snippet with a copy button. */
export function CodeBlock({ code, title, className }: { code: string; title?: string; className?: string }) {
  return (
    <div className={cn('rounded-lg border bg-muted/40', className)}>
      <div className="flex items-center justify-between border-b px-3 py-1">
        <span className="text-xs text-muted-foreground">{title ?? ''}</span>
        <CopyButton text={code} label={title ? `Copy ${title}` : 'Copy code'} />
      </div>
      <pre className="overflow-x-auto p-3 text-xs leading-relaxed">
        <code>{code}</code>
      </pre>
    </div>
  );
}

/** Pretty-prints a JSON value (or a raw string that may hold JSON). */
function formatJson(value: unknown): string {
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value ?? null, null, 2);
}

/** A read-only JSON block with copy. */
export function JsonBlock({ value, title, className }: { value: unknown; title?: string; className?: string }) {
  return <CodeBlock code={formatJson(value)} title={title} className={cn('[&_pre]:max-h-80', className)} />;
}

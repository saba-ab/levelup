import { AlertTriangle, Clock, KeyRound, ServerCrash } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { describeAIError } from '@/services/queries/ai';

interface AIErrorAlertProps {
  error: unknown;
  /** Remaining/limit from the last known usage, shown on quota errors. */
  quota?: { daily_limit: number; used?: number } | null;
}

/** Inline state for a failed drafting call: not configured, quota, provider down, validation. */
export function AIErrorAlert({ error, quota }: AIErrorAlertProps) {
  const described = describeAIError(error);
  const Icon =
    described.kind === 'quota' ? Clock : described.kind === 'not_configured' ? KeyRound : described.kind === 'unavailable' ? ServerCrash : AlertTriangle;
  const fieldErrors = described.fieldErrors ? Object.entries(described.fieldErrors) : [];

  return (
    <Alert variant={described.kind === 'quota' || described.kind === 'not_configured' ? 'default' : 'destructive'}>
      <Icon className="h-4 w-4" />
      <AlertTitle>{described.title}</AlertTitle>
      <AlertDescription className="space-y-1">
        <p>{described.message}</p>
        {described.kind === 'quota' && quota && quota.daily_limit > 0 && (
          <p className="text-xs">
            Daily limit: {quota.daily_limit.toLocaleString()} requests
            {quota.used !== undefined ? ` (${quota.used.toLocaleString()} used today)` : ''}.
          </p>
        )}
        {fieldErrors.length > 0 && (
          <ul className="list-disc pl-4 text-xs">
            {fieldErrors.map(([field, msgs]) => (
              <li key={field}>
                <span className="font-mono">{field}</span>: {msgs.join(', ')}
              </li>
            ))}
          </ul>
        )}
      </AlertDescription>
    </Alert>
  );
}

/** Shown when GET /ai/usage reports enabled: false (no provider key on the server). */
export function AINotConfiguredAlert() {
  return (
    <Alert>
      <KeyRound className="h-4 w-4" />
      <AlertTitle>AI is not configured on this server</AlertTitle>
      <AlertDescription>
        Drafting needs an AI provider API key in the server configuration. Ask your administrator to set it up.
      </AlertDescription>
    </Alert>
  );
}

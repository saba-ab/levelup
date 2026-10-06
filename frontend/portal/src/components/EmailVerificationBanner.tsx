import { useState } from 'react';
import { Loader2, MailWarning, X } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { useAuth } from '@/contexts/AuthContext';
import { useToast } from '@/hooks/use-toast';
import { meKeys, useMeQuery, useResendVerificationMutation } from '@/services/queries/users';
import { ApiRequestError } from '@/services/queries/rules';

/**
 * Shown to a signed-in user whose email is not verified yet (GET /auth/me
 * email_verified_at is null), with a button to resend the link. Mount it once
 * in the dashboard layout; it renders nothing otherwise. Dismissal lasts for
 * the browser session.
 */
const DISMISS_KEY = 'levelupos_verify_banner_dismissed';

function readDismissed(): boolean {
  try {
    return sessionStorage.getItem(DISMISS_KEY) === '1';
  } catch {
    return false;
  }
}

export default function EmailVerificationBanner({ className = '' }: { className?: string }) {
  const { isAuthenticated } = useAuth();
  const { data: me } = useMeQuery(isAuthenticated);
  const resend = useResendVerificationMutation();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [dismissed, setDismissed] = useState(readDismissed);
  const [sent, setSent] = useState(false);

  if (!isAuthenticated || dismissed || !me || me.user.email_verified_at) return null;

  const handleResend = async () => {
    try {
      await resend.mutateAsync();
      setSent(true);
      toast({ title: 'Verification email sent', description: `Check ${me.user.email} for a new link.` });
    } catch (err) {
      if (err instanceof ApiRequestError && err.code === 'email_already_verified') {
        queryClient.invalidateQueries({ queryKey: meKeys.all });
        toast({ title: 'Email already verified' });
        return;
      }
      toast({
        title: 'Could not resend',
        description: err instanceof Error ? err.message : 'Please try again.',
        variant: 'destructive',
      });
    }
  };

  const dismiss = () => {
    try {
      sessionStorage.setItem(DISMISS_KEY, '1');
    } catch {
      // Storage blocked: dismiss for this render tree only.
    }
    setDismissed(true);
  };

  return (
    <div
      role="status"
      className={`flex flex-col sm:flex-row sm:items-center gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm ${className}`}
    >
      <MailWarning className="h-4 w-4 shrink-0 text-amber-400" />
      <p className="flex-1">
        <span className="font-medium">Verify your email.</span>{' '}
        <span className="text-muted-foreground">
          {sent
            ? `We sent a new link to ${me.user.email}. Earlier links no longer work.`
            : `We sent a verification link to ${me.user.email}.`}
        </span>
      </p>
      <div className="flex items-center gap-2">
        <Button size="sm" variant="outline" onClick={handleResend} disabled={resend.isPending}>
          {resend.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
          {sent ? 'Resend again' : 'Resend email'}
        </Button>
        <Button size="icon" variant="ghost" className="h-8 w-8" onClick={dismiss} aria-label="Dismiss">
          <X className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

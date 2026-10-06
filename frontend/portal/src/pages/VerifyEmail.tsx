import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { Gamepad2, CheckCircle2, Loader2, MailWarning, RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useAuth } from '@/contexts/AuthContext';
import { useAuthService } from '@/services/api/auth';
import { meKeys, useResendVerificationMutation } from '@/services/queries/users';
import { ApiRequestError } from '@/services/queries/rules';
import { useToast } from '@/hooks/use-toast';

type VerifyState = 'verifying' | 'verified' | 'invalid' | 'error';

/** Landing page of the emailed link: /verify-email?token=… (verifies on load). */
export default function VerifyEmail() {
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token') ?? '';
  const [state, setState] = useState<VerifyState>(token ? 'verifying' : 'invalid');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const { verifyEmail } = useAuthService();
  const { isAuthenticated } = useAuth();
  const resend = useResendVerificationMutation();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  // Tokens are single use: StrictMode's double effect must not spend it twice.
  const submitted = useRef<string | null>(null);

  useEffect(() => {
    const key = `${token}#${attempt}`;
    if (!token || submitted.current === key) return;
    submitted.current = key;
    setState('verifying');
    verifyEmail({ token }).then(res => {
      if (res.success) {
        setState('verified');
        queryClient.invalidateQueries({ queryKey: meKeys.all });
      } else if (res.code === 'invalid_verification_token' || res.status === 422) {
        setState('invalid');
      } else {
        setErrorMessage(res.error);
        setState('error');
      }
    });
  }, [token, attempt, verifyEmail, queryClient]);

  const handleResend = async () => {
    try {
      await resend.mutateAsync();
      toast({ title: 'Verification email sent', description: 'Check your inbox for a new link.' });
    } catch (err) {
      if (err instanceof ApiRequestError && err.code === 'email_already_verified') {
        setState('verified');
        queryClient.invalidateQueries({ queryKey: meKeys.all });
        return;
      }
      toast({
        title: 'Could not resend',
        description: err instanceof Error ? err.message : 'Please try again.',
        variant: 'destructive',
      });
    }
  };

  const continueLink = isAuthenticated ? '/' : '/login';
  const continueLabel = isAuthenticated ? 'Go to dashboard' : 'Continue to login';

  return (
    <div className="min-h-screen flex items-center justify-center bg-background bg-gradient-radial from-primary/10 via-background to-background p-4">
      <div className="absolute inset-0 bg-grid opacity-30" />

      <div className="w-full max-w-md relative z-10 animate-fade-in">
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-primary/10 border border-primary/20 mb-4">
            <Gamepad2 className="w-8 h-8 text-primary" />
          </div>
          <h1 className="text-3xl font-bold text-gradient">LevelUpOs</h1>
          <p className="text-muted-foreground mt-2">Universal Gamification Platform</p>
        </div>

        <Card className="glass-card">
          {state === 'verifying' && (
            <CardHeader className="space-y-1 pb-6 text-center">
              <div className="mx-auto w-16 h-16 rounded-full bg-primary/10 flex items-center justify-center mb-4">
                <Loader2 className="w-8 h-8 text-primary animate-spin" />
              </div>
              <CardTitle className="text-2xl">Verifying your email…</CardTitle>
              <CardDescription>This only takes a moment.</CardDescription>
            </CardHeader>
          )}

          {state === 'verified' && (
            <>
              <CardHeader className="space-y-1 pb-4 text-center">
                <div className="mx-auto w-16 h-16 rounded-full bg-primary/10 flex items-center justify-center mb-4">
                  <CheckCircle2 className="w-8 h-8 text-primary" />
                </div>
                <CardTitle className="text-2xl">Email verified</CardTitle>
                <CardDescription>Thanks for confirming your address.</CardDescription>
              </CardHeader>
              <CardContent>
                <Link to={continueLink} className="block">
                  <Button variant="glow" className="w-full">{continueLabel}</Button>
                </Link>
              </CardContent>
            </>
          )}

          {state === 'invalid' && (
            <>
              <CardHeader className="space-y-1 pb-4 text-center">
                <div className="mx-auto w-16 h-16 rounded-full bg-destructive/10 flex items-center justify-center mb-4">
                  <MailWarning className="w-8 h-8 text-destructive" />
                </div>
                <CardTitle className="text-2xl">Link invalid or expired</CardTitle>
                <CardDescription>
                  This verification link was already used, has expired, or a newer one was sent.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                {isAuthenticated ? (
                  <Button variant="glow" className="w-full" onClick={handleResend} disabled={resend.isPending}>
                    {resend.isPending && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
                    Send a new verification link
                  </Button>
                ) : (
                  <p className="text-sm text-muted-foreground text-center">
                    Sign in to request a new verification link.
                  </p>
                )}
                <Link to={continueLink} className="block">
                  <Button variant="outline" className="w-full">{continueLabel}</Button>
                </Link>
              </CardContent>
            </>
          )}

          {state === 'error' && (
            <>
              <CardHeader className="space-y-1 pb-4 text-center">
                <div className="mx-auto w-16 h-16 rounded-full bg-destructive/10 flex items-center justify-center mb-4">
                  <MailWarning className="w-8 h-8 text-destructive" />
                </div>
                <CardTitle className="text-2xl">Something went wrong</CardTitle>
                <CardDescription>{errorMessage || 'We could not verify your email right now.'}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <Button variant="glow" className="w-full" onClick={() => setAttempt(a => a + 1)}>
                  <RefreshCw className="w-4 h-4 mr-2" />
                  Try again
                </Button>
                <Link to={continueLink} className="block">
                  <Button variant="outline" className="w-full">{continueLabel}</Button>
                </Link>
              </CardContent>
            </>
          )}
        </Card>
      </div>
    </div>
  );
}

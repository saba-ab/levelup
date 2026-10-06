import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { useQueryClient } from '@tanstack/react-query';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Eye, EyeOff, Gamepad2, ArrowLeft, MailWarning, RefreshCw, UserPlus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useAuth } from '@/contexts/AuthContext';
import { useAuthService } from '@/services/api/auth';
import { useInvitationPreviewQuery } from '@/services/queries/users';
import { ApiRequestError } from '@/services/queries/rules';
import type { InvitationPreview } from '@/services/api/models/identity';
import { useToast } from '@/hooks/use-toast';

const acceptSchema = z
  .object({
    name: z.string().trim().min(1, 'Please enter your name').max(255),
    password: z.string().min(8, 'Password must be at least 8 characters').max(72, 'Password must be at most 72 characters'),
    confirmPassword: z.string(),
  })
  .refine(data => data.password === data.confirmPassword, {
    message: "Passwords don't match",
    path: ['confirmPassword'],
  });

type AcceptFormData = z.infer<typeof acceptSchema>;

function InvalidInvitation({ title, description }: { title: string; description: string }) {
  return (
    <>
      <CardHeader className="space-y-1 pb-4 text-center">
        <div className="mx-auto w-16 h-16 rounded-full bg-destructive/10 flex items-center justify-center mb-4">
          <MailWarning className="w-8 h-8 text-destructive" />
        </div>
        <CardTitle className="text-2xl">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <Link to="/login" className="block">
          <Button variant="outline" className="w-full">
            <ArrowLeft className="w-4 h-4 mr-2" />
            Back to login
          </Button>
        </Link>
      </CardContent>
    </>
  );
}

function AcceptForm({ token, preview, onInvalid }: { token: string; preview: InvitationPreview; onInvalid: () => void }) {
  const [showPassword, setShowPassword] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [emailTaken, setEmailTaken] = useState(false);
  const { acceptInvite } = useAuthService();
  const { loginWithSession, isAuthenticated, user } = useAuth();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const { register, handleSubmit, setError, formState: { errors } } = useForm<AcceptFormData>({
    resolver: zodResolver(acceptSchema),
    defaultValues: { name: preview.name, password: '', confirmPassword: '' },
  });

  const onSubmit = async (data: AcceptFormData) => {
    setIsLoading(true);
    setEmailTaken(false);
    const res = await acceptInvite({
      token,
      name: data.name.trim(),
      password: data.password,
      password_confirmation: data.confirmPassword,
    });
    setIsLoading(false);
    if (res.success && res.data) {
      // A previous session may have cached another account's data.
      if (isAuthenticated) queryClient.clear();
      loginWithSession(res.data);
      toast({ title: `Welcome to ${preview.tenant_name}!`, description: 'Your account is ready.' });
      navigate('/', { replace: true });
      return;
    }
    if (res.code === 'invalid_invitation_token' || res.validationErrors?.token) {
      onInvalid();
      return;
    }
    if (res.code === 'email_taken') {
      setEmailTaken(true);
      return;
    }
    if (res.validationErrors) {
      const { name, password } = res.validationErrors;
      if (name) setError('name', { message: name.join(', ') });
      if (password) setError('password', { message: password.join(', ') });
      if (name || password) return;
    }
    toast({ title: 'Could not accept the invitation', description: res.error || 'Please try again.', variant: 'destructive' });
  };

  return (
    <>
      <CardHeader className="space-y-1 pb-4 text-center">
        <div className="mx-auto w-16 h-16 rounded-full bg-primary/10 flex items-center justify-center mb-4">
          <UserPlus className="w-8 h-8 text-primary" />
        </div>
        <CardTitle className="text-2xl">Join {preview.tenant_name}</CardTitle>
        <CardDescription>
          {preview.inviter_name ? `${preview.inviter_name} invited you` : 'You were invited'} to join{' '}
          <span className="font-medium text-foreground">{preview.tenant_name}</span> on LevelUpOs.
        </CardDescription>
        {preview.roles.length > 0 && (
          <div className="flex flex-wrap justify-center gap-2 pt-2">
            {preview.roles.map(role => (
              <Badge key={role.id} variant="outline">{role.label}</Badge>
            ))}
          </div>
        )}
      </CardHeader>
      <CardContent>
        {isAuthenticated && user && (
          <p className="text-sm text-muted-foreground bg-muted/50 rounded-md p-3 mb-4">
            You're signed in as <span className="font-medium text-foreground">{user.email}</span>. Accepting signs you in
            as the invited account instead.
          </p>
        )}
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-2">
            <label htmlFor="email" className="text-sm font-medium">Email</label>
            <Input id="email" type="email" value={preview.email} readOnly disabled />
          </div>

          <div className="space-y-2">
            <label htmlFor="name" className="text-sm font-medium">Your name</label>
            <Input id="name" placeholder="Jane Doe" autoComplete="name" {...register('name')} />
            {errors.name && <p className="text-sm text-destructive">{errors.name.message}</p>}
          </div>

          <div className="space-y-2">
            <label htmlFor="password" className="text-sm font-medium">Password</label>
            <div className="relative">
              <Input
                id="password"
                type={showPassword ? 'text' : 'password'}
                placeholder="At least 8 characters"
                autoComplete="new-password"
                {...register('password')}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
              >
                {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
            {errors.password && <p className="text-sm text-destructive">{errors.password.message}</p>}
          </div>

          <div className="space-y-2">
            <label htmlFor="confirmPassword" className="text-sm font-medium">Confirm password</label>
            <Input
              id="confirmPassword"
              type={showPassword ? 'text' : 'password'}
              placeholder="••••••••"
              autoComplete="new-password"
              {...register('confirmPassword')}
            />
            {errors.confirmPassword && <p className="text-sm text-destructive">{errors.confirmPassword.message}</p>}
          </div>

          {emailTaken && (
            <p className="text-sm text-destructive">
              An account with this email already exists.{' '}
              <Link to="/login" className="underline font-medium">Sign in instead</Link>.
            </p>
          )}

          <Button type="submit" className="w-full" variant="glow" disabled={isLoading}>
            {isLoading ? (
              <div className="w-5 h-5 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
            ) : (
              'Accept invitation'
            )}
          </Button>
        </form>

        <p className="text-center text-xs text-muted-foreground mt-6">
          This invitation expires on {new Date(preview.expires_at).toLocaleString()}.
        </p>
      </CardContent>
    </>
  );
}

/** Landing page of the invitation email: /accept-invite?token=… */
export default function AcceptInvite() {
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token') ?? '';
  const [invalidated, setInvalidated] = useState(false);
  const { data: preview, isLoading, error, refetch, isFetching } = useInvitationPreviewQuery(token || null);

  const notFound = !token || invalidated || (error instanceof ApiRequestError && (error.status === 404 || error.status === 422));

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
          {notFound ? (
            <InvalidInvitation
              title="Invitation not available"
              description="This invitation link is invalid, has expired, was revoked, or was already accepted. Ask your admin to send a new one."
            />
          ) : isLoading ? (
            <CardContent className="p-6 space-y-4">
              <Skeleton className="h-16 w-16 rounded-full mx-auto" />
              <Skeleton className="h-6 w-2/3 mx-auto" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </CardContent>
          ) : error || !preview ? (
            <>
              <CardHeader className="space-y-1 pb-4 text-center">
                <div className="mx-auto w-16 h-16 rounded-full bg-destructive/10 flex items-center justify-center mb-4">
                  <MailWarning className="w-8 h-8 text-destructive" />
                </div>
                <CardTitle className="text-2xl">Something went wrong</CardTitle>
                <CardDescription>{error?.message || 'We could not load this invitation.'}</CardDescription>
              </CardHeader>
              <CardContent>
                <Button variant="glow" className="w-full" onClick={() => refetch()} disabled={isFetching}>
                  <RefreshCw className="w-4 h-4 mr-2" />
                  Try again
                </Button>
              </CardContent>
            </>
          ) : (
            <AcceptForm token={token} preview={preview} onInvalid={() => setInvalidated(true)} />
          )}
        </Card>
      </div>
    </div>
  );
}

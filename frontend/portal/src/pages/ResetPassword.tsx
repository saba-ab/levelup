import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Eye, EyeOff, Gamepad2, ArrowLeft, KeyRound, ShieldAlert } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useAuth } from '@/contexts/AuthContext';
import { useAuthService } from '@/services/api/auth';
import { useToast } from '@/hooks/use-toast';

const resetSchema = z
  .object({
    password: z.string().min(8, 'Password must be at least 8 characters').max(72, 'Password must be at most 72 characters'),
    confirmPassword: z.string(),
  })
  .refine(data => data.password === data.confirmPassword, {
    message: "Passwords don't match",
    path: ['confirmPassword'],
  });

type ResetFormData = z.infer<typeof resetSchema>;

/** Landing page of the emailed link: /reset-password?token=… */
export default function ResetPassword() {
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token') ?? '';
  const [showPassword, setShowPassword] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [invalidToken, setInvalidToken] = useState(!token);
  const { resetPassword } = useAuthService();
  const { isAuthenticated, logout } = useAuth();
  const navigate = useNavigate();
  const { toast } = useToast();

  const { register, handleSubmit, setError, formState: { errors } } = useForm<ResetFormData>({
    resolver: zodResolver(resetSchema),
  });

  const onSubmit = async (data: ResetFormData) => {
    setIsLoading(true);
    const res = await resetPassword({ token, password: data.password, password_confirmation: data.confirmPassword });
    setIsLoading(false);
    if (res.success) {
      // The reset revokes every session of the account; drop any local one too.
      if (isAuthenticated) await logout();
      toast({ title: 'Password updated', description: 'Sign in with your new password.' });
      navigate('/login', { replace: true });
      return;
    }
    if (res.code === 'invalid_reset_token' || res.validationErrors?.token) {
      setInvalidToken(true);
      return;
    }
    if (res.validationErrors?.password) {
      setError('password', { message: res.validationErrors.password.join(', ') });
      return;
    }
    toast({ title: 'Could not reset password', description: res.error || 'Please try again.', variant: 'destructive' });
  };

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
          {invalidToken ? (
            <>
              <CardHeader className="space-y-1 pb-4 text-center">
                <div className="mx-auto w-16 h-16 rounded-full bg-destructive/10 flex items-center justify-center mb-4">
                  <ShieldAlert className="w-8 h-8 text-destructive" />
                </div>
                <CardTitle className="text-2xl">Link invalid or expired</CardTitle>
                <CardDescription>
                  This reset link has already been used, has expired, or was replaced by a newer one.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <Link to="/forgot-password" className="block">
                  <Button variant="glow" className="w-full">Request a new link</Button>
                </Link>
                <Link to="/login" className="block">
                  <Button variant="outline" className="w-full">
                    <ArrowLeft className="w-4 h-4 mr-2" />
                    Back to login
                  </Button>
                </Link>
              </CardContent>
            </>
          ) : (
            <>
              <CardHeader className="space-y-1 pb-4 text-center">
                <div className="mx-auto w-16 h-16 rounded-full bg-primary/10 flex items-center justify-center mb-4">
                  <KeyRound className="w-8 h-8 text-primary" />
                </div>
                <CardTitle className="text-2xl">Set a new password</CardTitle>
                <CardDescription>You'll be signed out everywhere and can sign in with the new password.</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
                  <div className="space-y-2">
                    <label htmlFor="password" className="text-sm font-medium">New password</label>
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

                  <Button type="submit" className="w-full" variant="glow" disabled={isLoading}>
                    {isLoading ? (
                      <div className="w-5 h-5 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
                    ) : (
                      'Update password'
                    )}
                  </Button>
                </form>

                <p className="text-center text-sm text-muted-foreground mt-6">
                  <Link to="/login" className="text-primary hover:underline font-medium">
                    Back to login
                  </Link>
                </p>
              </CardContent>
            </>
          )}
        </Card>
      </div>
    </div>
  );
}

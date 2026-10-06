import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Gamepad2, ArrowLeft, KeyRound, MailCheck } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useAuthService } from '@/services/api/auth';
import { useToast } from '@/hooks/use-toast';

const forgotSchema = z.object({
  email: z.string().email('Please enter a valid email'),
});

type ForgotFormData = z.infer<typeof forgotSchema>;

/**
 * POST /auth/forgot-password always answers 202 whether or not the address
 * has an account, so the confirmation never says whether it exists.
 */
export default function ForgotPassword() {
  const [isLoading, setIsLoading] = useState(false);
  const [sentTo, setSentTo] = useState<string | null>(null);
  const { forgotPassword } = useAuthService();
  const { toast } = useToast();

  const { register, handleSubmit, formState: { errors } } = useForm<ForgotFormData>({
    resolver: zodResolver(forgotSchema),
  });

  const onSubmit = async (data: ForgotFormData) => {
    setIsLoading(true);
    const res = await forgotPassword({ email: data.email.trim() });
    setIsLoading(false);
    if (res.success) {
      setSentTo(data.email.trim());
      return;
    }
    toast({
      title: 'Could not send the reset email',
      description:
        res.status === 429
          ? 'Too many requests. Please wait a while and try again.'
          : res.validationErrors?.email?.join(', ') || res.error || 'Please try again.',
      variant: 'destructive',
    });
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
          {sentTo ? (
            <>
              <CardHeader className="space-y-1 pb-4 text-center">
                <div className="mx-auto w-16 h-16 rounded-full bg-primary/10 flex items-center justify-center mb-4">
                  <MailCheck className="w-8 h-8 text-primary" />
                </div>
                <CardTitle className="text-2xl">Check your email</CardTitle>
                <CardDescription>
                  If an account exists for <span className="font-medium text-foreground">{sentTo}</span>, we sent a link to
                  reset your password.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <p className="text-sm text-muted-foreground text-center">
                  The link works once. Didn't get it? Check your spam folder, or{' '}
                  <button type="button" className="text-primary hover:underline" onClick={() => setSentTo(null)}>
                    try again
                  </button>
                  .
                </p>
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
                <CardTitle className="text-2xl">Forgot password?</CardTitle>
                <CardDescription>Enter your email and we'll send you a link to reset it.</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
                  <div className="space-y-2">
                    <label htmlFor="email" className="text-sm font-medium">Email</label>
                    <Input id="email" type="email" placeholder="you@company.com" autoComplete="email" {...register('email')} />
                    {errors.email && <p className="text-sm text-destructive">{errors.email.message}</p>}
                  </div>

                  <Button type="submit" className="w-full" variant="glow" disabled={isLoading}>
                    {isLoading ? (
                      <div className="w-5 h-5 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
                    ) : (
                      'Send reset link'
                    )}
                  </Button>
                </form>

                <p className="text-center text-sm text-muted-foreground mt-6">
                  Remembered it?{' '}
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

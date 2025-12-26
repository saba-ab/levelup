import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Eye, EyeOff, Gamepad2, ArrowLeft, ArrowRight, Check } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { useAuth } from '@/contexts/AuthContext';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';

const step1Schema = z.object({
  email: z.string().email('Please enter a valid email'),
  password: z.string().min(8, 'Password must be at least 8 characters'),
  confirmPassword: z.string(),
}).refine(data => data.password === data.confirmPassword, {
  message: "Passwords don't match",
  path: ['confirmPassword'],
});

const step2Schema = z.object({
  tenantName: z.string().min(2, 'Company name must be at least 2 characters'),
  firstName: z.string().min(1, 'First name is required'),
  lastName: z.string().min(1, 'Last name is required'),
  acceptTerms: z.boolean().refine(val => val === true, 'You must accept the terms'),
});

type Step1Data = z.infer<typeof step1Schema>;
type Step2Data = z.infer<typeof step2Schema>;

export default function Register() {
  const [step, setStep] = useState(1);
  const [showPassword, setShowPassword] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [step1Data, setStep1Data] = useState<Step1Data | null>(null);
  const { register: registerUser } = useAuth();
  const navigate = useNavigate();
  const { toast } = useToast();

  const step1Form = useForm<Step1Data>({
    resolver: zodResolver(step1Schema),
  });

  const step2Form = useForm<Step2Data>({
    resolver: zodResolver(step2Schema),
  });

  const onStep1Submit = (data: Step1Data) => {
    setStep1Data(data);
    setStep(2);
  };

  const onStep2Submit = async (data: Step2Data) => {
    if (!step1Data) return;
    
    setIsLoading(true);
    try {
      await registerUser({
        email: step1Data.email,
        password: step1Data.password,
        firstName: data.firstName,
        lastName: data.lastName,
        tenantName: data.tenantName,
      });
      toast({
        title: "Account created!",
        description: "Welcome to LevelUpOs. Let's get started!",
      });
      navigate('/');
    } catch (error) {
      toast({
        title: "Registration failed",
        description: "Something went wrong. Please try again.",
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  const getPasswordStrength = (password: string) => {
    let strength = 0;
    if (password.length >= 8) strength++;
    if (password.match(/[a-z]/) && password.match(/[A-Z]/)) strength++;
    if (password.match(/[0-9]/)) strength++;
    if (password.match(/[^a-zA-Z0-9]/)) strength++;
    return strength;
  };

  const password = step1Form.watch('password') || '';
  const passwordStrength = getPasswordStrength(password);

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

        {/* Progress Steps */}
        <div className="flex items-center justify-center gap-2 mb-6">
          <div className={cn(
            "w-10 h-10 rounded-full flex items-center justify-center text-sm font-medium transition-all",
            step >= 1 ? "bg-primary text-primary-foreground" : "bg-secondary text-muted-foreground"
          )}>
            {step > 1 ? <Check className="w-5 h-5" /> : '1'}
          </div>
          <div className={cn(
            "w-16 h-1 rounded-full transition-all",
            step >= 2 ? "bg-primary" : "bg-secondary"
          )} />
          <div className={cn(
            "w-10 h-10 rounded-full flex items-center justify-center text-sm font-medium transition-all",
            step >= 2 ? "bg-primary text-primary-foreground" : "bg-secondary text-muted-foreground"
          )}>
            2
          </div>
        </div>

        <Card className="glass-card">
          <CardHeader className="space-y-1 pb-4">
            <CardTitle className="text-2xl text-center">
              {step === 1 ? 'Create your account' : 'Complete your profile'}
            </CardTitle>
            <CardDescription className="text-center">
              {step === 1 ? 'Enter your email and create a password' : 'Tell us about your company'}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {step === 1 ? (
              <form onSubmit={step1Form.handleSubmit(onStep1Submit)} className="space-y-4">
                <div className="space-y-2">
                  <label htmlFor="email" className="text-sm font-medium">Email</label>
                  <Input
                    id="email"
                    type="email"
                    placeholder="you@company.com"
                    {...step1Form.register('email')}
                  />
                  {step1Form.formState.errors.email && (
                    <p className="text-sm text-destructive">{step1Form.formState.errors.email.message}</p>
                  )}
                </div>

                <div className="space-y-2">
                  <label htmlFor="password" className="text-sm font-medium">Password</label>
                  <div className="relative">
                    <Input
                      id="password"
                      type={showPassword ? 'text' : 'password'}
                      placeholder="••••••••"
                      {...step1Form.register('password')}
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                    </button>
                  </div>
                  {/* Password Strength Indicator */}
                  {password && (
                    <div className="space-y-1">
                      <div className="flex gap-1">
                        {[1, 2, 3, 4].map(i => (
                          <div
                            key={i}
                            className={cn(
                              "h-1 flex-1 rounded-full transition-colors",
                              i <= passwordStrength
                                ? passwordStrength <= 1 ? "bg-destructive"
                                  : passwordStrength <= 2 ? "bg-warning"
                                  : "bg-green-500"
                                : "bg-secondary"
                            )}
                          />
                        ))}
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {passwordStrength <= 1 ? 'Weak' : passwordStrength <= 2 ? 'Fair' : passwordStrength <= 3 ? 'Good' : 'Strong'}
                      </p>
                    </div>
                  )}
                  {step1Form.formState.errors.password && (
                    <p className="text-sm text-destructive">{step1Form.formState.errors.password.message}</p>
                  )}
                </div>

                <div className="space-y-2">
                  <label htmlFor="confirmPassword" className="text-sm font-medium">Confirm Password</label>
                  <Input
                    id="confirmPassword"
                    type="password"
                    placeholder="••••••••"
                    {...step1Form.register('confirmPassword')}
                  />
                  {step1Form.formState.errors.confirmPassword && (
                    <p className="text-sm text-destructive">{step1Form.formState.errors.confirmPassword.message}</p>
                  )}
                </div>

                <Button type="submit" className="w-full" variant="glow">
                  Continue
                  <ArrowRight className="w-4 h-4 ml-2" />
                </Button>
              </form>
            ) : (
              <form onSubmit={step2Form.handleSubmit(onStep2Submit)} className="space-y-4">
                <div className="space-y-2">
                  <label htmlFor="tenantName" className="text-sm font-medium">Company Name</label>
                  <Input
                    id="tenantName"
                    placeholder="Acme Inc."
                    {...step2Form.register('tenantName')}
                  />
                  {step2Form.formState.errors.tenantName && (
                    <p className="text-sm text-destructive">{step2Form.formState.errors.tenantName.message}</p>
                  )}
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-2">
                    <label htmlFor="firstName" className="text-sm font-medium">First Name</label>
                    <Input
                      id="firstName"
                      placeholder="John"
                      {...step2Form.register('firstName')}
                    />
                    {step2Form.formState.errors.firstName && (
                      <p className="text-sm text-destructive">{step2Form.formState.errors.firstName.message}</p>
                    )}
                  </div>
                  <div className="space-y-2">
                    <label htmlFor="lastName" className="text-sm font-medium">Last Name</label>
                    <Input
                      id="lastName"
                      placeholder="Doe"
                      {...step2Form.register('lastName')}
                    />
                    {step2Form.formState.errors.lastName && (
                      <p className="text-sm text-destructive">{step2Form.formState.errors.lastName.message}</p>
                    )}
                  </div>
                </div>

                <div className="flex items-start gap-2">
                  <Checkbox 
                    id="terms" 
                    onCheckedChange={(checked) => step2Form.setValue('acceptTerms', !!checked)}
                  />
                  <label htmlFor="terms" className="text-sm text-muted-foreground leading-tight">
                    I agree to the{' '}
                    <a href="#" className="text-primary hover:underline">Terms of Service</a>
                    {' '}and{' '}
                    <a href="#" className="text-primary hover:underline">Privacy Policy</a>
                  </label>
                </div>
                {step2Form.formState.errors.acceptTerms && (
                  <p className="text-sm text-destructive">{step2Form.formState.errors.acceptTerms.message}</p>
                )}

                <div className="flex gap-3">
                  <Button type="button" variant="outline" onClick={() => setStep(1)} className="flex-1">
                    <ArrowLeft className="w-4 h-4 mr-2" />
                    Back
                  </Button>
                  <Button type="submit" className="flex-1" variant="glow" disabled={isLoading}>
                    {isLoading ? (
                      <div className="w-5 h-5 border-2 border-primary-foreground/30 border-t-primary-foreground rounded-full animate-spin" />
                    ) : (
                      'Create Account'
                    )}
                  </Button>
                </div>
              </form>
            )}

            <p className="text-center text-sm text-muted-foreground mt-6">
              Already have an account?{' '}
              <Link to="/login" className="text-primary hover:underline font-medium">
                Sign in
              </Link>
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

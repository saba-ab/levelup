import { Link } from 'react-router-dom';
import { Gamepad2, ArrowLeft, KeyRound } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

/**
 * Self-service password reset is not available in this API version (there is
 * no reset endpoint). A tenant owner or admin can set a new password from
 * Settings → Team, so this page explains that instead of faking an email.
 */
export default function ForgotPassword() {
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
          <CardHeader className="space-y-1 pb-4 text-center">
            <div className="mx-auto w-16 h-16 rounded-full bg-primary/10 flex items-center justify-center mb-4">
              <KeyRound className="w-8 h-8 text-primary" />
            </div>
            <CardTitle className="text-2xl">Forgot password?</CardTitle>
            <CardDescription>
              Password reset by email is coming soon; it is not available in this API version yet.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground text-center">
              Ask your workspace owner or an admin to set a new password for you from Settings → Team.
            </p>
            <Link to="/login" className="block">
              <Button variant="outline" className="w-full">
                <ArrowLeft className="w-4 h-4 mr-2" />
                Back to login
              </Button>
            </Link>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

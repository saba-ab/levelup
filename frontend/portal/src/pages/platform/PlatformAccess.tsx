import type { ReactNode } from 'react';
import { ShieldAlert } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { useIsPlatformAdmin } from './platform-utils';

export function PlatformForbiddenCard() {
  return (
    <Card>
      <CardContent className="p-12 text-center">
        <div className="w-16 h-16 rounded-full bg-destructive/10 flex items-center justify-center mx-auto mb-4">
          <ShieldAlert className="w-8 h-8 text-destructive" />
        </div>
        <p className="text-sm font-mono text-muted-foreground mb-1">403</p>
        <h3 className="text-lg font-medium mb-2">Platform admins only</h3>
        <p className="text-muted-foreground max-w-md mx-auto">
          This area manages every tenant and the global event catalogue. It requires the platform admin role.
        </p>
      </CardContent>
    </Card>
  );
}

/** Renders children for platform admins, a 403 card for everyone else. */
export function PlatformGuard({ children }: { children: ReactNode }) {
  const isPlatformAdmin = useIsPlatformAdmin();
  if (!isPlatformAdmin) {
    return (
      <div className="space-y-6 animate-fade-in">
        <PlatformForbiddenCard />
      </div>
    );
  }
  return <>{children}</>;
}

/** Messages listed under a form field. */
export function FieldError({ messages }: { messages?: string[] }) {
  if (!messages?.length) return null;
  return (
    <ul className="space-y-0.5">
      {messages.map(m => (
        <li key={m} className="text-xs text-destructive whitespace-pre-line">
          {m}
        </li>
      ))}
    </ul>
  );
}

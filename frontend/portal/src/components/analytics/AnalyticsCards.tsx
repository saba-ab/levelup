import type { ReactNode } from 'react';
import { AlertCircle } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { formatNumber } from './chartTheme';

interface KpiCardProps {
  label: string;
  value: number | string;
  hint?: string;
  icon: ReactNode;
}

export function KpiCard({ label, value, hint, icon }: KpiCardProps) {
  return (
    <Card>
      <CardContent className="p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 flex-col gap-1">
            <p className="text-sm text-muted-foreground">{label}</p>
            <p className="text-2xl font-bold tabular-nums text-foreground">
              {typeof value === 'number' ? formatNumber(value) : value}
            </p>
            {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
          </div>
          <div className="rounded-lg bg-primary/10 p-2 text-primary">{icon}</div>
        </div>
      </CardContent>
    </Card>
  );
}

/** Loading / error placeholder for a whole tab. */
export function TabState({ loading, error }: { loading: boolean; error: string | null }) {
  if (loading) {
    return (
      <div className="flex flex-col gap-4">
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24" />
          ))}
        </div>
        <Skeleton className="h-[320px]" />
      </div>
    );
  }
  return (
    <Card>
      <CardContent className="flex items-center justify-center gap-2 py-12 text-sm text-destructive">
        <AlertCircle className="h-4 w-4" />
        {error}
      </CardContent>
    </Card>
  );
}

export function EmptyChart({ message = 'No data in this range.' }: { message?: string }) {
  return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">{message}</div>;
}

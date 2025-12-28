import { ReactNode } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';

interface StatCardProps {
  icon: ReactNode;
  value: string | number;
  label: string;
  iconClassName?: string;
  className?: string;
}

export function StatCard({ icon, value, label, iconClassName, className }: StatCardProps) {
  return (
    <Card className={className}>
      <CardContent className="pt-6 text-center">
        <div className={cn('w-6 h-6 mx-auto mb-2', iconClassName)}>{icon}</div>
        <p className="text-2xl font-bold">{typeof value === 'number' ? value.toLocaleString() : value}</p>
        <p className="text-xs text-muted-foreground">{label}</p>
      </CardContent>
    </Card>
  );
}

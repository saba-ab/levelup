import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { getLevelColor } from '@/lib/player-utils';

interface LevelBadgeProps {
  level: string;
  className?: string;
  size?: 'sm' | 'md' | 'lg';
}

const sizeClasses = {
  sm: 'text-xs px-2 py-0.5',
  md: 'text-sm px-2.5 py-0.5',
  lg: 'text-sm px-3 py-1',
};

export function LevelBadge({ level, className, size = 'md' }: LevelBadgeProps) {
  return (
    <Badge
      variant="outline"
      className={cn('capitalize', getLevelColor(level), sizeClasses[size], className)}
    >
      {level}
    </Badge>
  );
}

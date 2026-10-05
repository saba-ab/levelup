import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { getLevelColor } from '@/lib/player-utils';

interface LevelBadgeProps {
  /** Level name from the API, e.g. "Rookie". */
  level: string;
  /** level_number; drives the colour. */
  levelNumber?: number | null;
  className?: string;
  size?: 'sm' | 'md' | 'lg';
}

const sizeClasses = {
  sm: 'text-xs px-2 py-0.5',
  md: 'text-sm px-2.5 py-0.5',
  lg: 'text-sm px-3 py-1',
};

export function LevelBadge({ level, levelNumber, className, size = 'md' }: LevelBadgeProps) {
  return (
    <Badge
      variant="outline"
      className={cn('capitalize', getLevelColor(levelNumber), sizeClasses[size], className)}
    >
      {level}
    </Badge>
  );
}

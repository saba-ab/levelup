import { cn } from '@/lib/utils';
import { getPlayerInitials } from '@/lib/player-utils';

interface PlayerAvatarProps {
  /** Display name (or email) the initials are derived from. */
  name: string | null | undefined;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const sizeClasses = {
  sm: 'w-8 h-8 text-sm',
  md: 'w-10 h-10 text-sm',
  lg: 'w-20 h-20 text-3xl',
};

export function PlayerAvatar({ name, size = 'md', className }: PlayerAvatarProps) {
  return (
    <div
      className={cn(
        'rounded-full bg-primary/10 flex items-center justify-center font-medium shrink-0',
        sizeClasses[size],
        className
      )}
    >
      {getPlayerInitials(name)}
    </div>
  );
}

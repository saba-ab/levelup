import { cn } from '@/lib/utils';
import { getPlayerInitial } from '@/lib/player-utils';

interface PlayerAvatarProps {
  email: string;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const sizeClasses = {
  sm: 'w-8 h-8 text-sm',
  md: 'w-10 h-10 text-sm',
  lg: 'w-20 h-20 text-3xl',
};

export function PlayerAvatar({ email, size = 'md', className }: PlayerAvatarProps) {
  return (
    <div
      className={cn(
        'rounded-full bg-primary/10 flex items-center justify-center font-medium',
        sizeClasses[size],
        className
      )}
    >
      {getPlayerInitial(email)}
    </div>
  );
}

// Player-related utility functions and constants

export type PlayerLevel = 'diamond' | 'platinum' | 'gold' | 'silver' | 'bronze';

export type ActivityType = 'points' | 'badge' | 'mission' | 'level' | 'streak' | 'reward';

export const LEVEL_THRESHOLDS = {
  diamond: 50000,
  platinum: 15000,
  gold: 5000,
  silver: 1000,
  bronze: 0,
} as const;

export const LEVEL_ORDER: PlayerLevel[] = ['bronze', 'silver', 'gold', 'platinum', 'diamond'];

export function getLevelColor(level: string): string {
  switch (level.toLowerCase()) {
    case 'diamond':
      return 'border-cyan-400 text-cyan-400 bg-cyan-400/10';
    case 'platinum':
      return 'border-slate-300 text-slate-300 bg-slate-300/10';
    case 'gold':
      return 'border-yellow-500 text-yellow-500 bg-yellow-500/10';
    case 'silver':
      return 'border-gray-400 text-gray-400 bg-gray-400/10';
    default:
      return 'border-amber-700 text-amber-700 bg-amber-700/10';
  }
}

export function getActivityIcon(type: ActivityType | string): string {
  switch (type) {
    case 'points':
      return '⚡';
    case 'badge':
      return '🏅';
    case 'mission':
      return '🎯';
    case 'level':
      return '🎖️';
    case 'streak':
      return '🔥';
    case 'reward':
      return '🎁';
    default:
      return '📌';
  }
}

export function calculateLevelProgress(currentXp: number, currentLevel: string) {
  const level = currentLevel.toLowerCase() as PlayerLevel;
  
  if (level === 'diamond') {
    return { progress: 100, nextLevelXp: currentXp, currentLevelXp: LEVEL_THRESHOLDS.diamond };
  }

  const levelIndex = LEVEL_ORDER.indexOf(level);
  const nextLevel = LEVEL_ORDER[levelIndex + 1];
  const currentLevelXp = LEVEL_THRESHOLDS[level];
  const nextLevelXp = LEVEL_THRESHOLDS[nextLevel];

  const progress = ((currentXp - currentLevelXp) / (nextLevelXp - currentLevelXp)) * 100;
  
  return {
    progress: Math.min(100, Math.max(0, progress)),
    nextLevelXp,
    currentLevelXp,
    xpToNext: nextLevelXp - currentXp,
  };
}

export function getPlayerInitial(email: string): string {
  return email.charAt(0).toUpperCase();
}

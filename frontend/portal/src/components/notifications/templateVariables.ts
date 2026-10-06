import type { NotificationChannel, NotificationStatus, NotificationTrigger } from '@/services/api/models/notifications';

export const TRIGGER_LABELS: Record<NotificationTrigger, string> = {
  'badges.awarded': 'Badge awarded',
  'progression.level_reached': 'Level reached',
  'missions.completed': 'Mission completed',
  'streaks.milestone_reached': 'Streak milestone reached',
  'streaks.broken': 'Streak broken',
  'rewards.claimed': 'Reward claimed',
  'points.credited': 'Points credited',
};

export const CHANNEL_LABELS: Record<NotificationChannel, string> = {
  in_app: 'In-app',
  email: 'Email',
};

export const STATUS_STYLES: Record<NotificationStatus, string> = {
  delivered: 'bg-green-500/15 text-green-600 border-green-500/30 dark:text-green-400',
  pending: 'bg-amber-500/15 text-amber-600 border-amber-500/30 dark:text-amber-400',
  failed: 'bg-destructive/15 text-destructive border-destructive/30',
  skipped: 'bg-muted text-muted-foreground border-border',
};

export interface TemplateVariable {
  /** Go template expression, e.g. ".Badge.Name". */
  path: string;
  description: string;
}

export interface TemplateVariableGroup {
  label: string;
  variables: TemplateVariable[];
}

const v = (path: string, description: string): TemplateVariable => ({ path, description });

const PLAYER: TemplateVariableGroup = {
  label: 'Player',
  variables: [
    v('.Player.DisplayName', 'Display name'),
    v('.Player.ExternalID', 'Your external id'),
    v('.Player.ID', 'Player id (uuid)'),
  ],
};

const BADGE: TemplateVariableGroup = {
  label: 'Badge',
  variables: [
    v('.Badge.Name', 'Badge slug (the event carries no display name)'),
    v('.Badge.Slug', 'Slug'),
    v('.Badge.Tier', 'Tier'),
    v('.Badge.Category', 'Category'),
    v('.Badge.EarnedCount', 'Times earned'),
    v('.Badge.PointsValue', 'Points value'),
    v('.Badge.ID', 'Badge id'),
  ],
};

const LEVEL: TemplateVariableGroup = {
  label: 'Level',
  variables: [
    v('.Level.Number', 'Level number'),
    v('.Level.Name', 'Level name'),
    v('.Level.TotalXP', 'Player total XP'),
    v('.Level.PointsReward', 'Points reward'),
    v('.Level.ID', 'Level id'),
  ],
};

const POINTS: TemplateVariableGroup = {
  label: 'Points',
  variables: [
    v('.Points.Amount', 'Amount credited'),
    v('.Points.Balance', 'Balance after'),
    v('.Points.LifetimeEarned', 'Lifetime earned'),
    v('.Points.Kind', 'Ledger kind'),
  ],
};

const MISSION: TemplateVariableGroup = {
  label: 'Mission',
  variables: [
    v('.Mission.Name', 'Mission slug (the event carries no display name)'),
    v('.Mission.Slug', 'Slug'),
    v('.Mission.PointsReward', 'Points reward'),
    v('.Mission.XPReward', 'XP reward'),
    v('.Mission.ID', 'Mission id'),
  ],
};

const STREAK_MILESTONE: TemplateVariableGroup = {
  label: 'Streak',
  variables: [
    v('.Streak.Milestone', 'Milestone reached'),
    v('.Streak.BonusPoints', 'Bonus points'),
    v('.Streak.ID', 'Streak id'),
  ],
};

const STREAK_BROKEN: TemplateVariableGroup = {
  label: 'Streak',
  variables: [
    v('.Streak.FinalCount', 'Count when broken'),
    v('.Streak.ID', 'Streak id'),
  ],
};

const REWARD: TemplateVariableGroup = {
  label: 'Reward',
  variables: [
    v('.Reward.Name', 'Reward slug (the event carries no display name)'),
    v('.Reward.Slug', 'Slug'),
    v('.Reward.Type', 'Type'),
    v('.Reward.Code', 'Redemption code'),
    v('.Reward.Value', 'Value'),
    v('.Reward.PointsCost', 'Points cost'),
    v('.Reward.ID', 'Reward id'),
  ],
};

/** The fields each trigger populates (others render as zero values). */
export const TRIGGER_VARIABLES: Record<NotificationTrigger, TemplateVariableGroup[]> = {
  'badges.awarded': [PLAYER, BADGE],
  'progression.level_reached': [PLAYER, LEVEL],
  'missions.completed': [PLAYER, MISSION],
  'streaks.milestone_reached': [PLAYER, STREAK_MILESTONE],
  'streaks.broken': [PLAYER, STREAK_BROKEN],
  'rewards.claimed': [PLAYER, REWARD],
  'points.credited': [PLAYER, POINTS],
};

/** Mirrors the backend's SampleData (what validation and previews use). */
const SAMPLE: Record<string, Record<string, string | number>> = {
  Player: { ID: '00000000-0000-0000-0000-000000000001', ExternalID: 'player-42', DisplayName: 'Alex' },
  Badge: { ID: '00000000-0000-0000-0000-0000000000b1', Slug: 'first-steps', Name: 'first-steps', Tier: 'gold', Category: 'achievement', EarnedCount: 1, PointsValue: 100 },
  Level: { ID: '00000000-0000-0000-0000-0000000000c1', Number: 5, Name: 'Explorer', TotalXP: 1200, PointsReward: 50 },
  Points: { Amount: 250, Balance: 1250, LifetimeEarned: 4000, Kind: 'earn' },
  Mission: { ID: '00000000-0000-0000-0000-0000000000d1', Slug: 'weekly-quest', Name: 'weekly-quest', PointsReward: 300, XPReward: 150 },
  Streak: { ID: '00000000-0000-0000-0000-0000000000e1', Milestone: 7, BonusPoints: 70, FinalCount: 12 },
  Reward: { ID: '00000000-0000-0000-0000-0000000000f1', Slug: 'free-coffee', Name: 'free-coffee', Type: 'coupon', Code: 'COFFEE-1234', Value: '1', PointsCost: 500 },
};

/**
 * Approximate client-side render for the editor's live preview: plain field
 * actions ({{.Badge.Name}}) get sample values, control actions ({{if}},
 * {{end}}, ...) are dropped. The server preview is authoritative.
 */
export function renderTemplateLocally(src: string, trigger: NotificationTrigger): string {
  return src.replace(/\{\{-?\s*([\s\S]*?)\s*-?\}\}/g, (raw, inner: string) => {
    if (inner === '.Trigger') return trigger;
    const field = /^\.(\w+)\.(\w+)$/.exec(inner);
    if (field) {
      const value = SAMPLE[field[1]]?.[field[2]];
      return value === undefined ? raw : String(value);
    }
    if (/^(if|else|end|with|range)\b/.test(inner) || inner.startsWith('/*')) return '';
    return raw;
  });
}

import { Award, Gift, GitBranch, Target, TrendingUp, Users, type LucideIcon } from 'lucide-react';
import type { Permission } from '@/lib/permissions';
import type { AIDraftKind } from '@/services/api/models/ai';

export interface AIKindMeta {
  label: string;
  plural: string;
  icon: LucideIcon;
  /** Portal permission needed to create the entity a draft describes. */
  createPermission: Permission;
  /** Where the created entity is managed. */
  path: (id: string) => string;
}

export const AI_KIND_META: Record<AIDraftKind, AIKindMeta> = {
  badge: { label: 'Badge', plural: 'Badges', icon: Award, createPermission: 'manage:mechanics', path: () => '/mechanics/badges' },
  level: { label: 'Level', plural: 'Levels', icon: TrendingUp, createPermission: 'manage:mechanics', path: () => '/mechanics/levels' },
  mission: { label: 'Mission', plural: 'Missions', icon: Target, createPermission: 'manage:mechanics', path: () => '/mechanics/missions' },
  reward: { label: 'Reward', plural: 'Rewards', icon: Gift, createPermission: 'manage:mechanics', path: () => '/mechanics/rewards' },
  rule: { label: 'Rule', plural: 'Rules', icon: GitBranch, createPermission: 'manage:rules', path: (id) => `/rules/${id}` },
  segment: { label: 'Segment', plural: 'Segments', icon: Users, createPermission: 'manage:segments', path: () => '/segments' },
};

/**
 * Best-effort kind from a dialog's title/context ("Generate Badge Ideas"),
 * for call sites that predate the `kind` prop. Undefined when none matches.
 */
export function inferAIKind(...texts: (string | undefined)[]): AIDraftKind | undefined {
  const haystack = texts.filter(Boolean).join(' ').toLowerCase();
  const order: AIDraftKind[] = ['badge', 'level', 'mission', 'reward', 'rule', 'segment'];
  return order.find((k) => new RegExp(`\\b${k}s?\\b`).test(haystack));
}

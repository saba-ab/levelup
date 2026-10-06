// Notifications module (backend/internal/modules/notifications): templates
// rendered with Go text/template, per-tenant channel settings, delivery
// history and stats.
import type { CursorParams, ID } from './common';

/** The facts a template can react to (event topic without the version). */
export type NotificationTrigger =
  | 'badges.awarded'
  | 'progression.level_reached'
  | 'missions.completed'
  | 'streaks.milestone_reached'
  | 'streaks.broken'
  | 'rewards.claimed'
  | 'points.credited';

export const NOTIFICATION_TRIGGERS: NotificationTrigger[] = [
  'badges.awarded',
  'progression.level_reached',
  'missions.completed',
  'streaks.milestone_reached',
  'streaks.broken',
  'rewards.claimed',
  'points.credited',
];

export type NotificationChannel = 'in_app' | 'email';

export const NOTIFICATION_CHANNELS: NotificationChannel[] = ['in_app', 'email'];

export type NotificationStatus = 'pending' | 'delivered' | 'failed' | 'skipped';

export const NOTIFICATION_STATUSES: NotificationStatus[] = ['pending', 'delivered', 'failed', 'skipped'];

export interface NotificationTemplate {
  id: ID;
  name: string;
  trigger: NotificationTrigger;
  channels: NotificationChannel[];
  /** Go text/template source, e.g. "You earned {{.Badge.Name}}!". */
  title_template: string;
  /** Go text/template source; may be empty. */
  body_template: string;
  is_active: boolean;
  created_by?: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface NotificationTemplateFilters extends CursorParams {
  trigger?: NotificationTrigger;
  is_active?: boolean;
}

/** POST /notifications/templates (is_active defaults to true). */
export interface CreateNotificationTemplateData {
  name: string;
  trigger: NotificationTrigger;
  channels: NotificationChannel[];
  title_template: string;
  body_template?: string;
  is_active?: boolean;
}

/** PATCH /notifications/templates/{id}: omitted fields stay untouched. */
export type UpdateNotificationTemplateData = Partial<CreateNotificationTemplateData>;

/** POST /notifications/templates/{id}/preview body. */
export interface NotificationPreviewData {
  player_id?: ID;
}

export interface NotificationPreview {
  title: string;
  body: string;
  /** Email HTML rendition of the body. */
  html: string;
}

/** GET/PATCH /notifications/channels. in_app is always enabled. */
export interface NotificationChannelSettings {
  in_app: { enabled: boolean };
  email: { enabled: boolean; from_name: string };
  updated_at: string | null;
}

export interface UpdateNotificationChannelsData {
  email: { enabled?: boolean; from_name?: string };
}

/** One notification row of the delivery history. */
export interface NotificationRecord {
  id: ID;
  template_id: ID;
  template_name: string;
  player_id: ID;
  event_id: ID;
  trigger: NotificationTrigger;
  channel: NotificationChannel;
  status: NotificationStatus;
  reason?: string;
  title: string;
  body: string;
  attempts: number;
  last_error?: string;
  delivered_at: string | null;
  read_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface NotificationHistoryFilters extends CursorParams {
  template_id?: ID;
  status?: NotificationStatus;
  channel?: NotificationChannel;
  player_id?: ID;
}

export interface NotificationCounts {
  /** Every non-skipped notification. */
  sent: number;
  delivered: number;
  failed: number;
  pending: number;
  skipped: number;
  read: number;
}

export interface NotificationTemplateCounts extends NotificationCounts {
  template_id: ID;
  name: string;
  trigger: NotificationTrigger;
}

export interface NotificationStats extends NotificationCounts {
  by_channel: Record<NotificationChannel, NotificationCounts>;
  by_template: NotificationTemplateCounts[];
  /** read / delivered over in_app, 0..1. */
  open_rate: number;
}

/** RFC 3339 bounds: created at or after `from`, before `to`. */
export interface NotificationStatsParams {
  from?: string;
  to?: string;
}

// Analytics (backend/internal/modules/analytics): read-only dashboards over
// inclusive UTC day ranges. Days are "YYYY-MM-DD".

/** from/to: YYYY-MM-DD (UTC). Max 366 days; default the 30 days ending today. */
export interface AnalyticsRange {
  from?: string;
  to?: string;
}

export interface AnalyticsOverviewTotals {
  activities: number;
  active_players: number;
  new_players: number;
  points_credited: number;
  points_debited: number;
  badges_awarded: number;
  missions_started: number;
  missions_completed: number;
  levels_reached: number;
  rewards_claimed: number;
}

export interface AnalyticsOverviewDay {
  day: string;
  activities: number;
  active_players: number;
  points_credited: number;
  points_debited: number;
  new_players: number;
}

export interface AnalyticsOverview {
  from: string;
  to: string;
  totals: AnalyticsOverviewTotals;
  daily: AnalyticsOverviewDay[];
  /** Distinct active players ending at `to` over 1 / 7 / 30 days. */
  dau: number;
  wau: number;
  mau: number;
}

export interface AnalyticsDayCount {
  day: string;
  count: number;
}

export interface AnalyticsEngagementDay {
  day: string;
  badges_awarded: number;
  missions_started: number;
  missions_completed: number;
  levels_reached: number;
  rewards_claimed: number;
}

export interface AnalyticsEngagement {
  from: string;
  to: string;
  badges_per_day: AnalyticsDayCount[];
  missions: { started: number; completed: number };
  levels_reached: number;
  rewards_claimed: number;
  top_event_types: { event_type: string; count: number }[];
  top_badges: { badge_id: string; count: number }[];
  top_missions: { mission_id: string; count: number }[];
  levels_by_number: { level_number: string; count: number }[];
  daily: AnalyticsEngagementDay[];
}

export interface AnalyticsRetentionParams {
  /** Only "week" is supported. */
  cohort?: 'week';
  /** Number of cohorts, 1-52 (default 8). */
  weeks?: number;
}

export interface AnalyticsCohort {
  /** Monday of the ISO week (UTC). */
  cohort_start: string;
  size: number;
  /** retained[k]: percent (0-100) active k weeks later; only started weeks. */
  retained: number[];
}

export interface AnalyticsCurvePoint {
  week: number;
  pct: number;
  /** Cohorts contributing to this week offset. */
  cohorts: number;
}

export interface AnalyticsRetention {
  cohort: string;
  weeks: number;
  cohorts: AnalyticsCohort[];
  curve: AnalyticsCurvePoint[];
}

export interface AnalyticsFunnelParams extends AnalyticsRange {
  /** 2-10 event-type slugs, in order. */
  steps: string[];
}

export interface AnalyticsFunnelStep {
  event_type: string;
  players: number;
  pct_of_previous: number;
  pct_of_first_step: number;
}

export interface AnalyticsFunnel {
  from: string;
  to: string;
  steps: AnalyticsFunnelStep[];
  /** "day": order is resolved per UTC day, same-day steps count as in order. */
  approximation: 'day';
}

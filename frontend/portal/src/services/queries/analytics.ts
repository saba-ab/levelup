import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { useAnalyticsService } from '@/services/api/analytics';
import { useMechanicsService } from '@/services/api/mechanics';
import { fetchAllPages } from '@/services/api/pagination';
import type {
  AnalyticsRange,
  AnalyticsRetentionParams,
  AnalyticsFunnelParams,
} from '@/services/api/models/analytics';
import { ApiRequestError, unwrap } from './rules';

const analyticsKeys = {
  all: ['analytics'] as const,
  overview: (range: AnalyticsRange) => [...analyticsKeys.all, 'overview', range] as const,
  engagement: (range: AnalyticsRange) => [...analyticsKeys.all, 'engagement', range] as const,
  retention: (params: AnalyticsRetentionParams) => [...analyticsKeys.all, 'retention', params] as const,
  funnel: (params: AnalyticsFunnelParams) => [...analyticsKeys.all, 'funnel', params] as const,
  allMissions: () => [...analyticsKeys.all, 'mission-names'] as const,
};

/** Dashboards are aggregated server-side; refetching every minute is plenty. */
const STALE_MS = 60_000;

const ERROR_MESSAGES: Record<string, string> = {
  analytics_invalid_range: 'The date range is invalid: "from" must not be after "to".',
  analytics_range_too_large: 'The date range may span at most 366 days.',
  analytics_invalid_cohort: 'Retention supports weekly cohorts for 1 to 52 weeks.',
  analytics_invalid_funnel_steps: 'A funnel needs 2 to 10 event types.',
};

/** A user-facing message for a failed analytics call. */
export function describeAnalyticsError(err: unknown, fallback: string): string {
  if (err instanceof ApiRequestError) {
    if (err.code && ERROR_MESSAGES[err.code]) return ERROR_MESSAGES[err.code];
    if (err.status === 403) return 'You do not have permission to view analytics.';
    return err.message;
  }
  return fallback;
}

export function useAnalyticsOverviewQuery(range: AnalyticsRange, enabled = true) {
  const { getOverview } = useAnalyticsService();
  return useQuery({
    queryKey: analyticsKeys.overview(range),
    queryFn: async () => unwrap(await getOverview(range), 'Failed to load analytics overview'),
    placeholderData: keepPreviousData,
    staleTime: STALE_MS,
    enabled,
  });
}

export function useAnalyticsEngagementQuery(range: AnalyticsRange, enabled = true) {
  const { getEngagement } = useAnalyticsService();
  return useQuery({
    queryKey: analyticsKeys.engagement(range),
    queryFn: async () => unwrap(await getEngagement(range), 'Failed to load engagement analytics'),
    placeholderData: keepPreviousData,
    staleTime: STALE_MS,
    enabled,
  });
}

export function useAnalyticsRetentionQuery(params: AnalyticsRetentionParams, enabled = true) {
  const { getRetention } = useAnalyticsService();
  return useQuery({
    queryKey: analyticsKeys.retention(params),
    queryFn: async () => unwrap(await getRetention(params), 'Failed to load retention'),
    placeholderData: keepPreviousData,
    staleTime: STALE_MS,
    enabled,
  });
}

/** Disabled until at least two steps are chosen. */
export function useAnalyticsFunnelQuery(params: AnalyticsFunnelParams, enabled = true) {
  const { getFunnel } = useAnalyticsService();
  return useQuery({
    queryKey: analyticsKeys.funnel(params),
    queryFn: async () => unwrap(await getFunnel(params), 'Failed to load funnel'),
    placeholderData: keepPreviousData,
    staleTime: STALE_MS,
    enabled: enabled && params.steps.length >= 2,
    retry: false,
  });
}

/** Every mission (walks all pages), to name the engagement breakdown's ids. */
export function useAllMissionsForAnalyticsQuery(enabled = true) {
  const { listMissions } = useMechanicsService();
  return useQuery({
    queryKey: analyticsKeys.allMissions(),
    queryFn: async () =>
      unwrap(await fetchAllPages((cursor) => listMissions({ limit: 100, cursor })), 'Failed to fetch missions').data,
    staleTime: 5 * 60_000,
    enabled,
  });
}

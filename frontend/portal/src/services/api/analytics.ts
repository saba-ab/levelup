import { useCallback, useMemo } from 'react';
import { useApi } from '@/hooks/useApi';
import { API_VERSION, toQuery } from '@/lib/api-routes';
import type {
  AnalyticsRange,
  AnalyticsOverview,
  AnalyticsEngagement,
  AnalyticsRetention,
  AnalyticsRetentionParams,
  AnalyticsFunnel,
  AnalyticsFunnelParams,
} from './models/analytics';

export const ANALYTICS_ENDPOINTS = {
  OVERVIEW: `${API_VERSION}/analytics/overview`,
  ENGAGEMENT: `${API_VERSION}/analytics/engagement`,
  RETENTION: `${API_VERSION}/analytics/retention`,
  FUNNEL: `${API_VERSION}/analytics/funnel`,
} as const;

/** Pages render query errors inline. */
const QUIET = { showErrorToast: false } as const;

export function useAnalyticsService() {
  const api = useApi();

  const getOverview = useCallback(
    (range?: AnalyticsRange) =>
      api.get<AnalyticsOverview>(`${ANALYTICS_ENDPOINTS.OVERVIEW}${toQuery(range)}`, QUIET),
    [api],
  );

  const getEngagement = useCallback(
    (range?: AnalyticsRange) =>
      api.get<AnalyticsEngagement>(`${ANALYTICS_ENDPOINTS.ENGAGEMENT}${toQuery(range)}`, QUIET),
    [api],
  );

  const getRetention = useCallback(
    (params?: AnalyticsRetentionParams) =>
      api.get<AnalyticsRetention>(`${ANALYTICS_ENDPOINTS.RETENTION}${toQuery(params)}`, QUIET),
    [api],
  );

  const getFunnel = useCallback(
    ({ steps, ...range }: AnalyticsFunnelParams) =>
      api.get<AnalyticsFunnel>(`${ANALYTICS_ENDPOINTS.FUNNEL}${toQuery({ ...range, steps: steps.join(',') })}`, QUIET),
    [api],
  );

  return useMemo(
    () => ({ getOverview, getEngagement, getRetention, getFunnel }),
    [getOverview, getEngagement, getRetention, getFunnel],
  );
}

import { useCallback, useMemo } from 'react';
import { useApi } from '@/hooks/useApi';
import { API_VERSION, toQuery } from '@/lib/api-routes';
import type { CursorPage } from './models/common';
import type {
  AIDraft,
  AIDraftRequest,
  AIDraftResponse,
  AIPromptTemplate,
  AIUsageReport,
  AICreatedSegment,
} from './models/ai';

export const AI_ENDPOINTS = {
  DRAFTS: `${API_VERSION}/ai/drafts`,
  TEMPLATES: `${API_VERSION}/ai/templates`,
  USAGE: `${API_VERSION}/ai/usage`,
  /** Owned by the segments module; only the create call is needed here. */
  SEGMENTS: `${API_VERSION}/segments`,
} as const;

/**
 * Errors are rendered by the caller. Drafting is never retried: a 429 carries
 * Retry-After until the next UTC midnight, and a retry would spend quota.
 */
const QUIET = { showErrorToast: false, skipRetry: true } as const;

export function useAIService() {
  const api = useApi();

  /** 429 ai_quota_exceeded; 503 ai_not_configured / ai_unavailable / ai_bad_output; 422 ai_refused, ai_output_truncated. */
  const createDrafts = useCallback(
    (data: AIDraftRequest) => api.post<AIDraftResponse>(AI_ENDPOINTS.DRAFTS, data, QUIET),
    [api],
  );

  const listTemplates = useCallback(
    () => api.get<CursorPage<AIPromptTemplate>>(AI_ENDPOINTS.TEMPLATES),
    [api],
  );

  /** days: 1..90 (default 30). */
  const getUsage = useCallback(
    (days?: number) => api.get<AIUsageReport>(`${AI_ENDPOINTS.USAGE}${toQuery({ days })}`),
    [api],
  );

  /** POST /segments with a segment draft as the body. */
  const createSegment = useCallback(
    (draft: AIDraft) => api.post<AICreatedSegment>(AI_ENDPOINTS.SEGMENTS, draft, { showErrorToast: false }),
    [api],
  );

  return useMemo(
    () => ({ createDrafts, listTemplates, getUsage, createSegment }),
    [createDrafts, listTemplates, getUsage, createSegment],
  );
}

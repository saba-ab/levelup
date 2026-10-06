import { useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useAIService } from '@/services/api/ai';
import { useMechanicsService } from '@/services/api/mechanics';
import { fetchAllPages } from '@/services/api/pagination';
import type { ValidationErrors } from '@/hooks/useApi';
import type {
  CreateBadgeData,
  CreateLevelData,
  CreateMissionData,
  CreateRewardData,
  CreateRuleData,
} from '@/services/api/types';
import type { AIDraft, AIDraftContext, AIDraftKind, AIDraftRequest } from '@/services/api/models/ai';
import { AI_ERROR_CODES } from '@/services/api/models/ai';
import { useEventsQuery } from './events';
import { useAllBadgesQuery, useLevelsQuery, useCreateBadgeMutation, useCreateLevelMutation, useCreateMissionMutation, useCreateRewardMutation } from './mechanics';
import { useCreateRuleMutation, unwrap } from './rules';
import { queryKeys } from './keys';

export const aiKeys = {
  all: ['ai'] as const,
  templates: () => [...aiKeys.all, 'templates'] as const,
  usage: (days?: number) => [...aiKeys.all, 'usage', days] as const,
  contextMissions: () => [...aiKeys.all, 'context', 'missions'] as const,
  contextRewards: () => [...aiKeys.all, 'context', 'rewards'] as const,
};

/** The 10 built-in prompt templates. */
export function useAITemplatesQuery(enabled = true) {
  const { listTemplates } = useAIService();
  return useQuery({
    queryKey: aiKeys.templates(),
    queryFn: async () => unwrap(await listTemplates(), 'Failed to fetch AI templates').data,
    staleTime: 60 * 60 * 1000,
    enabled,
  });
}

/** Today's usage and quota plus the last `days` days (1..90). */
export function useAIUsageQuery(days?: number, enabled = true) {
  const { getUsage } = useAIService();
  return useQuery({
    queryKey: aiKeys.usage(days),
    queryFn: async () => unwrap(await getUsage(days), 'Failed to fetch AI usage'),
    enabled,
  });
}

/** POST /ai/drafts. Refreshes usage on success and on quota errors. */
export function useAIDraftsMutation() {
  const queryClient = useQueryClient();
  const { createDrafts } = useAIService();
  return useMutation({
    mutationFn: async (data: AIDraftRequest) => unwrap(await createDrafts(data), 'Failed to generate drafts'),
    onSettled: () => queryClient.invalidateQueries({ queryKey: [...aiKeys.all, 'usage'] }),
  });
}

// ---- tenant context ---------------------------------------------------------------

const MAX_CONTEXT_ITEMS = 200;
const MAX_CONTEXT_LEVELS = 100;
const clip = (s: string | null | undefined, n: number) => (s ? s.slice(0, n) : undefined);

/**
 * The tenant's event types, badges, missions, rewards and levels, in the
 * shape POST /ai/drafts takes as `context`, so drafts reference real ids.
 */
export function useAIDraftContext(enabled = true) {
  const { listMissions, listRewards } = useMechanicsService();
  const events = useEventsQuery();
  const badges = useAllBadgesQuery();
  const levels = useLevelsQuery();
  const missions = useQuery({
    queryKey: aiKeys.contextMissions(),
    queryFn: async () =>
      unwrap(await fetchAllPages((cursor) => listMissions({ limit: 100, cursor }), MAX_CONTEXT_ITEMS), 'Failed to fetch missions').data,
    enabled,
  });
  const rewards = useQuery({
    queryKey: aiKeys.contextRewards(),
    queryFn: async () =>
      unwrap(await fetchAllPages((cursor) => listRewards({ limit: 100, cursor }), MAX_CONTEXT_ITEMS), 'Failed to fetch rewards').data,
    enabled,
  });

  const context = useMemo<AIDraftContext>(
    () => ({
      event_types: (events.data ?? [])
        .filter((e) => e.is_active)
        .slice(0, MAX_CONTEXT_ITEMS)
        .map((e) => ({ slug: e.slug, name: clip(e.name, 255), description: clip(e.description, 500) })),
      badges: (badges.data ?? []).slice(0, MAX_CONTEXT_ITEMS).map((b) => ({ id: b.id, name: clip(b.name, 255) })),
      missions: (missions.data ?? []).slice(0, MAX_CONTEXT_ITEMS).map((m) => ({ id: m.id, name: clip(m.name, 255) })),
      rewards: (rewards.data ?? []).slice(0, MAX_CONTEXT_ITEMS).map((r) => ({ id: r.id, name: clip(r.name, 255) })),
      levels: (levels.data ?? []).slice(0, MAX_CONTEXT_LEVELS).map((l) => ({
        level_number: l.level_number,
        name: clip(l.name, 255),
        xp_required: l.xp_required,
      })),
    }),
    [events.data, badges.data, missions.data, rewards.data, levels.data],
  );

  const sources = [events, badges, levels, missions, rewards];
  return {
    context,
    isLoading: sources.some((q) => q.isLoading),
    /** Some lists failed: drafts can still be generated, with less context. */
    isPartial: sources.some((q) => q.isError),
  };
}

// ---- creating from drafts ------------------------------------------------------------

/** What a created entity is identified by in the UI. */
export interface CreatedFromDraft {
  id: string;
  name: string;
}

/** Creates a segment from its draft (POST /segments, owned by the segments module). */
function useCreateSegmentFromDraftMutation() {
  const queryClient = useQueryClient();
  const { createSegment } = useAIService();
  return useMutation({
    mutationFn: async (draft: AIDraft) => unwrap(await createSegment(draft), 'Failed to create segment'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.segments.all }),
  });
}

/**
 * Sends a draft to the owning module's create endpoint through the existing
 * mechanics / rules mutations (which invalidate their lists).
 */
export function useCreateFromDraft() {
  const badge = useCreateBadgeMutation();
  const level = useCreateLevelMutation();
  const mission = useCreateMissionMutation();
  const reward = useCreateRewardMutation();
  const rule = useCreateRuleMutation();
  const segment = useCreateSegmentFromDraftMutation();

  return async (kind: AIDraftKind, draft: AIDraft): Promise<CreatedFromDraft> => {
    switch (kind) {
      case 'badge': {
        const b = await badge.mutateAsync(draft as unknown as CreateBadgeData);
        return { id: b.id, name: b.name };
      }
      case 'level': {
        const l = await level.mutateAsync(draft as unknown as CreateLevelData);
        return { id: l.id, name: l.name };
      }
      case 'mission': {
        const m = await mission.mutateAsync(draft as unknown as CreateMissionData);
        return { id: m.id, name: m.name };
      }
      case 'reward': {
        const r = await reward.mutateAsync(draft as unknown as CreateRewardData);
        return { id: r.id, name: r.name };
      }
      case 'rule': {
        const r = await rule.mutateAsync(draft as unknown as CreateRuleData);
        return { id: r.id, name: r.name };
      }
      case 'segment': {
        const s = await segment.mutateAsync(draft);
        return { id: s.id, name: s.name };
      }
    }
  };
}

// ---- errors -----------------------------------------------------------------------------

export type AIErrorKind = 'not_configured' | 'quota' | 'unavailable' | 'invalid' | 'forbidden' | 'other';

export interface DescribedAIError {
  kind: AIErrorKind;
  title: string;
  message: string;
  code: string | null;
  fieldErrors: ValidationErrors | null;
}

/** Time until the quota resets (next UTC midnight), e.g. "5h 12m". */
export function timeUntilQuotaReset(now = new Date()): string {
  const reset = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + 1);
  const mins = Math.max(1, Math.ceil((reset - now.getTime()) / 60000));
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

/** Maps an AI / create error (ApiRequestError or MechanicsApiError) to what the UI shows. */
export function describeAIError(err: unknown): DescribedAIError {
  const e = err as { message?: string; code?: string | null; status?: number; validationErrors?: ValidationErrors | null };
  const code = e?.code ?? null;
  const status = e?.status ?? 0;
  const message = e?.message || 'Something went wrong';
  const fieldErrors = e?.validationErrors ?? null;
  if (code === AI_ERROR_CODES.NOT_CONFIGURED) {
    return { kind: 'not_configured', title: 'AI is not configured on this server', message: 'Set an AI provider API key on the server to enable drafting.', code, fieldErrors };
  }
  if (code === AI_ERROR_CODES.QUOTA_EXCEEDED || status === 429) {
    return { kind: 'quota', title: 'Daily AI limit reached', message: `The quota resets at midnight UTC (in ${timeUntilQuotaReset()}).`, code, fieldErrors };
  }
  if (status === 503 || code === AI_ERROR_CODES.UNAVAILABLE || code === AI_ERROR_CODES.BAD_OUTPUT) {
    return { kind: 'unavailable', title: 'AI provider unavailable', message, code, fieldErrors };
  }
  if (status === 403) {
    return { kind: 'forbidden', title: 'Not allowed', message: 'Your role does not have permission for this action.', code, fieldErrors };
  }
  if (status === 422 || status === 400) {
    return { kind: 'invalid', title: 'Request rejected', message, code, fieldErrors };
  }
  return { kind: 'other', title: 'Request failed', message, code, fieldErrors };
}

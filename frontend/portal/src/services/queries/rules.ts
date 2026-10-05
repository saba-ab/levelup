import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import type { ApiResponse, ValidationErrors } from '@/hooks/useApi';
import { useRulesService } from '@/services/api/rules';
import type {
  ID,
  CursorParams,
  RuleFilters,
  CreateRuleData,
  UpdateRuleData,
  CreateRuleVersionData,
  SimulateRulesData,
  RuleDecisionFilters,
} from '@/services/api/types';

/**
 * A failed API call, keeping what the UI branches on: the problem+json code
 * (e.g. invalid_rule_definition, no_draft_version) and per-field errors.
 */
export class ApiRequestError extends Error {
  readonly code: string | null;
  readonly validationErrors: ValidationErrors | null;
  readonly status: number;

  constructor(res: Pick<ApiResponse<unknown>, 'error' | 'code' | 'validationErrors' | 'status'>, fallback: string) {
    super(res.error || fallback);
    this.name = 'ApiRequestError';
    this.code = res.code;
    this.validationErrors = res.validationErrors;
    this.status = res.status;
  }
}

/** Returns the body of a successful response or throws ApiRequestError. */
export function unwrap<T>(res: ApiResponse<T>, fallback: string): T {
  if (!res.success) throw new ApiRequestError(res, fallback);
  return res.data as T;
}

export const ruleKeys = {
  all: ['rules'] as const,
  lists: () => [...ruleKeys.all, 'list'] as const,
  list: (filters?: RuleFilters) => [...ruleKeys.lists(), filters] as const,
  details: () => [...ruleKeys.all, 'detail'] as const,
  detail: (id: ID) => [...ruleKeys.details(), id] as const,
  versions: (id: ID, params?: CursorParams) => [...ruleKeys.detail(id), 'versions', params] as const,
  decisions: (filters?: RuleDecisionFilters) => [...ruleKeys.all, 'decisions', filters] as const,
  decision: (id: ID) => [...ruleKeys.all, 'decision', id] as const,
};

/** One cursor page of rules: { data, next_cursor }. */
export function useRulesQuery(filters?: RuleFilters) {
  const { listRules } = useRulesService();
  return useQuery({
    queryKey: ruleKeys.list(filters),
    queryFn: async () => unwrap(await listRules(filters), 'Failed to fetch rules'),
    placeholderData: keepPreviousData,
  });
}

export function useRuleQuery(ruleId: ID | undefined) {
  const { getRule } = useRulesService();
  return useQuery({
    queryKey: ruleKeys.detail(ruleId ?? ''),
    queryFn: async () => unwrap(await getRule(ruleId!), 'Failed to fetch rule'),
    enabled: !!ruleId,
  });
}

export function useRuleVersionsQuery(ruleId: ID | undefined, params?: CursorParams) {
  const { listRuleVersions } = useRulesService();
  return useQuery({
    queryKey: ruleKeys.versions(ruleId ?? '', params),
    queryFn: async () => unwrap(await listRuleVersions(ruleId!, params), 'Failed to fetch rule versions'),
    enabled: !!ruleId,
  });
}

export function useCreateRuleMutation() {
  const queryClient = useQueryClient();
  const { createRule } = useRulesService();
  return useMutation({
    mutationFn: async (data: CreateRuleData) => unwrap(await createRule(data), 'Failed to create rule'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ruleKeys.all }),
  });
}

export function useUpdateRuleMutation() {
  const queryClient = useQueryClient();
  const { updateRule } = useRulesService();
  return useMutation({
    mutationFn: async ({ ruleId, data }: { ruleId: ID; data: UpdateRuleData }) =>
      unwrap(await updateRule(ruleId, data), 'Failed to update rule'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ruleKeys.all }),
  });
}

export function useDeleteRuleMutation() {
  const queryClient = useQueryClient();
  const { deleteRule } = useRulesService();
  return useMutation({
    mutationFn: async (ruleId: ID) => {
      unwrap(await deleteRule(ruleId), 'Failed to delete rule');
      return ruleId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ruleKeys.all }),
  });
}

export function useCreateRuleVersionMutation() {
  const queryClient = useQueryClient();
  const { createRuleVersion } = useRulesService();
  return useMutation({
    mutationFn: async ({ ruleId, data }: { ruleId: ID; data: CreateRuleVersionData }) =>
      unwrap(await createRuleVersion(ruleId, data), 'Failed to create rule version'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ruleKeys.all }),
  });
}

export function usePublishRuleMutation() {
  const queryClient = useQueryClient();
  const { publishRule } = useRulesService();
  return useMutation({
    mutationFn: async ({ ruleId, version }: { ruleId: ID; version: number }) =>
      unwrap(await publishRule(ruleId, version), 'Failed to publish rule'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ruleKeys.all }),
  });
}

/** Evaluates a hypothetical activity against the live ruleset; writes nothing. */
export function useSimulateRulesMutation() {
  const { simulateRules } = useRulesService();
  return useMutation({
    mutationFn: async (data: SimulateRulesData) => unwrap(await simulateRules(data), 'Simulation failed'),
  });
}

export function useRuleDecisionsQuery(filters?: RuleDecisionFilters, enabled = true) {
  const { listDecisions } = useRulesService();
  return useQuery({
    queryKey: ruleKeys.decisions(filters),
    queryFn: async () => unwrap(await listDecisions(filters), 'Failed to fetch decisions'),
    enabled,
    placeholderData: keepPreviousData,
  });
}

export function useRuleDecisionQuery(decisionId: ID | undefined) {
  const { getDecision } = useRulesService();
  return useQuery({
    queryKey: ruleKeys.decision(decisionId ?? ''),
    queryFn: async () => unwrap(await getDecision(decisionId!), 'Failed to fetch decision'),
    enabled: !!decisionId,
  });
}

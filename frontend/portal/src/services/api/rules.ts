import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import type {
  Rule,
  RuleFilters,
  CreateRuleData,
  UpdateRuleData,
  CreateRuleVersionData,
  SimulateRulesData,
  RuleDecision,
  RuleDecisionDetail,
  RuleDecisionFilters,
  CursorPage,
  CursorParams,
  ID,
} from './types';
import type {
  CreateRuleDataV2,
  CreateRuleVersionDataV2,
  RuleStats,
  RuleStatsParams,
  RuleV2,
  RuleVersionV2,
  SimulateRulesDataV2,
  SimulationResultV2,
  UpdateRuleDataV2,
} from './models/rules';
import { API_VERSION, RULE_ENDPOINTS, toQuery } from '@/lib/api-routes';

/** Rules endpoints added after the shared RULE_ENDPOINTS. */
export const RULE_V2_ENDPOINTS = {
  /** GET ?from&to: per-rule firing counts (permission rules.view_decisions). */
  STATS: `${API_VERSION}/rules/stats`,
} as const;

/** Writes report errors to the caller (field errors render inline), not as toasts. */
const QUIET = { showErrorToast: false } as const;

/**
 * Rules engine. Definitions are compiled server-side: an invalid one is 422
 * invalid_rule_definition with field errors such as "conditions[0].operator".
 * There is no synchronous execute; use simulate (no writes) or POST /activities.
 */
export function useRulesService() {
  const api = useApi();

  const listRules = useCallback(
    (filters?: RuleFilters) => api.get<CursorPage<Rule>>(`${RULE_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );

  /** Includes current_version and latest_version with their definitions. */
  const getRule = useCallback(
    (ruleId: ID) => api.get<RuleV2>(RULE_ENDPOINTS.SHOW(ruleId)),
    [api],
  );

  /** Accepts the v2 parts (schedule, stop_processing). */
  const createRule = useCallback(
    (data: CreateRuleData | CreateRuleDataV2) => api.post<RuleV2>(RULE_ENDPOINTS.CREATE, data, QUIET),
    [api],
  );

  const updateRule = useCallback(
    (ruleId: ID, data: UpdateRuleData | UpdateRuleDataV2) =>
      api.patch<RuleV2>(RULE_ENDPOINTS.UPDATE(ruleId), data, QUIET),
    [api],
  );

  const deleteRule = useCallback((ruleId: ID) => api.delete<void>(RULE_ENDPOINTS.DELETE(ruleId), QUIET), [api]);

  const listRuleVersions = useCallback(
    (ruleId: ID, params?: CursorParams) =>
      api.get<CursorPage<RuleVersionV2>>(`${RULE_ENDPOINTS.VERSIONS(ruleId)}${toQuery(params)}`),
    [api],
  );

  const createRuleVersion = useCallback(
    (ruleId: ID, data: CreateRuleVersionData | CreateRuleVersionDataV2) =>
      api.post<RuleVersionV2>(RULE_ENDPOINTS.VERSIONS(ruleId), data, QUIET),
    [api],
  );

  /** Makes a version live and the rule active. */
  const publishRule = useCallback(
    (ruleId: ID, version: number) => api.post<Rule>(RULE_ENDPOINTS.PUBLISH(ruleId), { version }, QUIET),
    [api],
  );

  /** Live ruleset, or only data.definition (a draft) when given. */
  const simulateRules = useCallback(
    (data: SimulateRulesData | SimulateRulesDataV2) =>
      api.post<SimulationResultV2>(RULE_ENDPOINTS.SIMULATE, data, QUIET),
    [api],
  );

  const getRuleStats = useCallback(
    (params?: RuleStatsParams) =>
      api.get<RuleStats>(`${RULE_V2_ENDPOINTS.STATS}${toQuery(params)}`, { showErrorToast: false }),
    [api],
  );

  const listDecisions = useCallback(
    (filters?: RuleDecisionFilters) =>
      api.get<CursorPage<RuleDecision>>(`${RULE_ENDPOINTS.DECISIONS}${toQuery(filters)}`),
    [api],
  );

  const getDecision = useCallback(
    (decisionId: ID) => api.get<RuleDecisionDetail>(RULE_ENDPOINTS.DECISION(decisionId)),
    [api],
  );

  return useMemo(
    () => ({
      listRules,
      getRule,
      createRule,
      updateRule,
      deleteRule,
      listRuleVersions,
      createRuleVersion,
      publishRule,
      simulateRules,
      listDecisions,
      getDecision,
      getRuleStats,
    }),
    [
      listRules,
      getRule,
      createRule,
      updateRule,
      deleteRule,
      listRuleVersions,
      createRuleVersion,
      publishRule,
      simulateRules,
      listDecisions,
      getDecision,
      getRuleStats,
    ],
  );
}

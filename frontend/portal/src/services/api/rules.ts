import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import type {
  Rule,
  RuleFilters,
  CreateRuleData,
  UpdateRuleData,
  RuleVersion,
  CreateRuleVersionData,
  SimulateRulesData,
  SimulationResult,
  RuleDecision,
  RuleDecisionDetail,
  RuleDecisionFilters,
  CursorPage,
  CursorParams,
  ID,
} from './types';
import { RULE_ENDPOINTS, toQuery } from '@/lib/api-routes';

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
  const getRule = useCallback((ruleId: ID) => api.get<Rule>(RULE_ENDPOINTS.SHOW(ruleId)), [api]);

  const createRule = useCallback((data: CreateRuleData) => api.post<Rule>(RULE_ENDPOINTS.CREATE, data, QUIET), [api]);

  const updateRule = useCallback(
    (ruleId: ID, data: UpdateRuleData) => api.patch<Rule>(RULE_ENDPOINTS.UPDATE(ruleId), data, QUIET),
    [api],
  );

  const deleteRule = useCallback((ruleId: ID) => api.delete<void>(RULE_ENDPOINTS.DELETE(ruleId), QUIET), [api]);

  const listRuleVersions = useCallback(
    (ruleId: ID, params?: CursorParams) =>
      api.get<CursorPage<RuleVersion>>(`${RULE_ENDPOINTS.VERSIONS(ruleId)}${toQuery(params)}`),
    [api],
  );

  const createRuleVersion = useCallback(
    (ruleId: ID, data: CreateRuleVersionData) => api.post<RuleVersion>(RULE_ENDPOINTS.VERSIONS(ruleId), data, QUIET),
    [api],
  );

  /** Makes a version live and the rule active. */
  const publishRule = useCallback(
    (ruleId: ID, version: number) => api.post<Rule>(RULE_ENDPOINTS.PUBLISH(ruleId), { version }, QUIET),
    [api],
  );

  const simulateRules = useCallback(
    (data: SimulateRulesData) => api.post<SimulationResult>(RULE_ENDPOINTS.SIMULATE, data, QUIET),
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
    }),
    [listRules, getRule, createRule, updateRule, deleteRule, listRuleVersions, createRuleVersion, publishRule, simulateRules, listDecisions, getDecision],
  );
}

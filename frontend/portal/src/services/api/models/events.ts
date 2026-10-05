// events models (Go API contract, see docs/rewrite and backend/api/docs/swagger.json)
import type { PaginationParams, Timestamps } from './common';

// ==================== TRIGGER EVENTS ====================

export interface TriggerEventProperty {
  name: string;
  type: 'string' | 'number' | 'boolean' | 'array' | 'object';
  required?: boolean;
  description?: string;
}

export interface TriggerEvent extends Timestamps {
  id: number;
  name: string;
  slug: string;
  description?: string;
  is_predefined: boolean;
  is_active: boolean;
  metadata?: {
    icon?: string;
    category?: string;
    properties?: TriggerEventProperty[];
    [key: string]: unknown;
  };
}

export interface CreateTriggerEventData {
  name: string;
  slug?: string;
  description?: string;
  is_predefined?: boolean;
  is_active?: boolean;
  metadata?: {
    icon?: string;
    category?: string;
    properties?: TriggerEventProperty[];
    [key: string]: unknown;
  };
}

export interface UpdateTriggerEventData {
  name?: string;
  slug?: string;
  description?: string;
  is_active?: boolean;
  metadata?: {
    icon?: string;
    category?: string;
    properties?: TriggerEventProperty[];
    [key: string]: unknown;
  };
}

export interface TriggerEventFilters extends PaginationParams {
  search?: string;
  is_predefined?: boolean;
  is_active?: boolean;
}

// ==================== RULES ====================

export interface RuleConditions {
  [key: string]: { min?: number; max?: number; eq?: unknown };
}

export interface RuleActions {
  grant_points?: number;
  grant_xp?: number;
  award_badge_id?: number;
  start_mission_id?: number;
}

export interface Rule extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  trigger_event: string;
  is_active: boolean;
  priority: number;
  conditions?: RuleConditions;
  actions?: RuleActions;
}

export interface CreateRuleData {
  name: string;
  description?: string;
  trigger_event: string;
  is_active?: boolean;
  priority?: number;
  conditions?: RuleConditions;
  actions?: RuleActions;
}

export interface RuleVersion extends Timestamps {
  id: number;
  rule_id: number;
  version: number;
  name: string;
  description?: string;
  trigger_event: string;
  conditions?: RuleConditions;
  actions?: RuleActions;
}

export interface ExecuteRulesData {
  player_id: number;
  trigger_event: string;
  trigger_data?: Record<string, unknown>;
}

export interface RuleExecution extends Timestamps {
  id: number;
  rule_id: number;
  player_id: number;
  trigger_event: string;
  trigger_data?: Record<string, unknown>;
  status: 'success' | 'failed' | 'skipped';
  result?: Record<string, unknown>;
}

export interface RuleExecutionFilters extends PaginationParams {
  rule_id?: number;
  player_id?: number;
  status?: 'success' | 'failed' | 'skipped';
  date_from?: string;
  date_to?: string;
}

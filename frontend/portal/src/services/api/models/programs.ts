// programs models (Go API contract, see docs/rewrite and backend/api/docs/swagger.json)
import type { PaginationParams, Timestamps } from './common';

// ==================== PROGRAMS ====================

export type ProgramStatus = 'draft' | 'active' | 'paused' | 'ended';

export interface ProgramSettings {
  allow_public_signup: boolean;
  require_email_verification: boolean;
  default_level_id?: number;
  welcome_points?: number;
}

export interface ProgramMechanics {
  points_enabled: boolean;
  badges_enabled: boolean;
  levels_enabled: boolean;
  missions_enabled: boolean;
  streaks_enabled: boolean;
  leaderboards_enabled: boolean;
  rewards_enabled: boolean;
}

export interface Program extends Timestamps {
  id: number;
  tenant_id: number;
  name: string;
  description?: string;
  status: ProgramStatus;
  start_date?: string;
  end_date?: string;
  settings?: ProgramSettings;
  mechanics?: ProgramMechanics;
  metadata?: Record<string, unknown>;
  player_count: number;
}

export interface CreateProgramData {
  name: string;
  description?: string;
  start_date?: string;
  end_date?: string;
  settings?: Partial<ProgramSettings>;
  mechanics?: Partial<ProgramMechanics>;
  metadata?: Record<string, unknown>;
}

export interface UpdateProgramData {
  name?: string;
  description?: string;
  status?: ProgramStatus;
  start_date?: string;
  end_date?: string;
  settings?: Partial<ProgramSettings>;
  mechanics?: Partial<ProgramMechanics>;
  metadata?: Record<string, unknown>;
}

export interface ProgramFilters extends PaginationParams {
  search?: string;
  status?: ProgramStatus;
}

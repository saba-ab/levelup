// programs models (Go API contract: backend/internal/modules/program/internal/transport/dto.go)
import type { CursorParams, ID, Timestamps } from './common';

// ==================== PROGRAMS ====================

/** draft → active ⇄ paused → ended. Changed only via /activate, /pause, /end. */
export type ProgramStatus = 'draft' | 'active' | 'paused' | 'ended';

/**
 * Program settings are a free-form JSON object on the server. These are the
 * keys the portal reads and writes; unknown keys are preserved.
 */
export interface ProgramSettings {
  allow_public_signup?: boolean;
  require_email_verification?: boolean;
  welcome_points?: number;
  [key: string]: unknown;
}

/** Free-form JSON object on the server; the portal stores feature toggles here. */
export interface ProgramMechanics {
  points_enabled?: boolean;
  badges_enabled?: boolean;
  levels_enabled?: boolean;
  missions_enabled?: boolean;
  streaks_enabled?: boolean;
  leaderboards_enabled?: boolean;
  rewards_enabled?: boolean;
  [key: string]: unknown;
}

/** ProgramResp */
export interface Program extends Timestamps {
  id: ID;
  tenant_id: ID;
  name: string;
  slug: string;
  description: string | null;
  status: ProgramStatus;
  starts_at: string | null;
  ends_at: string | null;
  settings: ProgramSettings;
  mechanics: ProgramMechanics;
  version: number;
}

/** CreateReq. Programs are always created as draft; slug is derived from name when omitted. */
export interface CreateProgramData {
  name: string;
  slug?: string;
  description?: string;
  starts_at?: string;
  ends_at?: string;
  settings?: ProgramSettings;
  mechanics?: ProgramMechanics;
}

/**
 * PatchReq: partial update, send only changed fields. description, starts_at
 * and ends_at accept null to clear them. Status is not patchable.
 */
export interface UpdateProgramData {
  name?: string;
  slug?: string;
  description?: string | null;
  starts_at?: string | null;
  ends_at?: string | null;
  settings?: ProgramSettings;
  mechanics?: ProgramMechanics;
}

/** GET /programs query params. The API has no text search. */
export interface ProgramFilters extends CursorParams {
  status?: ProgramStatus;
}

// ==================== ENROLLMENTS ====================

/** EnrollmentResp (POST /programs/{id}/players: 201 new, 200 already enrolled). */
export interface ProgramEnrollment {
  program_id: ID;
  player_id: ID;
  enrolled_at: string;
}

/** PlayerResp: the player summary embedded in a member row. */
export interface ProgramMemberPlayer {
  external_id: string;
  display_name: string;
  active: boolean;
}

/** MemberResp: one row of GET /programs/{id}/players. player is null if the player is gone. */
export interface ProgramMember {
  player_id: ID;
  enrolled_at: string;
  player: ProgramMemberPlayer | null;
}

/** Result of enrolling many players one by one. */
export interface BulkEnrollResult {
  total: number;
  enrolled: number;
  alreadyEnrolled: number;
  failures: { playerId: ID; code: string | null; error: string }[];
}

// identity models: auth, users, tenant (backend module "identity").
import type { ID, CursorParams, Timestamps } from './common';

// ==================== ROLES ====================

/** Role keys as returned by the API (ids are stable: 1..6). */
export type RoleKey = 'platform_admin' | 'owner' | 'super_admin' | 'admin' | 'program_manager' | 'developer';

export interface Role {
  id: number;
  key: RoleKey;
  label: string;
}

/** Stable role ids, for PUT /users/{id}/roles and POST /users role_ids. */
export const ROLE_IDS: Record<RoleKey, number> = {
  platform_admin: 1,
  owner: 2,
  super_admin: 3,
  admin: 4,
  program_manager: 5,
  developer: 6,
};

// ==================== AUTH ====================

export interface AuthUser extends Timestamps {
  id: ID;
  tenant_id: ID | null;
  name: string;
  email: string;
  active: boolean;
  email_verified_at: string | null;
  role_ids: number[];
  roles: Role[];
}

export interface Tenant extends Timestamps {
  id: ID;
  name: string;
  slug: string;
  owner_user_id: ID | null;
  active: boolean;
  /** IANA zone; streak periods are computed in it. */
  timezone: string;
  settings: Record<string, unknown>;
}

/** Returned by register, login and refresh. */
export interface AuthResponse {
  user: AuthUser;
  tenant: Tenant | null;
  access_token: string;
  /** Opaque, single-use: every refresh returns a new one. */
  refresh_token: string;
  token_type: 'Bearer';
  /** Access-token lifetime in seconds. */
  expires_in: number;
}

export interface MeResponse {
  user: AuthUser;
  tenant: Tenant | null;
}

export interface RegisterData {
  tenant_name: string;
  name?: string;
  email: string;
  /** 8 to 72 characters. */
  password: string;
  password_confirmation?: string;
  timezone?: string;
}

export interface LoginData {
  email: string;
  password: string;
}

export interface UpdateTenantData {
  name?: string;
  timezone?: string;
  settings?: Record<string, unknown>;
}

// ==================== USERS ====================

export type User = AuthUser;

export interface CreateUserData {
  name: string;
  email: string;
  password: string;
  password_confirmation?: string;
  role_ids?: number[];
}

export interface UpdateUserData {
  name?: string;
  email?: string;
  password?: string;
  /** Required when changing your own password. */
  current_password?: string;
  active?: boolean;
}

export type UserFilters = CursorParams;

// ==================== API KEYS (ADR-0017) ====================

/** Roles an API key may hold (never owner or super admin). */
export const API_KEY_ROLE_KEYS: RoleKey[] = ['admin', 'program_manager', 'developer'];

export interface ApiKey {
  id: ID;
  name: string;
  /** Clear prefix: the key reads "lvl_live_<prefix>_…". */
  prefix: string;
  role_ids: number[];
  roles: Role[];
  created_by: ID | null;
  created_at: string;
  last_used_at: string | null;
  expires_at: string | null;
  revoked_at: string | null;
}

export interface CreateApiKeyData {
  name: string;
  role_ids?: number[];
  expires_at?: string;
}

/** The only response that contains the secret; it is never retrievable again. */
export interface CreatedApiKey {
  api_key: ApiKey;
  secret: string;
}

// ==================== ACCOUNT FLOWS ====================
// Emailed links land on the portal: /reset-password, /verify-email and
// /accept-invite, each with ?token=… (43-char base64url).

export interface ForgotPasswordData {
  email: string;
}

/** Single use; revokes every session of the user. 422 invalid_reset_token otherwise. */
export interface ResetPasswordData {
  token: string;
  /** 8 to 72 characters. */
  password: string;
  password_confirmation?: string;
}

/** 422 invalid_verification_token for unknown, expired or used tokens. */
export interface VerifyEmailData {
  token: string;
}

// ==================== INVITATIONS ====================

export type InvitationStatus = 'pending' | 'expired';

/** Never contains the token (only its hash is stored). */
export interface Invitation {
  id: ID;
  email: string;
  name: string;
  role_ids: number[];
  roles: Role[];
  /** Lists show open invitations only: pending, or expired (not accepted, not revoked). */
  status: InvitationStatus;
  invited_by: ID | null;
  expires_at: string;
  created_at: string;
}

export interface CreateInvitationData {
  email: string;
  name?: string;
  /** 1 to 10 role ids; same rules as creating a user. */
  role_ids: number[];
}

export type InvitationFilters = CursorParams;

/** GET /auth/invitations/{token}: anonymous preview for the accept page (404 invitation_not_found). */
export interface InvitationPreview {
  email: string;
  name: string;
  tenant_name: string;
  inviter_name: string;
  role_ids: number[];
  roles: Role[];
  expires_at: string;
}

/** POST /auth/accept-invite returns 201 with a session (AuthResponse). */
export interface AcceptInviteData {
  token: string;
  name?: string;
  /** 8 to 72 characters. */
  password: string;
  password_confirmation?: string;
}

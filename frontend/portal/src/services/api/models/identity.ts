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

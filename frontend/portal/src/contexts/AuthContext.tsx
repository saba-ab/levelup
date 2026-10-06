import React, { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react';
import { TeamRole, Permission, hasPermission, canAccessRoute } from '@/lib/permissions';
import { useApi, refreshSession } from '@/hooks/useApi';
import { authStorage } from '@/lib/auth-storage';
import type { AuthResponse, AuthUser, MeResponse, RegisterData, LoginData, RoleKey, Tenant } from '@/services/api/types';
import { AUTH_ENDPOINTS } from '@/lib/api-routes';

interface User {
  id: string;
  email: string;
  name: string;
  firstName: string;
  lastName: string;
  tenantId?: string;
  tenantName?: string;
  role: TeamRole;
  roleKeys: RoleKey[];
}

interface AuthContextType {
  user: User | null;
  tenant: Tenant | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  /** Stores a session obtained elsewhere (e.g. POST /auth/accept-invite) exactly like login does. */
  loginWithSession: (session: AuthResponse) => void;
  register: (data: RegisterFormData) => Promise<void>;
  logout: () => Promise<void>;
  refreshToken: () => Promise<boolean>;
  hasPermission: (permission: Permission) => boolean;
  canAccessRoute: (path: string) => boolean;
  setUserRole: (role: TeamRole) => void;
  getToken: () => string | null;
  /** Applies a tenant returned by PATCH /tenant so the header and session stay current. */
  applyTenant: (tenant: Tenant) => void;
}

interface RegisterFormData {
  email: string;
  password: string;
  firstName: string;
  lastName: string;
  tenantName: string;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

/**
 * The portal's role is the most senior API role the user holds. A platform
 * admin has no tenant and is treated like a super admin inside the portal.
 */
const ROLE_PRECEDENCE: [RoleKey, TeamRole][] = [
  ['owner', 'owner'],
  ['super_admin', 'super_admin'],
  ['platform_admin', 'super_admin'],
  ['admin', 'admin'],
  ['program_manager', 'program_manager'],
  ['developer', 'developer'],
];

function portalRole(keys: RoleKey[]): TeamRole {
  for (const [key, role] of ROLE_PRECEDENCE) {
    if (keys.includes(key)) return role;
  }
  return 'developer';
}

function toUser(apiUser: AuthUser, tenant: Tenant | null): User {
  const nameParts = (apiUser.name || '').split(' ');
  const roleKeys = (apiUser.roles || []).map(r => r.key);
  return {
    id: apiUser.id,
    email: apiUser.email,
    name: apiUser.name || '',
    firstName: nameParts[0] || '',
    lastName: nameParts.slice(1).join(' '),
    tenantId: apiUser.tenant_id ?? undefined,
    tenantName: tenant?.name,
    role: portalRole(roleKeys),
    roleKeys,
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [tenant, setTenant] = useState<Tenant | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const api = useApi();

  const applySession = useCallback((session: AuthResponse) => {
    authStorage.setTokens(session.access_token, session.refresh_token);
    const next = toUser(session.user, session.tenant);
    authStorage.setUser(next);
    setUser(next);
    setTenant(session.tenant);
  }, []);

  // Restore the session on load. GET /auth/me refreshes transparently when
  // the access token expired (useApi), so a 401 here means it is over.
  useEffect(() => {
    const init = async () => {
      if (authStorage.getAccessToken() || authStorage.getRefreshToken()) {
        const res = await api.get<MeResponse>(AUTH_ENDPOINTS.ME, { showErrorToast: false });
        if (res.success && res.data) {
          const next = toUser(res.data.user, res.data.tenant);
          authStorage.setUser(next);
          setUser(next);
          setTenant(res.data.tenant);
        } else if (res.status === 401) {
          authStorage.clear();
        } else {
          // Server unreachable: keep the cached user so the UI can show its
          // offline state instead of logging the user out.
          setUser(authStorage.getUser<User>());
        }
      }
      setIsLoading(false);
    };
    init();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // useApi rotates tokens in the background; keep the profile in sync.
  useEffect(() => {
    const onRefreshed = (e: Event) => {
      const session = (e as CustomEvent<AuthResponse>).detail;
      if (session?.user) {
        const next = toUser(session.user, session.tenant);
        authStorage.setUser(next);
        setUser(next);
        setTenant(session.tenant);
      }
    };
    window.addEventListener('levelupos:session-refreshed', onRefreshed);
    return () => window.removeEventListener('levelupos:session-refreshed', onRefreshed);
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const body: LoginData = { email, password };
    const res = await api.post<AuthResponse>(AUTH_ENDPOINTS.LOGIN, body, { skipAuth: true, showErrorToast: false });
    if (!res.success || !res.data) {
      throw new Error(res.error || 'Login failed');
    }
    applySession(res.data);
  }, [api, applySession]);

  const register = useCallback(async (data: RegisterFormData) => {
    const body: RegisterData = {
      tenant_name: data.tenantName,
      name: [data.firstName, data.lastName].filter(Boolean).join(' ') || undefined,
      email: data.email,
      password: data.password,
      password_confirmation: data.password,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    };
    const res = await api.post<AuthResponse>(AUTH_ENDPOINTS.REGISTER, body, { skipAuth: true, showErrorToast: false });
    if (!res.success || !res.data) {
      const fieldErrors = res.validationErrors
        ? Object.entries(res.validationErrors).map(([f, m]) => `${f}: ${m.join(', ')}`).join('\n')
        : null;
      throw new Error(fieldErrors || res.error || 'Registration failed');
    }
    applySession(res.data);
  }, [api, applySession]);

  const logout = useCallback(async () => {
    try {
      await api.post<void>(AUTH_ENDPOINTS.LOGOUT, undefined, { showErrorToast: false, skipRetry: true });
    } catch {
      // Local logout proceeds even if the server is unreachable.
    }
    authStorage.clear();
    setUser(null);
    setTenant(null);
  }, [api]);

  const refreshToken = useCallback(() => refreshSession(api.baseUrl), [api.baseUrl]);

  const getToken = useCallback(() => authStorage.getAccessToken(), []);

  const applyTenant = useCallback((next: Tenant) => {
    setTenant(next);
    setUser(prev => {
      if (!prev) return prev;
      const updated = { ...prev, tenantName: next.name };
      authStorage.setUser(updated);
      return updated;
    });
  }, []);

  const checkPermission = useCallback(
    (permission: Permission) => hasPermission(user?.role, permission),
    [user?.role],
  );

  const checkRouteAccess = useCallback(
    (path: string) => canAccessRoute(user?.role, path),
    [user?.role],
  );

  /** Previews the UI as another role (client-side only; the API still enforces real roles). */
  const setUserRole = useCallback((role: TeamRole) => {
    if (user) {
      const next = { ...user, role };
      setUser(next);
      authStorage.setUser(next);
    }
  }, [user]);

  return (
    <AuthContext.Provider value={{
      user,
      tenant,
      isAuthenticated: !!user,
      isLoading,
      login,
      loginWithSession: applySession,
      register,
      logout,
      refreshToken,
      hasPermission: checkPermission,
      canAccessRoute: checkRouteAccess,
      setUserRole,
      getToken,
      applyTenant,
    }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}

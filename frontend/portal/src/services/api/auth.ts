import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import { AuthResponse, RegisterData, LoginData, MeResponse, Tenant, UpdateTenantData } from './types';
import { AUTH_ENDPOINTS, TENANT_ENDPOINTS } from '@/lib/api-routes';
import { authStorage } from '@/lib/auth-storage';

export function useAuthService() {
  const api = useApi();

  /** Creates a tenant and its owner; returns a session. */
  const register = useCallback(
    (data: RegisterData) => api.post<AuthResponse>(AUTH_ENDPOINTS.REGISTER, data, { skipAuth: true }),
    [api],
  );

  const login = useCallback(
    (data: LoginData) => api.post<AuthResponse>(AUTH_ENDPOINTS.LOGIN, data, { skipAuth: true }),
    [api],
  );

  const me = useCallback(() => api.get<MeResponse>(AUTH_ENDPOINTS.ME), [api]);

  /** Rotates the stored refresh token (single-use) for a new session. */
  const refresh = useCallback(
    () => api.post<AuthResponse>(AUTH_ENDPOINTS.REFRESH, { refresh_token: authStorage.getRefreshToken() }, { skipAuth: true }),
    [api],
  );

  /** Denies the current access token and revokes every refresh token. */
  const logout = useCallback(() => api.post<void>(AUTH_ENDPOINTS.LOGOUT), [api]);

  const getTenant = useCallback(() => api.get<Tenant>(TENANT_ENDPOINTS.CURRENT), [api]);
  const updateTenant = useCallback((data: UpdateTenantData) => api.patch<Tenant>(TENANT_ENDPOINTS.CURRENT, data), [api]);

  return useMemo(
    () => ({ register, login, me, refresh, logout, getTenant, updateTenant }),
    [register, login, me, refresh, logout, getTenant, updateTenant],
  );
}

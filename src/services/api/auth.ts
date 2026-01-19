import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import { AuthResponse, RegisterData, LoginData, AuthUser } from './types';
import { AUTH_ENDPOINTS } from '@/lib/api-routes';

export function useAuthService() {
  const api = useApi();

  // Register new organization and user
  const register = useCallback(async (data: RegisterData) => {
    return api.post<AuthResponse>(AUTH_ENDPOINTS.REGISTER, data, { skipAuth: true });
  }, [api]);

  // Login
  const login = useCallback(async (data: LoginData) => {
    return api.post<AuthResponse>(AUTH_ENDPOINTS.LOGIN, data, { skipAuth: true });
  }, [api]);

  // Get current user
  const me = useCallback(async () => {
    return api.get<AuthUser>(AUTH_ENDPOINTS.ME);
  }, [api]);

  // Refresh token
  const refresh = useCallback(async () => {
    return api.post<AuthResponse>(AUTH_ENDPOINTS.REFRESH);
  }, [api]);

  // Logout
  const logout = useCallback(async () => {
    return api.post<{ message: string }>(AUTH_ENDPOINTS.LOGOUT);
  }, [api]);

  return {
    register,
    login,
    me,
    refresh,
    logout,
  };
}

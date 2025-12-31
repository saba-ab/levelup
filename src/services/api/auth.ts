import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import { AuthResponse, RegisterData, LoginData, AuthUser } from './types';

export function useAuthService() {
  const api = useApi();

  // Register new organization and user
  const register = useCallback(async (data: RegisterData) => {
    return api.post<AuthResponse>('/auth/register', data, { skipAuth: true });
  }, [api]);

  // Login
  const login = useCallback(async (data: LoginData) => {
    return api.post<AuthResponse>('/auth/login', data, { skipAuth: true });
  }, [api]);

  // Get current user
  const me = useCallback(async () => {
    return api.get<AuthUser>('/auth/me');
  }, [api]);

  // Refresh token
  const refresh = useCallback(async () => {
    return api.post<AuthResponse>('/auth/refresh');
  }, [api]);

  // Logout
  const logout = useCallback(async () => {
    return api.post<{ message: string }>('/auth/logout');
  }, [api]);

  return {
    register,
    login,
    me,
    refresh,
    logout,
  };
}

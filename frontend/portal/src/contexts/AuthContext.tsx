import React, { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react';
import { TeamRole, Permission, hasPermission, canAccessRoute } from '@/lib/permissions';
import { useApi } from '@/hooks/useApi';
import { AuthResponse, AuthUser, RegisterData as ApiRegisterData, LoginData } from '@/services/api/types';
import { AUTH_ENDPOINTS } from '@/lib/api-routes';

const TOKEN_KEY = 'levelupos_token';
const USER_KEY = 'levelupos_user';

interface User {
  id: number;
  email: string;
  name: string;
  firstName: string;
  lastName: string;
  tenantId?: number;
  tenantName?: string;
  role: TeamRole;
}

interface AuthContextType {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (data: RegisterFormData) => Promise<void>;
  logout: () => Promise<void>;
  refreshToken: () => Promise<boolean>;
  hasPermission: (permission: Permission) => boolean;
  canAccessRoute: (path: string) => boolean;
  setUserRole: (role: TeamRole) => void;
  getToken: () => string | null;
}

interface RegisterFormData {
  email: string;
  password: string;
  firstName: string;
  lastName: string;
  tenantName: string;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

// Helper to transform API user to local user format
function transformAuthUser(apiUser: AuthUser, role: TeamRole = 'owner'): User {
  const nameParts = apiUser.name?.split(' ') || ['', ''];
  return {
    id: apiUser.id,
    email: apiUser.email,
    name: apiUser.name || '',
    firstName: nameParts[0] || '',
    lastName: nameParts.slice(1).join(' ') || '',
    tenantId: apiUser.tenant_id,
    tenantName: undefined, // API doesn't return tenant name directly
    role,
  };
}

// Helper to get stored token
function getStoredToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

// Helper to get stored user
function getStoredUser(): User | null {
  const stored = localStorage.getItem(USER_KEY);
  if (stored) {
    try {
      return JSON.parse(stored);
    } catch {
      return null;
    }
  }
  return null;
}

// Helper to store auth data
function storeAuthData(token: string, user: User): void {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

// Helper to clear auth data
function clearAuthData(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const api = useApi();

  // Verify token and fetch current user on mount
  useEffect(() => {
    const initAuth = async () => {
      const storedToken = getStoredToken();
      const storedUser = getStoredUser();

      if (storedToken && storedUser) {
        // Try to verify the token by fetching current user
        try {
          const response = await api.get<AuthUser>(AUTH_ENDPOINTS.ME, { showErrorToast: false });

          if (response.success && response.data) {
            const verifiedUser = transformAuthUser(response.data, storedUser.role);
            setUser(verifiedUser);
            storeAuthData(storedToken, verifiedUser);
          } else if (response.status === 401) {
            // Token expired, try to refresh
            const refreshed = await refreshTokenInternal();
            if (!refreshed) {
              clearAuthData();
            }
          } else {
            // Other error, clear auth data
            clearAuthData();
          }
        } catch {
          clearAuthData();
        }
      }

      setIsLoading(false);
    };

    initAuth();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Internal refresh token function (doesn't depend on state)
  const refreshTokenInternal = async (): Promise<boolean> => {
    try {
      const response = await api.post<AuthResponse>(AUTH_ENDPOINTS.REFRESH, undefined, {
        showErrorToast: false
      });

      if (response.success && response.data) {
        const newUser = transformAuthUser(response.data.user, getStoredUser()?.role || 'owner');
        storeAuthData(response.data.access_token, newUser);
        setUser(newUser);
        return true;
      }
      return false;
    } catch {
      return false;
    }
  };

  const login = useCallback(async (email: string, password: string) => {
    const loginData: LoginData = { email, password };

    const response = await api.post<AuthResponse>(AUTH_ENDPOINTS.LOGIN, loginData, {
      skipAuth: true,
      showErrorToast: false
    });

    if (!response.success || !response.data) {
      throw new Error(response.error || 'Login failed');
    }

    const authUser = transformAuthUser(response.data.user, 'owner');
    storeAuthData(response.data.access_token, authUser);
    setUser(authUser);
  }, [api]);

  const register = useCallback(async (data: RegisterFormData) => {
    const registerData: ApiRegisterData = {
      tenant_name: data.tenantName,
      email: data.email,
      password: data.password,
      password_confirmation: data.password,
    };

    const response = await api.post<AuthResponse>(AUTH_ENDPOINTS.REGISTER, registerData, {
      skipAuth: true,
      showErrorToast: false
    });

    if (!response.success || !response.data) {
      throw new Error(response.error || 'Registration failed');
    }

    const authUser = transformAuthUser(response.data.user, 'owner');
    storeAuthData(response.data.access_token, authUser);
    setUser(authUser);
  }, [api]);

  const logout = useCallback(async () => {
    try {
      await api.post<{ message: string }>(AUTH_ENDPOINTS.LOGOUT, undefined, {
        showErrorToast: false
      });
    } catch {
      // Continue with local logout even if API call fails
    }

    clearAuthData();
    setUser(null);
  }, [api]);

  const refreshToken = useCallback(async (): Promise<boolean> => {
    return refreshTokenInternal();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const getToken = useCallback((): string | null => {
    return getStoredToken();
  }, []);

  const checkPermission = useCallback((permission: Permission): boolean => {
    return hasPermission(user?.role, permission);
  }, [user?.role]);

  const checkRouteAccess = useCallback((path: string): boolean => {
    return canAccessRoute(user?.role, path);
  }, [user?.role]);

  const setUserRole = useCallback((role: TeamRole) => {
    if (user) {
      const updatedUser = { ...user, role };
      setUser(updatedUser);
      const token = getStoredToken();
      if (token) {
        storeAuthData(token, updatedUser);
      }
    }
  }, [user]);

  return (
    <AuthContext.Provider value={{
      user,
      isAuthenticated: !!user,
      isLoading,
      login,
      register,
      logout,
      refreshToken,
      hasPermission: checkPermission,
      canAccessRoute: checkRouteAccess,
      setUserRole,
      getToken,
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

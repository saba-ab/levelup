import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { TeamRole, Permission, hasPermission, canAccessRoute } from '@/lib/permissions';

interface User {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  tenantName: string;
  role: TeamRole;
}

interface AuthContextType {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (data: RegisterData) => Promise<void>;
  logout: () => void;
  hasPermission: (permission: Permission) => boolean;
  canAccessRoute: (path: string) => boolean;
  setUserRole: (role: TeamRole) => void;
}

interface RegisterData {
  email: string;
  password: string;
  firstName: string;
  lastName: string;
  tenantName: string;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    // Check for stored user on mount
    const storedUser = localStorage.getItem('levelupos_user');
    if (storedUser) {
      setUser(JSON.parse(storedUser));
    }
    setIsLoading(false);
  }, []);

  const login = async (email: string, password: string) => {
    // Simulate API call
    await new Promise(resolve => setTimeout(resolve, 1000));
    
    // Mock successful login - default to owner role for demo
    const mockUser: User = {
      id: 'usr_001',
      email,
      firstName: 'Demo',
      lastName: 'User',
      tenantName: 'Demo Company',
      role: 'owner',
    };
    
    setUser(mockUser);
    localStorage.setItem('levelupos_user', JSON.stringify(mockUser));
  };

  const register = async (data: RegisterData) => {
    // Simulate API call
    await new Promise(resolve => setTimeout(resolve, 1500));
    
    const newUser: User = {
      id: 'usr_' + Math.random().toString(36).substr(2, 9),
      email: data.email,
      firstName: data.firstName,
      lastName: data.lastName,
      tenantName: data.tenantName,
      role: 'owner', // New registrations are owners
    };
    
    setUser(newUser);
    localStorage.setItem('levelupos_user', JSON.stringify(newUser));
  };

  const logout = () => {
    setUser(null);
    localStorage.removeItem('levelupos_user');
  };

  const checkPermission = (permission: Permission): boolean => {
    return hasPermission(user?.role, permission);
  };

  const checkRouteAccess = (path: string): boolean => {
    return canAccessRoute(user?.role, path);
  };

  const setUserRole = (role: TeamRole) => {
    if (user) {
      const updatedUser = { ...user, role };
      setUser(updatedUser);
      localStorage.setItem('levelupos_user', JSON.stringify(updatedUser));
    }
  };

  return (
    <AuthContext.Provider value={{
      user,
      isAuthenticated: !!user,
      isLoading,
      login,
      register,
      logout,
      hasPermission: checkPermission,
      canAccessRoute: checkRouteAccess,
      setUserRole,
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

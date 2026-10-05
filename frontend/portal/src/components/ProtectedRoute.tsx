import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from '@/contexts/AuthContext';
import { Permission } from '@/lib/permissions';

interface ProtectedRouteProps {
  children: React.ReactNode;
  permission?: Permission;
}

export default function ProtectedRoute({ children, permission }: ProtectedRouteProps) {
  const { isAuthenticated, hasPermission, canAccessRoute } = useAuth();
  const location = useLocation();

  // Check authentication first
  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  // Check specific permission if provided
  if (permission && !hasPermission(permission)) {
    return <Navigate to="/" replace />;
  }

  // Check route-based permission
  if (!canAccessRoute(location.pathname)) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}
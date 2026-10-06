import { useAuth } from '@/contexts/AuthContext';
import type { TeamRole } from '@/lib/permissions';

/**
 * The backend's "admin roles" (identity contracts AdminRoles). They hold
 * webhooks:manage and identity:api_keys_manage; every tenant role holds
 * webhooks:view, activity:view_any and rules:view_decisions.
 */
const ADMIN_ROLES: TeamRole[] = ['owner', 'super_admin', 'admin'];

/** What the signed-in user may do on the Integrations and Audit Logs pages. */
export function useIntegrationsAccess() {
  const { user } = useAuth();
  const isAdmin = !!user && ADMIN_ROLES.includes(user.role);
  return {
    /** webhooks:view (any tenant role). */
    canViewWebhooks: !!user,
    /** webhooks:manage: create, edit, toggle, test, rotate, delete, redeliver. */
    canManageWebhooks: isAdmin,
    /** identity:api_keys_manage (humans with an admin role). */
    canManageApiKeys: isAdmin,
  };
}

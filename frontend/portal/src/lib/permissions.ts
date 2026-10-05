/** Portal roles: the tenant API roles (platform_admin maps to super_admin in AuthContext). */
export type TeamRole = 'owner' | 'super_admin' | 'admin' | 'program_manager' | 'developer';

export type Permission = 
  | 'view:overview'
  | 'view:ai-hub'
  | 'view:programs'
  | 'manage:programs'
  | 'view:rules'
  | 'manage:rules'
  | 'view:mechanics'
  | 'manage:mechanics'
  | 'view:players'
  | 'manage:players'
  | 'view:segments'
  | 'manage:segments'
  | 'view:analytics'
  | 'view:notifications'
  | 'manage:notifications'
  | 'view:integrations'
  | 'manage:integrations'
  | 'view:docs'
  | 'view:audit-logs'
  | 'view:settings'
  | 'manage:settings'
  | 'manage:team'
  | 'manage:billing';

// Role permission mappings
const rolePermissions: Record<TeamRole, Permission[]> = {
  owner: [
    'view:overview', 'view:ai-hub', 'view:programs', 'manage:programs',
    'view:rules', 'manage:rules', 'view:mechanics', 'manage:mechanics',
    'view:players', 'manage:players', 'view:segments', 'manage:segments',
    'view:analytics', 'view:notifications', 'manage:notifications',
    'view:integrations', 'manage:integrations', 'view:docs', 'view:audit-logs',
    'view:settings', 'manage:settings', 'manage:team', 'manage:billing',
  ],
  super_admin: [
    'view:overview', 'view:ai-hub', 'view:programs', 'manage:programs',
    'view:rules', 'manage:rules', 'view:mechanics', 'manage:mechanics',
    'view:players', 'manage:players', 'view:segments', 'manage:segments',
    'view:analytics', 'view:notifications', 'manage:notifications',
    'view:integrations', 'manage:integrations', 'view:docs', 'view:audit-logs',
    'view:settings', 'manage:settings', 'manage:team',
  ],
  admin: [
    'view:overview', 'view:ai-hub', 'view:programs', 'manage:programs',
    'view:rules', 'manage:rules', 'view:mechanics', 'manage:mechanics',
    'view:players', 'manage:players', 'view:segments', 'manage:segments',
    'view:analytics', 'view:notifications', 'manage:notifications',
    'view:docs', 'view:settings',
  ],
  program_manager: [
    'view:overview', 'view:ai-hub', 'view:programs', 'manage:programs',
    'view:rules', 'manage:rules', 'view:mechanics', 'manage:mechanics',
    'view:players', 'view:segments', 'manage:segments', 'view:analytics',
    'view:notifications', 'manage:notifications', 'view:docs',
  ],
  developer: [
    'view:overview', 'view:rules', 'view:mechanics', 'view:players',
    'view:integrations', 'manage:integrations', 'view:docs', 'view:audit-logs',
  ],
};

// Route to permission mapping
export const routePermissions: Record<string, Permission> = {
  '/': 'view:overview',
  '/ai-hub': 'view:ai-hub',
  '/programs': 'view:programs',
  '/rules': 'view:rules',
  '/rules/new': 'manage:rules',
  '/events': 'view:rules',
  '/mechanics': 'view:mechanics',
  '/mechanics/points': 'view:mechanics',
  '/mechanics/badges': 'view:mechanics',
  '/mechanics/levels': 'view:mechanics',
  '/mechanics/missions': 'view:mechanics',
  '/mechanics/streaks': 'view:mechanics',
  '/mechanics/leaderboards': 'view:mechanics',
  '/mechanics/rewards': 'view:mechanics',
  '/players': 'view:players',
  '/players/compare': 'view:players',
  '/segments': 'view:segments',
  '/analytics': 'view:analytics',
  '/notifications': 'view:notifications',
  '/integrations': 'view:integrations',
  '/docs': 'view:docs',
  '/docs/api': 'view:docs',
  '/docs/guides': 'view:docs',
  '/docs/developer': 'view:docs',
  '/audit-logs': 'view:audit-logs',
  '/settings': 'view:settings',
};

// Check if a role has a specific permission
export function hasPermission(role: TeamRole | undefined, permission: Permission): boolean {
  if (!role) return false;
  return rolePermissions[role]?.includes(permission) ?? false;
}

// Check if a role can access a specific route
export function canAccessRoute(role: TeamRole | undefined, path: string): boolean {
  if (!role) return false;
  
  // Check exact match first
  if (routePermissions[path]) {
    return hasPermission(role, routePermissions[path]);
  }
  
  // Check for dynamic routes (e.g., /players/:id)
  if (path.startsWith('/players/')) {
    return hasPermission(role, 'view:players');
  }
  if (path.startsWith('/programs/')) {
    return hasPermission(role, 'view:programs');
  }
  if (path.startsWith('/rules/')) {
    return hasPermission(role, 'manage:rules');
  }
  
  // Default allow for unknown routes
  return true;
}

// Get all permissions for a role
export function getRolePermissions(role: TeamRole): Permission[] {
  return rolePermissions[role] || [];
}

// Navigation items with their required permissions
export interface NavPermission {
  path: string;
  permission: Permission;
  children?: NavPermission[];
}

export const navPermissions: NavPermission[] = [
  { path: '/', permission: 'view:overview' },
  { path: '/ai-hub', permission: 'view:ai-hub' },
  { path: '/programs', permission: 'view:programs' },
  { path: '/rules', permission: 'view:rules' },
  { path: '/events', permission: 'view:rules' },
  { 
    path: '/mechanics', 
    permission: 'view:mechanics',
    children: [
      { path: '/mechanics/points', permission: 'view:mechanics' },
      { path: '/mechanics/badges', permission: 'view:mechanics' },
      { path: '/mechanics/levels', permission: 'view:mechanics' },
      { path: '/mechanics/missions', permission: 'view:mechanics' },
      { path: '/mechanics/streaks', permission: 'view:mechanics' },
      { path: '/mechanics/leaderboards', permission: 'view:mechanics' },
      { path: '/mechanics/rewards', permission: 'view:mechanics' },
    ],
  },
  { path: '/players', permission: 'view:players' },
  { path: '/segments', permission: 'view:segments' },
  { path: '/analytics', permission: 'view:analytics' },
  { path: '/notifications', permission: 'view:notifications' },
  { path: '/integrations', permission: 'view:integrations' },
  { 
    path: '/docs', 
    permission: 'view:docs',
    children: [
      { path: '/docs', permission: 'view:docs' },
      { path: '/docs/api', permission: 'view:docs' },
      { path: '/docs/guides', permission: 'view:docs' },
      { path: '/docs/developer', permission: 'view:docs' },
    ],
  },
  { path: '/audit-logs', permission: 'view:audit-logs' },
  { path: '/settings', permission: 'view:settings' },
];

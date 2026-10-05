import { useNavigate, useLocation } from 'react-router-dom';
import { Bell, Search, Sun, Moon, ChevronDown, LogOut, User, Settings, Shield } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useAuth } from '@/contexts/AuthContext';
import { useTheme } from '@/contexts/ThemeContext';
import { TeamRole } from '@/lib/permissions';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';

const breadcrumbMap: Record<string, string> = {
  '/': 'Overview',
  '/programs': 'Programs',
  '/ai-hub': 'AI Hub',
  '/rules': 'Rules',
  '/rules/new': 'Rule Builder',
  '/events': 'Events',
  '/mechanics/points': 'Points & Wallets',
  '/mechanics/badges': 'Badges',
  '/mechanics/levels': 'Levels',
  '/mechanics/missions': 'Missions',
  '/mechanics/streaks': 'Streaks',
  '/mechanics/leaderboards': 'Leaderboards',
  '/mechanics/rewards': 'Rewards',
  '/players': 'Players',
  '/players/compare': 'Compare Players',
  '/segments': 'Segments',
  '/analytics': 'Analytics',
  '/notifications': 'Notifications',
  '/integrations': 'Integrations',
  '/docs': 'Documentation',
  '/docs/api': 'API Reference',
  '/docs/guides': 'User Guides',
  '/docs/developer': 'Developer Docs',
  '/audit-logs': 'Audit & Logs',
  '/settings': 'Settings',
};

const roleLabels: Record<TeamRole, string> = {
  owner: 'Owner',
  super_admin: 'Super Admin',
  admin: 'Admin',
  program_manager: 'Program Manager',
  developer: 'Developer',
};

interface HeaderProps {
  sidebarCollapsed: boolean;
  onOpenSearch?: () => void;
}

export default function Header({ sidebarCollapsed, onOpenSearch }: HeaderProps) {
  const { user, logout, setUserRole } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const navigate = useNavigate();
  const location = useLocation();

  const currentPage =
    breadcrumbMap[location.pathname] ||
    (location.pathname.startsWith('/players/') ? 'Player' : location.pathname.startsWith('/programs/') ? 'Program' : 'Dashboard');

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  return (
    <header
      className={cn(
        "fixed top-0 right-0 h-16 bg-background/80 backdrop-blur-xl border-b border-border/50 z-30 flex items-center justify-between px-6 transition-all duration-300",
        sidebarCollapsed ? "left-16" : "left-64"
      )}
    >
      {/* Left Section - Breadcrumb */}
      <div className="flex items-center gap-4">
        <nav className="flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Dashboard</span>
          <span className="text-muted-foreground">/</span>
          <span className="font-medium">{currentPage}</span>
        </nav>
      </div>

      {/* Right Section */}
      <div className="flex items-center gap-4">
        {/* Search */}
        <button
          onClick={onOpenSearch}
          className="relative hidden md:flex items-center gap-2 w-64 h-9 px-3 rounded-md bg-secondary/50 text-muted-foreground text-sm hover:bg-secondary transition-colors"
        >
          <Search className="w-4 h-4" />
          <span>Search...</span>
          <kbd className="ml-auto pointer-events-none inline-flex h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-mono text-[10px] font-medium text-muted-foreground">
            <span className="text-xs">⌘</span>K
          </kbd>
        </button>

        {/* Role Switcher (Demo) */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="gap-2 h-8">
              <Shield className="w-3.5 h-3.5" />
              <span className="hidden sm:inline text-xs">{user?.role ? roleLabels[user.role] : 'Role'}</span>
              <ChevronDown className="w-3 h-3" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuLabel className="text-xs text-muted-foreground">Preview as role (UI only)</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {(Object.keys(roleLabels) as TeamRole[]).map((role) => (
              <DropdownMenuItem 
                key={role} 
                onClick={() => setUserRole(role)}
                className={cn(user?.role === role && "bg-primary/10")}
              >
                <span className="flex-1">{roleLabels[role]}</span>
                {user?.role === role && <Badge variant="secondary" className="text-[10px] px-1.5">Active</Badge>}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        {/* Theme Toggle */}
        <Button variant="ghost" size="icon" onClick={toggleTheme}>
          {theme === 'dark' ? (
            <Sun className="w-5 h-5" />
          ) : (
            <Moon className="w-5 h-5" />
          )}
        </Button>

        {/* Notifications: no notifications API yet */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Notifications">
              <Bell className="w-5 h-5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-72">
            <DropdownMenuLabel>Notifications</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <p className="px-3 py-4 text-sm text-muted-foreground">
              Notifications are coming soon: they are not available in this API version.
            </p>
          </DropdownMenuContent>
        </DropdownMenu>

        {/* User Menu */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button className="flex items-center gap-2 rounded-full bg-secondary/50 px-2 py-1.5 hover:bg-secondary transition-colors">
              <div className="w-7 h-7 rounded-full bg-primary/20 flex items-center justify-center">
                <span className="text-sm font-medium text-primary">
                  {user?.firstName?.[0]}{user?.lastName?.[0]}
                </span>
              </div>
              <span className="text-sm font-medium hidden sm:block">{user?.firstName}</span>
              <ChevronDown className="w-4 h-4 text-muted-foreground" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>
              <div>
                <p className="font-medium">{user?.firstName} {user?.lastName}</p>
                <p className="text-xs text-muted-foreground">{user?.email}</p>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => navigate('/settings')}>
              <User className="w-4 h-4 mr-2" />
              Profile
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => navigate('/settings')}>
              <Settings className="w-4 h-4 mr-2" />
              Settings
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={handleLogout} className="text-destructive">
              <LogOut className="w-4 h-4 mr-2" />
              Logout
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}

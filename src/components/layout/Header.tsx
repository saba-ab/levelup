import React, { useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { Bell, Search, Sun, Moon, ChevronDown, LogOut, User, Settings, CheckCircle, AlertCircle, AlertTriangle, Info } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useAuth } from '@/contexts/AuthContext';
import { useTheme } from '@/contexts/ThemeContext';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { notifications } from '@/lib/mockData';

const breadcrumbMap: Record<string, string> = {
  '/': 'Overview',
  '/programs': 'Programs',
  '/rules': 'Rules',
  '/mechanics/points': 'Points & Wallets',
  '/mechanics/badges': 'Badges',
  '/mechanics/levels': 'Levels',
  '/mechanics/missions': 'Missions',
  '/mechanics/streaks': 'Streaks',
  '/mechanics/leaderboards': 'Leaderboards',
  '/mechanics/rewards': 'Rewards',
  '/users': 'Users',
  '/segments': 'Segments',
  '/analytics': 'Analytics',
  '/notifications': 'Notifications',
  '/integrations': 'Integrations',
  '/audit-logs': 'Audit & Logs',
  '/settings': 'Settings',
};

interface HeaderProps {
  sidebarCollapsed: boolean;
  onOpenSearch?: () => void;
}

export default function Header({ sidebarCollapsed, onOpenSearch }: HeaderProps) {
  const [environment, setEnvironment] = useState<'sandbox' | 'production'>('sandbox');
  const { user, logout } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const navigate = useNavigate();
  const location = useLocation();

  const currentPage = breadcrumbMap[location.pathname] || 'Dashboard';

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

        {/* Environment Switcher */}
        <div className="flex items-center rounded-full bg-secondary/50 p-1">
          <button
            onClick={() => setEnvironment('sandbox')}
            className={cn(
              "px-3 py-1 rounded-full text-xs font-medium transition-all",
              environment === 'sandbox'
                ? "bg-amber-500/20 text-amber-500"
                : "text-muted-foreground hover:text-foreground"
            )}
          >
            Sandbox
          </button>
          <button
            onClick={() => setEnvironment('production')}
            className={cn(
              "px-3 py-1 rounded-full text-xs font-medium transition-all",
              environment === 'production'
                ? "bg-green-500/20 text-green-500"
                : "text-muted-foreground hover:text-foreground"
            )}
          >
            Production
          </button>
        </div>

        {/* Theme Toggle */}
        <Button variant="ghost" size="icon" onClick={toggleTheme}>
          {theme === 'dark' ? (
            <Sun className="w-5 h-5" />
          ) : (
            <Moon className="w-5 h-5" />
          )}
        </Button>

        {/* Notifications */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="relative">
              <Bell className="w-5 h-5" />
              {notifications.filter(n => !n.read).length > 0 && (
                <span className="absolute -top-0.5 -right-0.5 min-w-[18px] h-[18px] flex items-center justify-center bg-destructive text-destructive-foreground text-[10px] font-bold rounded-full px-1">
                  {notifications.filter(n => !n.read).length}
                </span>
              )}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-80">
            <DropdownMenuLabel className="flex items-center justify-between">
              <span>Notifications</span>
              <Button variant="ghost" size="sm" className="h-auto p-0 text-xs text-muted-foreground hover:text-foreground">
                Mark all as read
              </Button>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <div className="max-h-[300px] overflow-y-auto">
              {notifications.slice(0, 5).map((notification) => (
                <DropdownMenuItem key={notification.id} className="flex items-start gap-3 p-3 cursor-pointer">
                  <div className={cn(
                    "mt-0.5 p-1 rounded-full shrink-0",
                    notification.type === 'success' && "bg-green-500/20 text-green-500",
                    notification.type === 'error' && "bg-destructive/20 text-destructive",
                    notification.type === 'warning' && "bg-amber-500/20 text-amber-500",
                    notification.type === 'info' && "bg-blue-500/20 text-blue-500"
                  )}>
                    {notification.type === 'success' && <CheckCircle className="w-3.5 h-3.5" />}
                    {notification.type === 'error' && <AlertCircle className="w-3.5 h-3.5" />}
                    {notification.type === 'warning' && <AlertTriangle className="w-3.5 h-3.5" />}
                    {notification.type === 'info' && <Info className="w-3.5 h-3.5" />}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <p className={cn("text-sm font-medium truncate", !notification.read && "text-foreground")}>{notification.title}</p>
                      {!notification.read && <span className="w-2 h-2 bg-primary rounded-full shrink-0" />}
                    </div>
                    <p className="text-xs text-muted-foreground truncate">{notification.message}</p>
                    <p className="text-[10px] text-muted-foreground mt-1">{notification.timestamp}</p>
                  </div>
                </DropdownMenuItem>
              ))}
            </div>
            <DropdownMenuSeparator />
            <DropdownMenuItem 
              className="justify-center text-sm text-primary cursor-pointer"
              onClick={() => navigate('/notifications')}
            >
              View all notifications
            </DropdownMenuItem>
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

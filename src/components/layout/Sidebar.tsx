import React, { useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { cn } from '@/lib/utils';
import {
  LayoutDashboard,
  FolderOpen,
  GitBranch,
  Coins,
  Award,
  TrendingUp,
  Target,
  Flame,
  Trophy,
  Gift,
  Users,
  Filter,
  BarChart3,
  Plug,
  FileText,
  Bell,
  Settings,
  ChevronDown,
  ChevronLeft,
  Gamepad2,
  BookOpen,
  Code,
  Book,
  Terminal,
} from 'lucide-react';

interface NavItem {
  label: string;
  path: string;
  icon: React.ReactNode;
  children?: { label: string; path: string; icon: React.ReactNode }[];
}

const navItems: NavItem[] = [
  { label: 'Overview', path: '/', icon: <LayoutDashboard className="w-5 h-5" /> },
  { label: 'Programs', path: '/programs', icon: <FolderOpen className="w-5 h-5" /> },
  { label: 'Rules', path: '/rules', icon: <GitBranch className="w-5 h-5" /> },
  {
    label: 'Mechanics',
    path: '/mechanics',
    icon: <Gamepad2 className="w-5 h-5" />,
    children: [
      { label: 'Points & Wallets', path: '/mechanics/points', icon: <Coins className="w-4 h-4" /> },
      { label: 'Badges', path: '/mechanics/badges', icon: <Award className="w-4 h-4" /> },
      { label: 'Levels', path: '/mechanics/levels', icon: <TrendingUp className="w-4 h-4" /> },
      { label: 'Missions', path: '/mechanics/missions', icon: <Target className="w-4 h-4" /> },
      { label: 'Streaks', path: '/mechanics/streaks', icon: <Flame className="w-4 h-4" /> },
      { label: 'Leaderboards', path: '/mechanics/leaderboards', icon: <Trophy className="w-4 h-4" /> },
      { label: 'Rewards', path: '/mechanics/rewards', icon: <Gift className="w-4 h-4" /> },
    ],
  },
  { label: 'Users', path: '/users', icon: <Users className="w-5 h-5" /> },
  { label: 'Segments', path: '/segments', icon: <Filter className="w-5 h-5" /> },
  { label: 'Analytics', path: '/analytics', icon: <BarChart3 className="w-5 h-5" /> },
  { label: 'Notifications', path: '/notifications', icon: <Bell className="w-5 h-5" /> },
  { label: 'Integrations', path: '/integrations', icon: <Plug className="w-5 h-5" /> },
  {
    label: 'Documentation',
    path: '/docs',
    icon: <BookOpen className="w-5 h-5" />,
    children: [
      { label: 'Overview', path: '/docs', icon: <Book className="w-4 h-4" /> },
      { label: 'API Reference', path: '/docs/api', icon: <Code className="w-4 h-4" /> },
      { label: 'User Guides', path: '/docs/guides', icon: <BookOpen className="w-4 h-4" /> },
      { label: 'Developer Docs', path: '/docs/developer', icon: <Terminal className="w-4 h-4" /> },
    ],
  },
  { label: 'Audit & Logs', path: '/audit-logs', icon: <FileText className="w-5 h-5" /> },
  { label: 'Settings', path: '/settings', icon: <Settings className="w-5 h-5" /> },
];

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
}

export default function Sidebar({ collapsed, onToggle }: SidebarProps) {
  const location = useLocation();
  const [expandedMenus, setExpandedMenus] = useState<string[]>(['Mechanics']);

  const toggleMenu = (label: string) => {
    setExpandedMenus(prev =>
      prev.includes(label)
        ? prev.filter(item => item !== label)
        : [...prev, label]
    );
  };

  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/';
    return location.pathname.startsWith(path);
  };

  return (
    <aside
      className={cn(
        "fixed left-0 top-0 h-screen bg-card/50 backdrop-blur-xl border-r border-border/50 transition-all duration-300 z-40 flex flex-col",
        collapsed ? "w-16" : "w-64"
      )}
    >
      {/* Logo */}
      <div className={cn(
        "h-16 flex items-center border-b border-border/50 px-4",
        collapsed ? "justify-center" : "gap-3"
      )}>
        <div className="w-8 h-8 rounded-lg bg-primary/20 flex items-center justify-center">
          <Gamepad2 className="w-5 h-5 text-primary" />
        </div>
        {!collapsed && (
          <span className="font-bold text-lg text-gradient">LevelUpOs</span>
        )}
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto py-4 px-2">
        <ul className="space-y-1">
          {navItems.map((item) => (
            <li key={item.label}>
              {item.children ? (
                <div>
                  <button
                    onClick={() => !collapsed && toggleMenu(item.label)}
                    className={cn(
                      "w-full nav-item",
                      collapsed ? "justify-center" : "justify-between",
                      isActive(item.path) && "active"
                    )}
                  >
                    <div className="flex items-center gap-3">
                      {item.icon}
                      {!collapsed && <span>{item.label}</span>}
                    </div>
                    {!collapsed && (
                      <ChevronDown
                        className={cn(
                          "w-4 h-4 transition-transform",
                          expandedMenus.includes(item.label) && "rotate-180"
                        )}
                      />
                    )}
                  </button>
                  {!collapsed && expandedMenus.includes(item.label) && (
                    <ul className="mt-1 ml-4 border-l border-border/50 pl-3 space-y-1">
                      {item.children.map((child) => (
                        <li key={child.path}>
                          <Link
                            to={child.path}
                            className={cn(
                              "nav-item text-sm",
                              isActive(child.path) && "active"
                            )}
                          >
                            {child.icon}
                            <span>{child.label}</span>
                          </Link>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              ) : (
                <Link
                  to={item.path}
                  className={cn(
                    "nav-item",
                    collapsed && "justify-center",
                    isActive(item.path) && "active"
                  )}
                  title={collapsed ? item.label : undefined}
                >
                  {item.icon}
                  {!collapsed && <span>{item.label}</span>}
                </Link>
              )}
            </li>
          ))}
        </ul>
      </nav>

      {/* Collapse Button */}
      <div className="p-2 border-t border-border/50">
        <button
          onClick={onToggle}
          className="w-full nav-item justify-center hover:bg-secondary"
        >
          <ChevronLeft className={cn(
            "w-5 h-5 transition-transform",
            collapsed && "rotate-180"
          )} />
        </button>
      </div>
    </aside>
  );
}

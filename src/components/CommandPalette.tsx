import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
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
  Search,
  User,
  Gamepad2,
} from "lucide-react";

const pages = [
  { name: "Overview", path: "/", icon: LayoutDashboard, keywords: ["home", "dashboard"] },
  { name: "Programs", path: "/programs", icon: FolderOpen, keywords: ["campaigns", "projects"] },
  { name: "Rules", path: "/rules", icon: GitBranch, keywords: ["automation", "logic"] },
  { name: "Points & Wallets", path: "/mechanics/points", icon: Coins, keywords: ["currency", "balance", "ledger"] },
  { name: "Badges", path: "/mechanics/badges", icon: Award, keywords: ["achievements", "trophies"] },
  { name: "Levels", path: "/mechanics/levels", icon: TrendingUp, keywords: ["xp", "experience", "progression"] },
  { name: "Missions", path: "/mechanics/missions", icon: Target, keywords: ["quests", "challenges", "objectives"] },
  { name: "Streaks", path: "/mechanics/streaks", icon: Flame, keywords: ["daily", "consecutive"] },
  { name: "Leaderboards", path: "/mechanics/leaderboards", icon: Trophy, keywords: ["rankings", "competition"] },
  { name: "Rewards", path: "/mechanics/rewards", icon: Gift, keywords: ["catalog", "redemption", "prizes"] },
  { name: "Users", path: "/users", icon: Users, keywords: ["members", "players", "accounts"] },
  { name: "Segments", path: "/segments", icon: Filter, keywords: ["groups", "cohorts", "targeting"] },
  { name: "Analytics", path: "/analytics", icon: BarChart3, keywords: ["reports", "metrics", "data"] },
  { name: "Notifications", path: "/notifications", icon: Bell, keywords: ["alerts", "messages", "email", "push"] },
  { name: "Integrations", path: "/integrations", icon: Plug, keywords: ["api", "webhooks", "sdk"] },
  { name: "Audit & Logs", path: "/audit-logs", icon: FileText, keywords: ["history", "events", "decisions"] },
  { name: "Settings", path: "/settings", icon: Settings, keywords: ["preferences", "configuration"] },
];

const mockUsers = [
  { id: "1", name: "John Doe", email: "john@example.com" },
  { id: "2", name: "Jane Smith", email: "jane@example.com" },
  { id: "3", name: "Mike Johnson", email: "mike@example.com" },
  { id: "4", name: "Sarah Wilson", email: "sarah@example.com" },
  { id: "5", name: "Chris Brown", email: "chris@example.com" },
];

const mockSegments = [
  { id: "1", name: "High Value Users", userCount: 1234 },
  { id: "2", name: "At-Risk Users", userCount: 567 },
  { id: "3", name: "Power Users", userCount: 234 },
  { id: "4", name: "New Users", userCount: 890 },
];

const mockRules = [
  { id: "1", name: "Welcome Bonus", trigger: "user.registered" },
  { id: "2", name: "Purchase Reward", trigger: "purchase.completed" },
  { id: "3", name: "Referral Bonus", trigger: "referral.completed" },
  { id: "4", name: "Daily Login Streak", trigger: "user.login" },
];

interface CommandPaletteProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}

export function CommandPalette({ open: controlledOpen, onOpenChange }: CommandPaletteProps) {
  const [internalOpen, setInternalOpen] = useState(false);
  const navigate = useNavigate();
  
  const open = controlledOpen ?? internalOpen;
  const setOpen = onOpenChange ?? setInternalOpen;

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((open) => !open);
      }
    };

    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, []);

  const runCommand = (command: () => void) => {
    setOpen(false);
    command();
  };

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <CommandInput placeholder="Search pages, users, segments, rules..." />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        
        <CommandGroup heading="Pages">
          {pages.map((page) => (
            <CommandItem
              key={page.path}
              value={`${page.name} ${page.keywords.join(" ")}`}
              onSelect={() => runCommand(() => navigate(page.path))}
            >
              <page.icon className="mr-2 h-4 w-4" />
              <span>{page.name}</span>
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="Users">
          {mockUsers.map((user) => (
            <CommandItem
              key={user.id}
              value={`user ${user.name} ${user.email}`}
              onSelect={() => runCommand(() => navigate(`/users?search=${encodeURIComponent(user.email)}`))}
            >
              <User className="mr-2 h-4 w-4" />
              <span>{user.name}</span>
              <span className="ml-2 text-xs text-muted-foreground">{user.email}</span>
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="Segments">
          {mockSegments.map((segment) => (
            <CommandItem
              key={segment.id}
              value={`segment ${segment.name}`}
              onSelect={() => runCommand(() => navigate("/segments"))}
            >
              <Filter className="mr-2 h-4 w-4" />
              <span>{segment.name}</span>
              <span className="ml-2 text-xs text-muted-foreground">{segment.userCount} users</span>
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="Rules">
          {mockRules.map((rule) => (
            <CommandItem
              key={rule.id}
              value={`rule ${rule.name} ${rule.trigger}`}
              onSelect={() => runCommand(() => navigate("/rules"))}
            >
              <GitBranch className="mr-2 h-4 w-4" />
              <span>{rule.name}</span>
              <span className="ml-2 text-xs text-muted-foreground">{rule.trigger}</span>
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="Quick Actions">
          <CommandItem
            value="create new rule"
            onSelect={() => runCommand(() => navigate("/rules/new"))}
          >
            <GitBranch className="mr-2 h-4 w-4" />
            <span>Create New Rule</span>
          </CommandItem>
          <CommandItem
            value="create new segment"
            onSelect={() => runCommand(() => navigate("/segments"))}
          >
            <Filter className="mr-2 h-4 w-4" />
            <span>Create New Segment</span>
          </CommandItem>
          <CommandItem
            value="view analytics"
            onSelect={() => runCommand(() => navigate("/analytics"))}
          >
            <BarChart3 className="mr-2 h-4 w-4" />
            <span>View Analytics</span>
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}

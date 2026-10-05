import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "@/hooks/useApi";
import { PLAYER_ENDPOINTS, RULE_ENDPOINTS, toQuery } from "@/lib/api-routes";
import type { CursorPage, Rule } from "@/services/api/types";
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
  Zap,
  BookOpen,
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
  { name: "Events", path: "/events", icon: Zap, keywords: ["triggers", "activities", "event types"] },
  { name: "Players", path: "/players", icon: Users, keywords: ["members", "users", "accounts"] },
  { name: "Segments", path: "/segments", icon: Filter, keywords: ["groups", "cohorts", "targeting"] },
  { name: "Analytics", path: "/analytics", icon: BarChart3, keywords: ["reports", "metrics", "data"] },
  { name: "Notifications", path: "/notifications", icon: Bell, keywords: ["alerts", "messages", "email", "push"] },
  { name: "Integrations", path: "/integrations", icon: Plug, keywords: ["api", "webhooks", "sdk"] },
  { name: "Audit & Logs", path: "/audit-logs", icon: FileText, keywords: ["history", "events", "decisions"] },
  { name: "Documentation", path: "/docs", icon: BookOpen, keywords: ["api", "guides", "help"] },
  { name: "Settings", path: "/settings", icon: Settings, keywords: ["preferences", "configuration", "team"] },
];

/** Minimal player shape the palette needs. */
interface PlayerHit {
  id: string;
  external_id: string;
  display_name?: string | null;
  email?: string | null;
}

interface CommandPaletteProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}

export function CommandPalette({ open: controlledOpen, onOpenChange }: CommandPaletteProps) {
  const [internalOpen, setInternalOpen] = useState(false);
  const navigate = useNavigate();
  
  const open = controlledOpen ?? internalOpen;
  const setOpen = onOpenChange ?? setInternalOpen;
  const api = useApi();

  // Loaded only while the palette is open; cmdk filters them client-side.
  const { data: players = [] } = useQuery({
    queryKey: ["command-palette", "players"],
    queryFn: async () =>
      (await api.get<CursorPage<PlayerHit>>(`${PLAYER_ENDPOINTS.LIST}${toQuery({ limit: 50 })}`, { showErrorToast: false }))
        .data?.data ?? [],
    enabled: open,
    staleTime: 60_000,
  });
  const { data: rules = [] } = useQuery({
    queryKey: ["command-palette", "rules"],
    queryFn: async () =>
      (await api.get<CursorPage<Rule>>(`${RULE_ENDPOINTS.LIST}${toQuery({ limit: 50 })}`, { showErrorToast: false }))
        .data?.data ?? [],
    enabled: open,
    staleTime: 60_000,
  });

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((open) => !open);
      }
    };

    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, [setOpen]);

  const runCommand = (command: () => void) => {
    setOpen(false);
    command();
  };

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <CommandInput placeholder="Search pages, players, rules..." />
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

        {players.length > 0 && (
          <>
            <CommandGroup heading="Players">
              {players.map((player) => (
                <CommandItem
                  key={player.id}
                  value={`player ${player.display_name ?? ""} ${player.external_id} ${player.email ?? ""} ${player.id}`}
                  onSelect={() => runCommand(() => navigate(`/players/${player.id}`))}
                >
                  <User className="mr-2 h-4 w-4" />
                  <span>{player.display_name || player.external_id}</span>
                  <span className="ml-2 text-xs text-muted-foreground">{player.email ?? player.external_id}</span>
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandSeparator />
          </>
        )}

        {rules.length > 0 && (
          <>
            <CommandGroup heading="Rules">
              {rules.map((rule) => (
                <CommandItem
                  key={rule.id}
                  value={`rule ${rule.name} ${rule.trigger_event} ${rule.id}`}
                  onSelect={() => runCommand(() => navigate(`/rules/${rule.id}`))}
                >
                  <GitBranch className="mr-2 h-4 w-4" />
                  <span>{rule.name}</span>
                  <span className="ml-2 text-xs text-muted-foreground">{rule.trigger_event}</span>
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandSeparator />
          </>
        )}

        <CommandGroup heading="Quick Actions">
          <CommandItem
            value="create new rule"
            onSelect={() => runCommand(() => navigate("/rules/new"))}
          >
            <GitBranch className="mr-2 h-4 w-4" />
            <span>Create New Rule</span>
          </CommandItem>
          <CommandItem
            value="simulate rules decisions"
            onSelect={() => runCommand(() => navigate("/rules"))}
          >
            <Search className="mr-2 h-4 w-4" />
            <span>Simulate Rules / View Decisions</span>
          </CommandItem>
          <CommandItem
            value="activity log send test activity"
            onSelect={() => runCommand(() => navigate("/events"))}
          >
            <Zap className="mr-2 h-4 w-4" />
            <span>Activity Log</span>
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}

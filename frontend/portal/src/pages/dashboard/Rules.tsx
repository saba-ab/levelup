import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  Plus,
  Search,
  MoreHorizontal,
  Play,
  Pause,
  Pencil,
  Archive,
  GitBranch,
  Zap,
  Clock,
  Trash2,
  FlaskConical,
  Copy,
  BarChart3,
} from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { cn } from '@/lib/utils';
import { useToast } from '@/hooks/use-toast';
import { useAuth } from '@/contexts/AuthContext';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useRulesService } from '@/services/api/rules';
import {
  useRulesQuery,
  useUpdateRuleMutation,
  useDeleteRuleMutation,
  usePublishRuleMutation,
  useRuleDecisionsQuery,
  useRuleStatsQuery,
  useDuplicateRuleMutation,
  ApiRequestError,
  ruleKeys,
} from '@/services/queries/rules';
import type { RuleStat } from '@/services/api/models/rules';
import { useQueryClient } from '@tanstack/react-query';
import type { Rule, RuleStatus, RuleDecision } from '@/services/api/types';
import CursorPager from '@/components/CursorPager';
import RuleSimulator from '@/components/RuleSimulator';
import { DecisionDetail } from '@/components/RuleTrace';

const statusClass: Record<RuleStatus, string> = {
  active: 'border-green-500/50 text-green-500 bg-green-500/10',
  draft: 'border-amber-500/50 text-amber-500 bg-amber-500/10',
  inactive: 'border-blue-500/50 text-blue-500 bg-blue-500/10',
  archived: 'border-muted-foreground/50 text-muted-foreground bg-muted',
};

const triggerIcons: Record<string, string> = {
  purchase_completed: '🛒',
  user_signup: '👋',
  user_login: '🔐',
  referral_completed: '🔗',
  subscription_upgraded: '⬆️',
};

const STATS_PERIODS = [7, 30, 90] as const;
type StatsPeriod = (typeof STATS_PERIODS)[number];

/** YYYY-MM-DD (UTC) of `days` days ago: the stats window starts at that UTC midnight. */
function utcDateDaysAgo(days: number): string {
  return new Date(Date.now() - days * 86_400_000).toISOString().slice(0, 10);
}

function StatsSummary({
  period,
  onPeriodChange,
  totals,
  isLoading,
  error,
}: {
  period: StatsPeriod;
  onPeriodChange: (p: StatsPeriod) => void;
  totals: { fired: number; limited: number; points_awarded: number; xp_awarded: number } | undefined;
  isLoading: boolean;
  error: Error | null;
}) {
  const items = [
    { label: 'Rules fired', value: totals?.fired },
    { label: 'Refused by limits', value: totals?.limited },
    { label: 'Points awarded', value: totals?.points_awarded },
    { label: 'XP awarded', value: totals?.xp_awarded },
  ];
  return (
    <Card>
      <CardContent className="p-4 space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <BarChart3 className="w-4 h-4 text-primary" />
            <span className="text-sm font-medium">Rule activity</span>
          </div>
          <Select value={String(period)} onValueChange={v => onPeriodChange(Number(v) as StatsPeriod)}>
            <SelectTrigger className="w-[140px] h-8">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STATS_PERIODS.map(p => (
                <SelectItem key={p} value={String(p)}>
                  Last {p} days
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : (
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            {items.map(i => (
              <div key={i.label}>
                {isLoading ? (
                  <Skeleton className="h-7 w-16 mb-1" />
                ) : (
                  <p className="text-2xl font-bold tabular-nums">{(i.value ?? 0).toLocaleString()}</p>
                )}
                <p className="text-xs text-muted-foreground">{i.label}</p>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function DecisionsTab() {
  const pager = useCursorPagination(25);
  const [playerId, setPlayerId] = useState('');
  const [selected, setSelected] = useState<RuleDecision | null>(null);
  const { data, isLoading, isFetching, error } = useRuleDecisionsQuery({
    limit: pager.limit,
    cursor: pager.cursor,
    player_id: playerId.trim() || undefined,
  });
  const decisions = data?.data ?? [];

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="p-4">
          <Input
            placeholder="Filter by player id (UUID)"
            value={playerId}
            onChange={e => {
              setPlayerId(e.target.value);
              pager.reset();
            }}
            className="max-w-sm"
          />
        </CardContent>
      </Card>
      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Event type</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Player</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Outcome</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Evaluated</th>
                  <th className="text-right p-4 text-sm font-medium text-muted-foreground">Duration</th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  <tr>
                    <td colSpan={5} className="p-4">
                      <Skeleton className="h-5 w-full" />
                    </td>
                  </tr>
                ) : error ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center text-destructive">{error.message}</td>
                  </tr>
                ) : decisions.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center text-muted-foreground">
                      No decisions yet. A decision is recorded each time rules evaluate an activity.
                    </td>
                  </tr>
                ) : (
                  decisions.map(d => (
                    <tr
                      key={d.id}
                      className="border-b border-border/50 hover:bg-secondary/30 cursor-pointer"
                      onClick={() => setSelected(d)}
                    >
                      <td className="p-4">
                        <code className="text-sm bg-secondary px-2 py-1 rounded">{d.event_type}</code>
                      </td>
                      <td className="p-4 text-sm">{d.player_external_id ?? d.player_id ?? '—'}</td>
                      <td className="p-4">
                        <Badge variant="outline" className={cn(d.outcome === 'matched' && 'border-green-500/50 text-green-500')}>
                          {d.outcome}
                        </Badge>
                      </td>
                      <td className="p-4 text-sm text-muted-foreground">{new Date(d.evaluated_at).toLocaleString()}</td>
                      <td className="p-4 text-sm text-muted-foreground text-right">{d.duration_us} µs</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
          <CursorPager
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            nextCursor={data?.next_cursor}
            onPrevious={pager.previous}
            onNext={pager.next}
            isFetching={isFetching}
          />
        </CardContent>
      </Card>
      <Dialog open={!!selected} onOpenChange={o => !o && setSelected(null)}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Decision</DialogTitle>
            <DialogDescription>
              Activity <code>{selected?.activity_id}</code>
            </DialogDescription>
          </DialogHeader>
          {selected && <DecisionDetail decisionId={selected.id} />}
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default function Rules() {
  const navigate = useNavigate();
  const { toast } = useToast();
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:rules');
  const queryClient = useQueryClient();
  const { getRule } = useRulesService();
  const pager = useCursorPagination(25);
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | RuleStatus>('all');
  const [selectedRules, setSelectedRules] = useState<string[]>([]);
  const [simulateRule, setSimulateRule] = useState<Rule | null>(null);
  const [ruleToDelete, setRuleToDelete] = useState<Rule | null>(null);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [statsPeriod, setStatsPeriod] = useState<StatsPeriod>(30);

  const { data, isLoading, isFetching, error } = useRulesQuery({
    limit: pager.limit,
    cursor: pager.cursor,
    status: statusFilter === 'all' ? undefined : statusFilter,
  });
  const updateRule = useUpdateRuleMutation();
  const deleteRule = useDeleteRuleMutation();
  const publishRule = usePublishRuleMutation();
  const duplicateRule = useDuplicateRuleMutation();

  const statsQuery = useRuleStatsQuery({ from: utcDateDaysAgo(statsPeriod) });
  /** Stats need rules.view_decisions: without it the column and summary are hidden. */
  const statsForbidden = statsQuery.error instanceof ApiRequestError && statsQuery.error.status === 403;
  const statsByRule = new Map<string, RuleStat>((statsQuery.data?.data ?? []).map(r => [r.rule_id, r]));
  const columnCount = (canManage ? 7 : 6) + (statsForbidden ? 0 : 1);

  const rules = (data?.data ?? []).filter(r => {
    const q = searchQuery.trim().toLowerCase();
    return !q || r.name.toLowerCase().includes(q) || r.trigger_event.toLowerCase().includes(q);
  });

  const toggleSelect = (id: string) =>
    setSelectedRules(prev => (prev.includes(id) ? prev.filter(i => i !== id) : [...prev, id]));

  const toggleSelectAll = () =>
    setSelectedRules(selectedRules.length === rules.length ? [] : rules.map(r => r.id));

  /** Publishes the rule's latest version (the list has no versions, so load the rule first). */
  const publishLatest = async (rule: Rule) => {
    const res = await getRule(rule.id);
    const version = res.data?.latest_version?.version;
    if (!res.success || !version) throw new Error(res.error || 'Rule has no version to publish');
    await publishRule.mutateAsync({ ruleId: rule.id, version });
  };

  const setStatus = async (rule: Rule, status: 'active' | 'inactive' | 'archived') => {
    await updateRule.mutateAsync({ ruleId: rule.id, data: { status } });
  };

  const runAction = async (label: string, fn: () => Promise<void>) => {
    try {
      await fn();
      toast({ title: label });
    } catch (err) {
      toast({ title: 'Error', description: err instanceof Error ? err.message : 'Action failed', variant: 'destructive' });
    }
  };

  const bulkAction = async (action: 'publish' | 'archive') => {
    const targets = (data?.data ?? []).filter(r => selectedRules.includes(r.id));
    setBulkBusy(true);
    const results = await Promise.allSettled(
      targets.map(r => (action === 'publish' ? publishLatest(r) : setStatus(r, 'archived'))),
    );
    setBulkBusy(false);
    const failed = results.filter(r => r.status === 'rejected').length;
    queryClient.invalidateQueries({ queryKey: ruleKeys.all });
    toast({
      title: `${targets.length - failed} rule(s) ${action === 'publish' ? 'published' : 'archived'}`,
      description: failed ? `${failed} failed.` : undefined,
      variant: failed ? 'destructive' : undefined,
    });
    setSelectedRules([]);
  };

  const handleDuplicate = async (rule: Rule) => {
    try {
      const copy = await duplicateRule.mutateAsync(rule.id);
      toast({ title: 'Rule duplicated', description: `"${copy.name}" was created as a draft; it is not live until published.` });
    } catch (err) {
      toast({ title: 'Error', description: err instanceof Error ? err.message : 'Failed to duplicate rule', variant: 'destructive' });
    }
  };

  const handleDelete = async () => {
    if (!ruleToDelete) return;
    const rule = ruleToDelete;
    setRuleToDelete(null);
    await runAction('Rule deleted', async () => {
      await deleteRule.mutateAsync(rule.id);
    });
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Rules Engine</h1>
          <p className="text-muted-foreground mt-1">Create and manage your gamification rules.</p>
        </div>
        {canManage && (
          <Link to="/rules/new">
            <Button variant="glow">
              <Plus className="w-4 h-4" />
              Create Rule
            </Button>
          </Link>
        )}
      </div>

      <Tabs defaultValue="rules" className="space-y-6">
        <TabsList>
          <TabsTrigger value="rules">Rules</TabsTrigger>
          <TabsTrigger value="simulate">Simulate</TabsTrigger>
          <TabsTrigger value="decisions">Decisions</TabsTrigger>
        </TabsList>

        <TabsContent value="rules" className="space-y-6">
          {!statsForbidden && (
            <StatsSummary
              period={statsPeriod}
              onPeriodChange={setStatsPeriod}
              totals={statsQuery.data?.totals}
              isLoading={statsQuery.isLoading}
              error={statsQuery.error}
            />
          )}

          <Card>
            <CardContent className="p-4">
              <div className="flex flex-col sm:flex-row gap-4">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                  <Input
                    placeholder="Search this page by name or trigger..."
                    className="pl-9"
                    value={searchQuery}
                    onChange={e => setSearchQuery(e.target.value)}
                  />
                </div>
                <Select
                  value={statusFilter}
                  onValueChange={v => {
                    setStatusFilter(v as typeof statusFilter);
                    pager.reset();
                    setSelectedRules([]);
                  }}
                >
                  <SelectTrigger className="w-full sm:w-[180px]">
                    <SelectValue placeholder="Filter by status" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Status</SelectItem>
                    <SelectItem value="active">Active</SelectItem>
                    <SelectItem value="draft">Draft</SelectItem>
                    <SelectItem value="inactive">Inactive</SelectItem>
                    <SelectItem value="archived">Archived</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>

          {canManage && selectedRules.length > 0 && (
            <Card className="bg-primary/5 border-primary/20">
              <CardContent className="p-4 flex items-center justify-between">
                <span className="text-sm font-medium">
                  {selectedRules.length} rule{selectedRules.length > 1 ? 's' : ''} selected
                </span>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" disabled={bulkBusy} onClick={() => bulkAction('publish')}>
                    <Play className="w-4 h-4 mr-1" />
                    Publish latest
                  </Button>
                  <Button variant="outline" size="sm" disabled={bulkBusy} onClick={() => bulkAction('archive')}>
                    <Archive className="w-4 h-4 mr-1" />
                    Archive
                  </Button>
                </div>
              </CardContent>
            </Card>
          )}

          <Card>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-border">
                      {canManage && (
                        <th className="p-4 w-12">
                          <Checkbox
                            checked={selectedRules.length === rules.length && rules.length > 0}
                            onCheckedChange={toggleSelectAll}
                          />
                        </th>
                      )}
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Rule Name</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Trigger Event</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Priority</th>
                      {!statsForbidden && (
                        <th
                          className="text-right p-4 text-sm font-medium text-muted-foreground"
                          title={`Times the rule fired in the last ${statsPeriod} days`}
                        >
                          Fired ({statsPeriod}d)
                        </th>
                      )}
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Last Modified</th>
                      <th className="text-right p-4 text-sm font-medium text-muted-foreground">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {isLoading ? (
                      [...Array(4)].map((_, i) => (
                        <tr key={i} className="border-b border-border/50">
                          <td colSpan={columnCount} className="p-4">
                            <Skeleton className="h-8 w-full" />
                          </td>
                        </tr>
                      ))
                    ) : error ? (
                      <tr>
                        <td colSpan={columnCount} className="p-8 text-center text-destructive">{error.message}</td>
                      </tr>
                    ) : rules.length === 0 ? (
                      <tr>
                        <td colSpan={columnCount} className="p-8 text-center">
                          <div className="flex flex-col items-center gap-3">
                            <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center">
                              <GitBranch className="w-8 h-8 text-muted-foreground" />
                            </div>
                            <div>
                              <p className="font-medium">No rules found</p>
                              <p className="text-sm text-muted-foreground">Create your first rule to start gamifying.</p>
                            </div>
                          </div>
                        </td>
                      </tr>
                    ) : (
                      rules.map(rule => (
                        <tr key={rule.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors">
                          {canManage && (
                            <td className="p-4">
                              <Checkbox
                                checked={selectedRules.includes(rule.id)}
                                onCheckedChange={() => toggleSelect(rule.id)}
                              />
                            </td>
                          )}
                          <td className="p-4">
                            <div className="flex items-center gap-3">
                              <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
                                <GitBranch className="w-5 h-5 text-primary" />
                              </div>
                              <div>
                                <span className="font-medium">{rule.name}</span>
                                {rule.description && (
                                  <p className="text-xs text-muted-foreground line-clamp-1">{rule.description}</p>
                                )}
                              </div>
                            </div>
                          </td>
                          <td className="p-4">
                            <div className="flex items-center gap-2">
                              <span className="text-lg">{triggerIcons[rule.trigger_event] ?? '⚡'}</span>
                              <code className="text-sm bg-secondary px-2 py-1 rounded">{rule.trigger_event}</code>
                            </div>
                          </td>
                          <td className="p-4">
                            <Badge variant="outline" className={cn('capitalize', statusClass[rule.status])}>
                              {rule.status}
                            </Badge>
                          </td>
                          <td className="p-4">
                            <span className="text-muted-foreground">{rule.priority}</span>
                          </td>
                          {!statsForbidden && (
                            <td className="p-4 text-right">
                              {statsQuery.isLoading ? (
                                <Skeleton className="h-4 w-10 ml-auto" />
                              ) : statsQuery.error ? (
                                <span className="text-muted-foreground">—</span>
                              ) : (
                                <div>
                                  <span className="font-medium tabular-nums">
                                    {(statsByRule.get(rule.id)?.fired ?? 0).toLocaleString()}
                                  </span>
                                  {(statsByRule.get(rule.id)?.limited ?? 0) > 0 && (
                                    <p className="text-xs text-muted-foreground">
                                      {statsByRule.get(rule.id)!.limited.toLocaleString()} limited
                                    </p>
                                  )}
                                </div>
                              )}
                            </td>
                          )}
                          <td className="p-4">
                            <div className="flex items-center gap-2 text-muted-foreground">
                              <Clock className="w-4 h-4" />
                              <span className="text-sm">{new Date(rule.updated_at).toLocaleDateString()}</span>
                            </div>
                          </td>
                          <td className="p-4 text-right">
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button variant="ghost" size="icon">
                                  <MoreHorizontal className="w-4 h-4" />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end">
                                <DropdownMenuItem onClick={() => setSimulateRule(rule)}>
                                  <Zap className="w-4 h-4 mr-2" />
                                  Simulate
                                </DropdownMenuItem>
                                {canManage && (
                                  <>
                                    <DropdownMenuItem onClick={() => navigate(`/rules/${rule.id}`)}>
                                      <Pencil className="w-4 h-4 mr-2" />
                                      Edit
                                    </DropdownMenuItem>
                                    <DropdownMenuItem disabled={duplicateRule.isPending} onClick={() => handleDuplicate(rule)}>
                                      <Copy className="w-4 h-4 mr-2" />
                                      Duplicate
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                      onClick={() => runAction('Latest version published', () => publishLatest(rule))}
                                    >
                                      <Play className="w-4 h-4 mr-2" />
                                      Publish latest version
                                    </DropdownMenuItem>
                                    {rule.status === 'active' ? (
                                      <DropdownMenuItem onClick={() => runAction('Rule paused', () => setStatus(rule, 'inactive'))}>
                                        <Pause className="w-4 h-4 mr-2" />
                                        Pause
                                      </DropdownMenuItem>
                                    ) : (
                                      rule.current_version_id && (
                                        <DropdownMenuItem
                                          onClick={() => runAction('Rule activated', () => setStatus(rule, 'active'))}
                                        >
                                          <Play className="w-4 h-4 mr-2" />
                                          Activate
                                        </DropdownMenuItem>
                                      )
                                    )}
                                    {rule.status !== 'archived' && (
                                      <DropdownMenuItem onClick={() => runAction('Rule archived', () => setStatus(rule, 'archived'))}>
                                        <Archive className="w-4 h-4 mr-2" />
                                        Archive
                                      </DropdownMenuItem>
                                    )}
                                    <DropdownMenuSeparator />
                                    <DropdownMenuItem className="text-destructive" onClick={() => setRuleToDelete(rule)}>
                                      <Trash2 className="w-4 h-4 mr-2" />
                                      Delete
                                    </DropdownMenuItem>
                                  </>
                                )}
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
              <CursorPager
                page={pager.page}
                hasPrevious={pager.hasPrevious}
                nextCursor={data?.next_cursor}
                onPrevious={() => {
                  pager.previous();
                  setSelectedRules([]);
                }}
                onNext={c => {
                  pager.next(c);
                  setSelectedRules([]);
                }}
                isFetching={isFetching}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="simulate">
          <Card>
            <CardContent className="p-6 space-y-2">
              <div className="flex items-center gap-2 mb-2">
                <FlaskConical className="w-5 h-5 text-primary" />
                <h2 className="text-lg font-semibold">Simulate an activity</h2>
              </div>
              <RuleSimulator canSendActivity={canManage} />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="decisions">
          <DecisionsTab />
        </TabsContent>
      </Tabs>

      <Dialog open={!!simulateRule} onOpenChange={o => !o && setSimulateRule(null)}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Simulate "{simulateRule?.name}"</DialogTitle>
            <DialogDescription>
              Evaluates a hypothetical <code>{simulateRule?.trigger_event}</code> activity against the live ruleset.
            </DialogDescription>
          </DialogHeader>
          {simulateRule && (
            <RuleSimulator
              defaultEventType={simulateRule.trigger_event}
              focusRuleId={simulateRule.id}
              canSendActivity={canManage}
            />
          )}
        </DialogContent>
      </Dialog>

      <AlertDialog open={!!ruleToDelete} onOpenChange={o => !o && setRuleToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete rule</AlertDialogTitle>
            <AlertDialogDescription>
              Delete "{ruleToDelete?.name}"? It stops evaluating new activities. Past decisions are kept.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

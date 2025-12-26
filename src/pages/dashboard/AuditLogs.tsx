import { useState } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Search, Filter, ChevronDown, ChevronRight, FileText, Activity, Webhook, CheckCircle2, XCircle, Clock, AlertTriangle, Download, RefreshCw } from "lucide-react";

const decisionLogs = [
  { id: "1", ruleId: "rule_001", ruleName: "Welcome Bonus", userId: "user_123", decision: "granted", points: 100, timestamp: "2024-01-20 14:32:15", executionTime: 12, conditions: [{ name: "is_new_user", result: true }, { name: "email_verified", result: true }] },
  { id: "2", ruleId: "rule_002", ruleName: "Purchase Reward", userId: "user_456", decision: "granted", points: 50, timestamp: "2024-01-20 14:30:42", executionTime: 8, conditions: [{ name: "min_purchase_amount", result: true, value: "$25.00" }] },
  { id: "3", ruleId: "rule_003", ruleName: "Referral Bonus", userId: "user_789", decision: "denied", points: 0, timestamp: "2024-01-20 14:28:10", executionTime: 15, conditions: [{ name: "valid_referral", result: false }, { name: "not_self_referral", result: true }] },
  { id: "4", ruleId: "rule_001", ruleName: "Welcome Bonus", userId: "user_234", decision: "denied", points: 0, timestamp: "2024-01-20 14:25:00", executionTime: 5, conditions: [{ name: "is_new_user", result: false }] },
  { id: "5", ruleId: "rule_004", ruleName: "Daily Login Streak", userId: "user_567", decision: "granted", points: 25, timestamp: "2024-01-20 14:22:30", executionTime: 10, conditions: [{ name: "consecutive_days", result: true, value: "7 days" }] },
];

const eventLogs = [
  { id: "1", eventType: "points.earned", userId: "user_123", source: "api", data: { amount: 100, reason: "purchase", orderId: "ord_abc123" }, timestamp: "2024-01-20 14:32:15", ip: "192.168.1.1" },
  { id: "2", eventType: "badge.unlocked", userId: "user_456", source: "rule_engine", data: { badgeId: "early_adopter", badgeName: "Early Adopter" }, timestamp: "2024-01-20 14:30:42", ip: "192.168.1.2" },
  { id: "3", eventType: "level.up", userId: "user_789", source: "rule_engine", data: { previousLevel: 4, newLevel: 5, xpRequired: 500 }, timestamp: "2024-01-20 14:28:10", ip: "192.168.1.3" },
  { id: "4", eventType: "mission.completed", userId: "user_234", source: "api", data: { missionId: "mission_001", missionName: "First Purchase", reward: 200 }, timestamp: "2024-01-20 14:25:00", ip: "192.168.1.4" },
  { id: "5", eventType: "reward.redeemed", userId: "user_567", source: "api", data: { rewardId: "reward_001", rewardName: "$10 Gift Card", cost: 1000 }, timestamp: "2024-01-20 14:22:30", ip: "192.168.1.5" },
  { id: "6", eventType: "streak.milestone", userId: "user_890", source: "rule_engine", data: { streakDays: 30, milestone: "30-day streak", bonusPoints: 500 }, timestamp: "2024-01-20 14:20:00", ip: "192.168.1.6" },
];

const webhookLogs = [
  { id: "1", webhookId: "wh_001", url: "https://api.example.com/webhooks/gamify", event: "points.earned", status: "success", statusCode: 200, responseTime: 125, timestamp: "2024-01-20 14:32:16", requestBody: { userId: "user_123", points: 100 }, responseBody: { received: true } },
  { id: "2", webhookId: "wh_001", url: "https://api.example.com/webhooks/gamify", event: "badge.unlocked", status: "success", statusCode: 200, responseTime: 98, timestamp: "2024-01-20 14:30:43", requestBody: { userId: "user_456", badgeId: "early_adopter" }, responseBody: { received: true } },
  { id: "3", webhookId: "wh_002", url: "https://hooks.slack.com/services/xxx", event: "level.up", status: "success", statusCode: 200, responseTime: 210, timestamp: "2024-01-20 14:28:11", requestBody: { userId: "user_789", level: 5 }, responseBody: { ok: true } },
  { id: "4", webhookId: "wh_003", url: "https://webhook.site/test", event: "mission.completed", status: "failed", statusCode: 500, responseTime: 5000, timestamp: "2024-01-20 14:25:01", requestBody: { userId: "user_234", missionId: "mission_001" }, responseBody: { error: "Internal server error" } },
  { id: "5", webhookId: "wh_001", url: "https://api.example.com/webhooks/gamify", event: "reward.redeemed", status: "success", statusCode: 200, responseTime: 145, timestamp: "2024-01-20 14:22:31", requestBody: { userId: "user_567", rewardId: "reward_001" }, responseBody: { received: true } },
  { id: "6", webhookId: "wh_002", url: "https://hooks.slack.com/services/xxx", event: "streak.milestone", status: "retry", statusCode: 429, responseTime: 50, timestamp: "2024-01-20 14:20:01", requestBody: { userId: "user_890", milestone: "30-day" }, responseBody: { error: "Rate limited" } },
];

const eventTypes = ["all", "points.earned", "points.redeemed", "badge.unlocked", "level.up", "mission.completed", "reward.redeemed", "streak.milestone"];

export default function AuditLogs() {
  const [decisionSearch, setDecisionSearch] = useState("");
  const [decisionFilter, setDecisionFilter] = useState("all");
  const [expandedDecisions, setExpandedDecisions] = useState<Record<string, boolean>>({});
  
  const [eventSearch, setEventSearch] = useState("");
  const [eventFilter, setEventFilter] = useState("all");
  const [expandedEvents, setExpandedEvents] = useState<Record<string, boolean>>({});
  
  const [webhookSearch, setWebhookSearch] = useState("");
  const [webhookFilter, setWebhookFilter] = useState("all");
  const [expandedWebhooks, setExpandedWebhooks] = useState<Record<string, boolean>>({});

  const toggleExpanded = (id: string, setter: React.Dispatch<React.SetStateAction<Record<string, boolean>>>) => {
    setter(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const filteredDecisions = decisionLogs.filter(log => {
    const matchesSearch = log.ruleName.toLowerCase().includes(decisionSearch.toLowerCase()) ||
                          log.userId.toLowerCase().includes(decisionSearch.toLowerCase());
    const matchesFilter = decisionFilter === "all" || log.decision === decisionFilter;
    return matchesSearch && matchesFilter;
  });

  const filteredEvents = eventLogs.filter(log => {
    const matchesSearch = log.eventType.toLowerCase().includes(eventSearch.toLowerCase()) ||
                          log.userId.toLowerCase().includes(eventSearch.toLowerCase());
    const matchesFilter = eventFilter === "all" || log.eventType === eventFilter;
    return matchesSearch && matchesFilter;
  });

  const filteredWebhooks = webhookLogs.filter(log => {
    const matchesSearch = log.url.toLowerCase().includes(webhookSearch.toLowerCase()) ||
                          log.event.toLowerCase().includes(webhookSearch.toLowerCase());
    const matchesFilter = webhookFilter === "all" || log.status === webhookFilter;
    return matchesSearch && matchesFilter;
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Audit & Logs</h1>
          <p className="text-muted-foreground">Monitor decisions, events, and webhook deliveries</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline">
            <Download className="h-4 w-4 mr-2" />
            Export
          </Button>
          <Button variant="outline">
            <RefreshCw className="h-4 w-4 mr-2" />
            Refresh
          </Button>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Total Events (24h)</CardDescription>
            <CardTitle className="text-2xl">12,847</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Rules Evaluated</CardDescription>
            <CardTitle className="text-2xl">8,432</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Webhook Deliveries</CardDescription>
            <CardTitle className="text-2xl">3,215</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Error Rate</CardDescription>
            <CardTitle className="text-2xl text-destructive">0.8%</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <Tabs defaultValue="decisions" className="space-y-6">
        <TabsList className="grid w-full max-w-md grid-cols-3">
          <TabsTrigger value="decisions" className="flex items-center gap-2">
            <FileText className="h-4 w-4" />
            Decision Logs
          </TabsTrigger>
          <TabsTrigger value="events" className="flex items-center gap-2">
            <Activity className="h-4 w-4" />
            Event Explorer
          </TabsTrigger>
          <TabsTrigger value="webhooks" className="flex items-center gap-2">
            <Webhook className="h-4 w-4" />
            Webhook Logs
          </TabsTrigger>
        </TabsList>

        {/* Decision Logs Tab */}
        <TabsContent value="decisions" className="space-y-4">
          <div className="flex gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by rule name or user ID..."
                className="pl-10"
                value={decisionSearch}
                onChange={(e) => setDecisionSearch(e.target.value)}
              />
            </div>
            <Select value={decisionFilter} onValueChange={setDecisionFilter}>
              <SelectTrigger className="w-40">
                <Filter className="h-4 w-4 mr-2" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Decisions</SelectItem>
                <SelectItem value="granted">Granted</SelectItem>
                <SelectItem value="denied">Denied</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <Card>
            <CardContent className="p-0">
              <ScrollArea className="h-[500px]">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10"></TableHead>
                      <TableHead>Rule</TableHead>
                      <TableHead>User</TableHead>
                      <TableHead>Decision</TableHead>
                      <TableHead>Points</TableHead>
                      <TableHead>Exec Time</TableHead>
                      <TableHead>Timestamp</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredDecisions.map((log) => (
                      <Collapsible key={log.id} open={expandedDecisions[log.id]} onOpenChange={() => toggleExpanded(log.id, setExpandedDecisions)} asChild>
                        <>
                          <CollapsibleTrigger asChild>
                            <TableRow className="cursor-pointer hover:bg-muted/50">
                              <TableCell>
                                {expandedDecisions[log.id] ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                              </TableCell>
                              <TableCell>
                                <div>
                                  <p className="font-medium">{log.ruleName}</p>
                                  <p className="text-xs text-muted-foreground">{log.ruleId}</p>
                                </div>
                              </TableCell>
                              <TableCell className="font-mono text-sm">{log.userId}</TableCell>
                              <TableCell>
                                <Badge variant={log.decision === "granted" ? "default" : "destructive"}>
                                  {log.decision === "granted" ? <CheckCircle2 className="h-3 w-3 mr-1" /> : <XCircle className="h-3 w-3 mr-1" />}
                                  {log.decision}
                                </Badge>
                              </TableCell>
                              <TableCell>{log.points > 0 ? `+${log.points}` : "-"}</TableCell>
                              <TableCell>{log.executionTime}ms</TableCell>
                              <TableCell className="text-muted-foreground">{log.timestamp}</TableCell>
                            </TableRow>
                          </CollapsibleTrigger>
                          <CollapsibleContent asChild>
                            <TableRow className="bg-muted/30">
                              <TableCell colSpan={7} className="p-4">
                                <div className="space-y-2">
                                  <p className="text-sm font-medium">Condition Evaluation:</p>
                                  <div className="grid gap-2">
                                    {log.conditions.map((cond, idx) => (
                                      <div key={idx} className="flex items-center gap-2 text-sm">
                                        {cond.result ? (
                                          <CheckCircle2 className="h-4 w-4 text-green-500" />
                                        ) : (
                                          <XCircle className="h-4 w-4 text-destructive" />
                                        )}
                                        <code className="bg-muted px-2 py-0.5 rounded">{cond.name}</code>
                                        <span className="text-muted-foreground">→ {cond.result ? "true" : "false"}</span>
                                        {cond.value && <Badge variant="outline">{cond.value}</Badge>}
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              </TableCell>
                            </TableRow>
                          </CollapsibleContent>
                        </>
                      </Collapsible>
                    ))}
                  </TableBody>
                </Table>
              </ScrollArea>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Event Explorer Tab */}
        <TabsContent value="events" className="space-y-4">
          <div className="flex gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by event type or user ID..."
                className="pl-10"
                value={eventSearch}
                onChange={(e) => setEventSearch(e.target.value)}
              />
            </div>
            <Select value={eventFilter} onValueChange={setEventFilter}>
              <SelectTrigger className="w-48">
                <Filter className="h-4 w-4 mr-2" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {eventTypes.map((type) => (
                  <SelectItem key={type} value={type}>
                    {type === "all" ? "All Events" : type}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <Card>
            <CardContent className="p-0">
              <ScrollArea className="h-[500px]">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10"></TableHead>
                      <TableHead>Event Type</TableHead>
                      <TableHead>User</TableHead>
                      <TableHead>Source</TableHead>
                      <TableHead>IP Address</TableHead>
                      <TableHead>Timestamp</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredEvents.map((log) => (
                      <Collapsible key={log.id} open={expandedEvents[log.id]} onOpenChange={() => toggleExpanded(log.id, setExpandedEvents)} asChild>
                        <>
                          <CollapsibleTrigger asChild>
                            <TableRow className="cursor-pointer hover:bg-muted/50">
                              <TableCell>
                                {expandedEvents[log.id] ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                              </TableCell>
                              <TableCell>
                                <Badge variant="outline">{log.eventType}</Badge>
                              </TableCell>
                              <TableCell className="font-mono text-sm">{log.userId}</TableCell>
                              <TableCell>
                                <Badge variant="secondary">{log.source}</Badge>
                              </TableCell>
                              <TableCell className="font-mono text-sm text-muted-foreground">{log.ip}</TableCell>
                              <TableCell className="text-muted-foreground">{log.timestamp}</TableCell>
                            </TableRow>
                          </CollapsibleTrigger>
                          <CollapsibleContent asChild>
                            <TableRow className="bg-muted/30">
                              <TableCell colSpan={6} className="p-4">
                                <div className="space-y-2">
                                  <p className="text-sm font-medium">Event Data:</p>
                                  <pre className="bg-muted p-3 rounded-lg text-sm overflow-x-auto">
                                    <code>{JSON.stringify(log.data, null, 2)}</code>
                                  </pre>
                                </div>
                              </TableCell>
                            </TableRow>
                          </CollapsibleContent>
                        </>
                      </Collapsible>
                    ))}
                  </TableBody>
                </Table>
              </ScrollArea>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Webhook Logs Tab */}
        <TabsContent value="webhooks" className="space-y-4">
          <div className="flex gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by URL or event..."
                className="pl-10"
                value={webhookSearch}
                onChange={(e) => setWebhookSearch(e.target.value)}
              />
            </div>
            <Select value={webhookFilter} onValueChange={setWebhookFilter}>
              <SelectTrigger className="w-40">
                <Filter className="h-4 w-4 mr-2" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Status</SelectItem>
                <SelectItem value="success">Success</SelectItem>
                <SelectItem value="failed">Failed</SelectItem>
                <SelectItem value="retry">Retry</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <Card>
            <CardContent className="p-0">
              <ScrollArea className="h-[500px]">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-10"></TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Event</TableHead>
                      <TableHead>Endpoint</TableHead>
                      <TableHead>Response</TableHead>
                      <TableHead>Duration</TableHead>
                      <TableHead>Timestamp</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredWebhooks.map((log) => (
                      <Collapsible key={log.id} open={expandedWebhooks[log.id]} onOpenChange={() => toggleExpanded(log.id, setExpandedWebhooks)} asChild>
                        <>
                          <CollapsibleTrigger asChild>
                            <TableRow className="cursor-pointer hover:bg-muted/50">
                              <TableCell>
                                {expandedWebhooks[log.id] ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                              </TableCell>
                              <TableCell>
                                {log.status === "success" ? (
                                  <CheckCircle2 className="h-5 w-5 text-green-500" />
                                ) : log.status === "failed" ? (
                                  <XCircle className="h-5 w-5 text-destructive" />
                                ) : (
                                  <AlertTriangle className="h-5 w-5 text-yellow-500" />
                                )}
                              </TableCell>
                              <TableCell>
                                <Badge variant="outline">{log.event}</Badge>
                              </TableCell>
                              <TableCell className="font-mono text-sm max-w-[200px] truncate">{log.url}</TableCell>
                              <TableCell>
                                <Badge variant={log.statusCode >= 200 && log.statusCode < 300 ? "default" : "destructive"}>
                                  {log.statusCode}
                                </Badge>
                              </TableCell>
                              <TableCell>{log.responseTime}ms</TableCell>
                              <TableCell className="text-muted-foreground">{log.timestamp}</TableCell>
                            </TableRow>
                          </CollapsibleTrigger>
                          <CollapsibleContent asChild>
                            <TableRow className="bg-muted/30">
                              <TableCell colSpan={7} className="p-4">
                                <div className="grid gap-4 md:grid-cols-2">
                                  <div className="space-y-2">
                                    <p className="text-sm font-medium">Request Body:</p>
                                    <pre className="bg-muted p-3 rounded-lg text-sm overflow-x-auto">
                                      <code>{JSON.stringify(log.requestBody, null, 2)}</code>
                                    </pre>
                                  </div>
                                  <div className="space-y-2">
                                    <p className="text-sm font-medium">Response Body:</p>
                                    <pre className="bg-muted p-3 rounded-lg text-sm overflow-x-auto">
                                      <code>{JSON.stringify(log.responseBody, null, 2)}</code>
                                    </pre>
                                  </div>
                                </div>
                                {log.status === "failed" && (
                                  <div className="mt-4">
                                    <Button size="sm">
                                      <RefreshCw className="h-4 w-4 mr-2" />
                                      Retry Delivery
                                    </Button>
                                  </div>
                                )}
                              </TableCell>
                            </TableRow>
                          </CollapsibleContent>
                        </>
                      </Collapsible>
                    ))}
                  </TableBody>
                </Table>
              </ScrollArea>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

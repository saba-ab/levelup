import { useState } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { Key, Plus, Copy, Eye, EyeOff, Trash2, RefreshCw, Webhook, Send, CheckCircle2, XCircle, Clock, Code, Terminal, BookOpen } from "lucide-react";
import { toast } from "@/hooks/use-toast";

const apiKeys = [
  { id: "1", name: "Production API Key", key: "pk_live_xxxxxxxxxxxxxxxxxxxx", created: "2024-01-15", lastUsed: "2024-01-20", status: "active" },
  { id: "2", name: "Development API Key", key: "pk_test_xxxxxxxxxxxxxxxxxxxx", created: "2024-01-10", lastUsed: "2024-01-19", status: "active" },
  { id: "3", name: "Mobile App Key", key: "pk_live_yyyyyyyyyyyyyyyyyyyy", created: "2024-01-05", lastUsed: "2024-01-18", status: "active" },
];

const webhooks = [
  { id: "1", url: "https://api.example.com/webhooks/gamify", events: ["points.earned", "badge.unlocked"], status: "active", lastTriggered: "2024-01-20 14:30", successRate: 98.5 },
  { id: "2", url: "https://hooks.slack.com/services/xxx", events: ["level.up", "mission.completed"], status: "active", lastTriggered: "2024-01-20 12:15", successRate: 100 },
  { id: "3", url: "https://webhook.site/test", events: ["user.registered"], status: "inactive", lastTriggered: "2024-01-15 09:00", successRate: 85.2 },
];

const webhookEvents = [
  { category: "Points", events: ["points.earned", "points.redeemed", "points.expired"] },
  { category: "Badges", events: ["badge.unlocked", "badge.progress"] },
  { category: "Levels", events: ["level.up", "xp.earned"] },
  { category: "Missions", events: ["mission.started", "mission.completed", "mission.failed"] },
  { category: "Streaks", events: ["streak.started", "streak.milestone", "streak.broken"] },
  { category: "Rewards", events: ["reward.redeemed", "reward.shipped"] },
  { category: "Users", events: ["user.registered", "user.profile_updated"] },
];

const webhookLogs = [
  { id: "1", event: "points.earned", url: "https://api.example.com/webhooks/gamify", status: "success", timestamp: "2024-01-20 14:30:25", responseTime: 125 },
  { id: "2", event: "badge.unlocked", url: "https://api.example.com/webhooks/gamify", status: "success", timestamp: "2024-01-20 14:28:10", responseTime: 98 },
  { id: "3", event: "level.up", url: "https://hooks.slack.com/services/xxx", status: "success", timestamp: "2024-01-20 12:15:00", responseTime: 210 },
  { id: "4", event: "user.registered", url: "https://webhook.site/test", status: "failed", timestamp: "2024-01-20 10:00:00", responseTime: 5000 },
];

export default function Integrations() {
  const [showKey, setShowKey] = useState<Record<string, boolean>>({});
  const [isCreateKeyOpen, setIsCreateKeyOpen] = useState(false);
  const [isCreateWebhookOpen, setIsCreateWebhookOpen] = useState(false);
  const [isTestWebhookOpen, setIsTestWebhookOpen] = useState(false);
  const [selectedWebhook, setSelectedWebhook] = useState<typeof webhooks[0] | null>(null);
  const [testPayload, setTestPayload] = useState(JSON.stringify({ userId: "user_123", points: 100, reason: "purchase" }, null, 2));
  const [isTesting, setIsTesting] = useState(false);

  const toggleKeyVisibility = (id: string) => {
    setShowKey(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    toast({ title: "Copied to clipboard" });
  };

  const handleTestWebhook = () => {
    setIsTesting(true);
    setTimeout(() => {
      setIsTesting(false);
      toast({ title: "Webhook test successful", description: "Response received in 125ms" });
    }, 1500);
  };

  return (
    <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Integrations</h1>
          <p className="text-muted-foreground">Manage API keys, webhooks, and SDK configurations</p>
        </div>

        <Tabs defaultValue="api-keys" className="space-y-6">
          <TabsList className="grid w-full max-w-md grid-cols-3">
            <TabsTrigger value="api-keys" className="flex items-center gap-2">
              <Key className="h-4 w-4" />
              API Keys
            </TabsTrigger>
            <TabsTrigger value="webhooks" className="flex items-center gap-2">
              <Webhook className="h-4 w-4" />
              Webhooks
            </TabsTrigger>
            <TabsTrigger value="sdk" className="flex items-center gap-2">
              <Code className="h-4 w-4" />
              SDK
            </TabsTrigger>
          </TabsList>

          {/* API Keys Tab */}
          <TabsContent value="api-keys" className="space-y-6">
            <div className="flex justify-between items-center">
              <div>
                <h2 className="text-xl font-semibold">API Keys</h2>
                <p className="text-sm text-muted-foreground">Manage your API keys for authenticating requests</p>
              </div>
              <Dialog open={isCreateKeyOpen} onOpenChange={setIsCreateKeyOpen}>
                <DialogTrigger asChild>
                  <Button>
                    <Plus className="h-4 w-4 mr-2" />
                    Create API Key
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Create New API Key</DialogTitle>
                    <DialogDescription>Generate a new API key for your application</DialogDescription>
                  </DialogHeader>
                  <div className="space-y-4 py-4">
                    <div className="space-y-2">
                      <Label>Key Name</Label>
                      <Input placeholder="e.g., Production API Key" />
                    </div>
                    <div className="space-y-2">
                      <Label>Environment</Label>
                      <Select defaultValue="production">
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="production">Production</SelectItem>
                          <SelectItem value="development">Development</SelectItem>
                          <SelectItem value="staging">Staging</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label>Permissions</Label>
                      <div className="space-y-2">
                        {["Read", "Write", "Delete", "Admin"].map((perm) => (
                          <div key={perm} className="flex items-center justify-between">
                            <span className="text-sm">{perm}</span>
                            <Switch defaultChecked={perm === "Read"} />
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                  <DialogFooter>
                    <Button variant="outline" onClick={() => setIsCreateKeyOpen(false)}>Cancel</Button>
                    <Button onClick={() => {
                      setIsCreateKeyOpen(false);
                      toast({ title: "API Key created", description: "Your new API key has been generated" });
                    }}>Create Key</Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </div>

            <Card>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Name</TableHead>
                      <TableHead>API Key</TableHead>
                      <TableHead>Created</TableHead>
                      <TableHead>Last Used</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {apiKeys.map((key) => (
                      <TableRow key={key.id}>
                        <TableCell className="font-medium">{key.name}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <code className="text-sm bg-muted px-2 py-1 rounded font-mono">
                              {showKey[key.id] ? key.key : key.key.replace(/./g, "•").slice(0, 24) + "..."}
                            </code>
                            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => toggleKeyVisibility(key.id)}>
                              {showKey[key.id] ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                            </Button>
                            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => copyToClipboard(key.key)}>
                              <Copy className="h-4 w-4" />
                            </Button>
                          </div>
                        </TableCell>
                        <TableCell>{key.created}</TableCell>
                        <TableCell>{key.lastUsed}</TableCell>
                        <TableCell>
                          <Badge variant={key.status === "active" ? "default" : "secondary"}>
                            {key.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-2">
                            <Button variant="ghost" size="icon" className="h-8 w-8">
                              <RefreshCw className="h-4 w-4" />
                            </Button>
                            <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive">
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Rate Limits</CardTitle>
                <CardDescription>Current API usage and limits</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="grid gap-4 md:grid-cols-3">
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Requests (hourly)</span>
                      <span>8,432 / 10,000</span>
                    </div>
                    <div className="h-2 bg-muted rounded-full overflow-hidden">
                      <div className="h-full bg-primary rounded-full" style={{ width: "84.32%" }} />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Requests (daily)</span>
                      <span>156,789 / 500,000</span>
                    </div>
                    <div className="h-2 bg-muted rounded-full overflow-hidden">
                      <div className="h-full bg-primary rounded-full" style={{ width: "31.36%" }} />
                    </div>
                  </div>
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Requests (monthly)</span>
                      <span>2.1M / 10M</span>
                    </div>
                    <div className="h-2 bg-muted rounded-full overflow-hidden">
                      <div className="h-full bg-primary rounded-full" style={{ width: "21%" }} />
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          {/* Webhooks Tab */}
          <TabsContent value="webhooks" className="space-y-6">
            <div className="flex justify-between items-center">
              <div>
                <h2 className="text-xl font-semibold">Webhooks</h2>
                <p className="text-sm text-muted-foreground">Configure endpoints to receive real-time event notifications</p>
              </div>
              <Dialog open={isCreateWebhookOpen} onOpenChange={setIsCreateWebhookOpen}>
                <DialogTrigger asChild>
                  <Button>
                    <Plus className="h-4 w-4 mr-2" />
                    Add Webhook
                  </Button>
                </DialogTrigger>
                <DialogContent className="max-w-2xl">
                  <DialogHeader>
                    <DialogTitle>Create Webhook</DialogTitle>
                    <DialogDescription>Configure a new webhook endpoint</DialogDescription>
                  </DialogHeader>
                  <div className="space-y-4 py-4">
                    <div className="space-y-2">
                      <Label>Endpoint URL</Label>
                      <Input placeholder="https://your-domain.com/webhooks/gamify" />
                    </div>
                    <div className="space-y-2">
                      <Label>Secret Key (optional)</Label>
                      <Input placeholder="whsec_xxxxxxxxxxxxxxxx" type="password" />
                      <p className="text-xs text-muted-foreground">Used to sign webhook payloads for verification</p>
                    </div>
                    <div className="space-y-2">
                      <Label>Events to Subscribe</Label>
                      <div className="border rounded-lg p-4 max-h-60 overflow-y-auto space-y-4">
                        {webhookEvents.map((category) => (
                          <div key={category.category}>
                            <div className="flex items-center gap-2 mb-2">
                              <Switch id={category.category} />
                              <Label htmlFor={category.category} className="font-medium">{category.category}</Label>
                            </div>
                            <div className="ml-6 space-y-1">
                              {category.events.map((event) => (
                                <div key={event} className="flex items-center gap-2">
                                  <Switch id={event} />
                                  <Label htmlFor={event} className="text-sm font-normal text-muted-foreground">{event}</Label>
                                </div>
                              ))}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  </div>
                  <DialogFooter>
                    <Button variant="outline" onClick={() => setIsCreateWebhookOpen(false)}>Cancel</Button>
                    <Button onClick={() => {
                      setIsCreateWebhookOpen(false);
                      toast({ title: "Webhook created", description: "Your webhook endpoint has been configured" });
                    }}>Create Webhook</Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </div>

            <div className="grid gap-4">
              {webhooks.map((webhook) => (
                <Card key={webhook.id}>
                  <CardContent className="p-4">
                    <div className="flex items-start justify-between">
                      <div className="space-y-2">
                        <div className="flex items-center gap-2">
                          <code className="text-sm font-mono bg-muted px-2 py-1 rounded">{webhook.url}</code>
                          <Badge variant={webhook.status === "active" ? "default" : "secondary"}>
                            {webhook.status}
                          </Badge>
                        </div>
                        <div className="flex flex-wrap gap-1">
                          {webhook.events.map((event) => (
                            <Badge key={event} variant="outline" className="text-xs">{event}</Badge>
                          ))}
                        </div>
                        <div className="flex items-center gap-4 text-sm text-muted-foreground">
                          <span className="flex items-center gap-1">
                            <Clock className="h-3 w-3" />
                            Last triggered: {webhook.lastTriggered}
                          </span>
                          <span className="flex items-center gap-1">
                            {webhook.successRate >= 95 ? (
                              <CheckCircle2 className="h-3 w-3 text-green-500" />
                            ) : (
                              <XCircle className="h-3 w-3 text-destructive" />
                            )}
                            Success rate: {webhook.successRate}%
                          </span>
                        </div>
                      </div>
                      <div className="flex gap-2">
                        <Button variant="outline" size="sm" onClick={() => {
                          setSelectedWebhook(webhook);
                          setIsTestWebhookOpen(true);
                        }}>
                          <Send className="h-4 w-4 mr-2" />
                          Test
                        </Button>
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive">
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>

            {/* Test Webhook Dialog */}
            <Dialog open={isTestWebhookOpen} onOpenChange={setIsTestWebhookOpen}>
              <DialogContent className="max-w-2xl">
                <DialogHeader>
                  <DialogTitle>Test Webhook</DialogTitle>
                  <DialogDescription>Send a test payload to {selectedWebhook?.url}</DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <Label>Event Type</Label>
                    <Select defaultValue="points.earned">
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {webhookEvents.flatMap(cat => cat.events).map((event) => (
                          <SelectItem key={event} value={event}>{event}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>Payload</Label>
                    <Textarea
                      value={testPayload}
                      onChange={(e) => setTestPayload(e.target.value)}
                      className="font-mono text-sm h-40"
                    />
                  </div>
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setIsTestWebhookOpen(false)}>Cancel</Button>
                  <Button onClick={handleTestWebhook} disabled={isTesting}>
                    {isTesting ? (
                      <>
                        <RefreshCw className="h-4 w-4 mr-2 animate-spin" />
                        Sending...
                      </>
                    ) : (
                      <>
                        <Send className="h-4 w-4 mr-2" />
                        Send Test
                      </>
                    )}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

            {/* Webhook Logs */}
            <Card>
              <CardHeader>
                <CardTitle>Recent Deliveries</CardTitle>
                <CardDescription>Last 24 hours of webhook activity</CardDescription>
              </CardHeader>
              <CardContent className="p-0">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Status</TableHead>
                      <TableHead>Event</TableHead>
                      <TableHead>Endpoint</TableHead>
                      <TableHead>Response Time</TableHead>
                      <TableHead>Timestamp</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {webhookLogs.map((log) => (
                      <TableRow key={log.id}>
                        <TableCell>
                          {log.status === "success" ? (
                            <CheckCircle2 className="h-4 w-4 text-green-500" />
                          ) : (
                            <XCircle className="h-4 w-4 text-destructive" />
                          )}
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline">{log.event}</Badge>
                        </TableCell>
                        <TableCell className="font-mono text-sm max-w-[200px] truncate">{log.url}</TableCell>
                        <TableCell>{log.responseTime}ms</TableCell>
                        <TableCell className="text-muted-foreground">{log.timestamp}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </TabsContent>

          {/* SDK Tab */}
          <TabsContent value="sdk" className="space-y-6">
            <div>
              <h2 className="text-xl font-semibold">SDK Documentation</h2>
              <p className="text-sm text-muted-foreground">Get started with our SDKs and client libraries</p>
            </div>

            <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
              {[
                { name: "JavaScript", icon: "🟨", version: "v2.4.1", description: "For web applications and Node.js" },
                { name: "Python", icon: "🐍", version: "v1.8.0", description: "For backend services and scripts" },
                { name: "Ruby", icon: "💎", version: "v1.5.2", description: "For Rails and Ruby applications" },
                { name: "Go", icon: "🔵", version: "v1.2.0", description: "For high-performance services" },
                { name: "PHP", icon: "🐘", version: "v2.1.0", description: "For WordPress and PHP apps" },
                { name: "REST API", icon: "🌐", version: "v3.0", description: "Direct HTTP integration" },
              ].map((sdk) => (
                <Card key={sdk.name} className="cursor-pointer hover:border-primary transition-colors">
                  <CardHeader>
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <span className="text-2xl">{sdk.icon}</span>
                        <div>
                          <CardTitle className="text-lg">{sdk.name}</CardTitle>
                          <Badge variant="secondary" className="mt-1">{sdk.version}</Badge>
                        </div>
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <p className="text-sm text-muted-foreground">{sdk.description}</p>
                  </CardContent>
                </Card>
              ))}
            </div>

            <Card>
              <CardHeader>
                <div className="flex items-center gap-2">
                  <Terminal className="h-5 w-5" />
                  <CardTitle>Quick Start</CardTitle>
                </div>
                <CardDescription>Get up and running in minutes</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label>1. Install the SDK</Label>
                    <Button variant="ghost" size="sm" onClick={() => copyToClipboard("npm install @gamify/sdk")}>
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                  <pre className="bg-muted p-4 rounded-lg overflow-x-auto">
                    <code className="text-sm">npm install @gamify/sdk</code>
                  </pre>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label>2. Initialize the client</Label>
                    <Button variant="ghost" size="sm" onClick={() => copyToClipboard(`import { GamifyClient } from '@gamify/sdk';

const client = new GamifyClient({
  apiKey: 'your_api_key',
  environment: 'production'
});`)}>
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                  <pre className="bg-muted p-4 rounded-lg overflow-x-auto">
                    <code className="text-sm">{`import { GamifyClient } from '@gamify/sdk';

const client = new GamifyClient({
  apiKey: 'your_api_key',
  environment: 'production'
});`}</code>
                  </pre>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label>3. Award points to a user</Label>
                    <Button variant="ghost" size="sm" onClick={() => copyToClipboard(`await client.points.award({
  userId: 'user_123',
  amount: 100,
  reason: 'completed_purchase',
  metadata: {
    orderId: 'order_456'
  }
});`)}>
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                  <pre className="bg-muted p-4 rounded-lg overflow-x-auto">
                    <code className="text-sm">{`await client.points.award({
  userId: 'user_123',
  amount: 100,
  reason: 'completed_purchase',
  metadata: {
    orderId: 'order_456'
  }
});`}</code>
                  </pre>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label>4. Check user progress</Label>
                    <Button variant="ghost" size="sm" onClick={() => copyToClipboard(`const user = await client.users.get('user_123');
console.log(user.points);    // 1250
console.log(user.level);     // 5
console.log(user.badges);    // ['early_adopter', 'power_user']`)}>
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                  <pre className="bg-muted p-4 rounded-lg overflow-x-auto">
                    <code className="text-sm">{`const user = await client.users.get('user_123');
console.log(user.points);    // 1250
console.log(user.level);     // 5
console.log(user.badges);    // ['early_adopter', 'power_user']`}</code>
                  </pre>
                </div>
              </CardContent>
            </Card>

            <div className="grid gap-4 md:grid-cols-2">
              <Card>
                <CardHeader>
                  <div className="flex items-center gap-2">
                    <BookOpen className="h-5 w-5" />
                    <CardTitle>Documentation</CardTitle>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <Button variant="outline" className="w-full justify-start">API Reference</Button>
                  <Button variant="outline" className="w-full justify-start">Authentication Guide</Button>
                  <Button variant="outline" className="w-full justify-start">Error Handling</Button>
                  <Button variant="outline" className="w-full justify-start">Rate Limiting</Button>
                  <Button variant="outline" className="w-full justify-start">Webhooks Guide</Button>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <div className="flex items-center gap-2">
                    <Code className="h-5 w-5" />
                    <CardTitle>Code Examples</CardTitle>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <Button variant="outline" className="w-full justify-start">Points & Leaderboards</Button>
                  <Button variant="outline" className="w-full justify-start">Badges & Achievements</Button>
                  <Button variant="outline" className="w-full justify-start">Missions & Quests</Button>
                  <Button variant="outline" className="w-full justify-start">User Profiles</Button>
                  <Button variant="outline" className="w-full justify-start">Analytics Integration</Button>
                </CardContent>
              </Card>
            </div>
          </TabsContent>
      </Tabs>
    </div>
  );
}

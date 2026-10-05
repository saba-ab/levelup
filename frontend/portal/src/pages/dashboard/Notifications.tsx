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
import { ScrollArea } from "@/components/ui/scroll-area";
import { FileText, Mail, Bell, MessageSquare, Smartphone, Plus, Edit, Trash2, Send, CheckCircle2, XCircle, Clock, Eye, Copy } from "lucide-react";
import { toast } from "@/hooks/use-toast";
import FeatureUnavailable from '@/components/FeatureUnavailable';

const templates = [
  { id: "1", name: "Welcome Message", trigger: "user.registered", channels: ["email", "push"], subject: "Welcome to Gamify!", status: "active", lastEdited: "2024-01-20" },
  { id: "2", name: "Points Earned", trigger: "points.earned", channels: ["push", "in-app"], subject: "You earned {{points}} points!", status: "active", lastEdited: "2024-01-19" },
  { id: "3", name: "Badge Unlocked", trigger: "badge.unlocked", channels: ["email", "push", "in-app"], subject: "New badge unlocked: {{badge_name}}", status: "active", lastEdited: "2024-01-18" },
  { id: "4", name: "Level Up", trigger: "level.up", channels: ["push", "in-app"], subject: "Congratulations! You reached Level {{level}}", status: "active", lastEdited: "2024-01-17" },
  { id: "5", name: "Streak Reminder", trigger: "streak.at_risk", channels: ["push"], subject: "Keep your streak alive!", status: "active", lastEdited: "2024-01-16" },
  { id: "6", name: "Reward Available", trigger: "reward.available", channels: ["email", "push"], subject: "New reward available for you!", status: "inactive", lastEdited: "2024-01-15" },
];

const channels = [
  { id: "email", name: "Email", icon: Mail, enabled: true, provider: "Resend", dailyLimit: 10000, sent24h: 2456 },
  { id: "push", name: "Push Notifications", icon: Bell, enabled: true, provider: "Firebase FCM", dailyLimit: 50000, sent24h: 12890 },
  { id: "sms", name: "SMS", icon: Smartphone, enabled: false, provider: "Twilio", dailyLimit: 5000, sent24h: 0 },
  { id: "in-app", name: "In-App", icon: MessageSquare, enabled: true, provider: "Native", dailyLimit: null, sent24h: 8234 },
];

const sendHistory = [
  { id: "1", template: "Points Earned", recipient: "john@example.com", channel: "push", status: "delivered", timestamp: "2024-01-20 14:32:15", openedAt: "2024-01-20 14:35:00" },
  { id: "2", template: "Badge Unlocked", recipient: "jane@example.com", channel: "email", status: "delivered", timestamp: "2024-01-20 14:30:42", openedAt: null },
  { id: "3", template: "Level Up", recipient: "mike@example.com", channel: "push", status: "delivered", timestamp: "2024-01-20 14:28:10", openedAt: "2024-01-20 14:28:45" },
  { id: "4", template: "Welcome Message", recipient: "sarah@example.com", channel: "email", status: "failed", timestamp: "2024-01-20 14:25:00", openedAt: null, error: "Invalid email address" },
  { id: "5", template: "Streak Reminder", recipient: "chris@example.com", channel: "push", status: "delivered", timestamp: "2024-01-20 14:22:30", openedAt: null },
  { id: "6", template: "Points Earned", recipient: "alex@example.com", channel: "in-app", status: "pending", timestamp: "2024-01-20 14:20:00", openedAt: null },
];

const triggerOptions = [
  { value: "user.registered", label: "User Registered" },
  { value: "points.earned", label: "Points Earned" },
  { value: "points.redeemed", label: "Points Redeemed" },
  { value: "badge.unlocked", label: "Badge Unlocked" },
  { value: "level.up", label: "Level Up" },
  { value: "mission.completed", label: "Mission Completed" },
  { value: "streak.milestone", label: "Streak Milestone" },
  { value: "streak.at_risk", label: "Streak At Risk" },
  { value: "reward.available", label: "Reward Available" },
  { value: "reward.redeemed", label: "Reward Redeemed" },
];

const channelIcons: Record<string, React.ElementType> = {
  email: Mail,
  push: Bell,
  sms: Smartphone,
  "in-app": MessageSquare,
};

/** Planned layout only: this module has no Go API yet. Rendered inert below the banner. */
function NotificationsPreview() {
  const [isTemplateDialogOpen, setIsTemplateDialogOpen] = useState(false);
  const [selectedChannels, setSelectedChannels] = useState<string[]>(["email", "push"]);
  const [historyFilter, setHistoryFilter] = useState("all");

  const toggleChannel = (channelId: string) => {
    setSelectedChannels(prev => 
      prev.includes(channelId) 
        ? prev.filter(c => c !== channelId)
        : [...prev, channelId]
    );
  };

  const filteredHistory = sendHistory.filter(item => {
    if (historyFilter === "all") return true;
    return item.status === historyFilter;
  });

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Notifications</h1>
        <p className="text-muted-foreground">Manage templates, channels, and delivery history</p>
      </div>

      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Sent Today</CardDescription>
            <CardTitle className="text-2xl">23,580</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Delivery Rate</CardDescription>
            <CardTitle className="text-2xl text-green-500">98.5%</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Open Rate</CardDescription>
            <CardTitle className="text-2xl">42.3%</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Active Templates</CardDescription>
            <CardTitle className="text-2xl">{templates.filter(t => t.status === "active").length}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <Tabs defaultValue="templates" className="space-y-6">
        <TabsList className="grid w-full max-w-md grid-cols-3">
          <TabsTrigger value="templates" className="flex items-center gap-2">
            <FileText className="h-4 w-4" />
            Templates
          </TabsTrigger>
          <TabsTrigger value="channels" className="flex items-center gap-2">
            <Bell className="h-4 w-4" />
            Channels
          </TabsTrigger>
          <TabsTrigger value="history" className="flex items-center gap-2">
            <Clock className="h-4 w-4" />
            History
          </TabsTrigger>
        </TabsList>

        {/* Templates Tab */}
        <TabsContent value="templates" className="space-y-4">
          <div className="flex justify-between items-center">
            <div>
              <h2 className="text-xl font-semibold">Notification Templates</h2>
              <p className="text-sm text-muted-foreground">Create and manage notification templates for different events</p>
            </div>
            <Dialog open={isTemplateDialogOpen} onOpenChange={setIsTemplateDialogOpen}>
              <DialogTrigger asChild>
                <Button>
                  <Plus className="h-4 w-4 mr-2" />
                  Create Template
                </Button>
              </DialogTrigger>
              <DialogContent className="max-w-2xl">
                <DialogHeader>
                  <DialogTitle>Create Notification Template</DialogTitle>
                  <DialogDescription>Design a new notification template</DialogDescription>
                </DialogHeader>
                <ScrollArea className="max-h-[60vh]">
                  <div className="space-y-4 py-4 pr-4">
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label>Template Name</Label>
                        <Input placeholder="e.g., Welcome Message" />
                      </div>
                      <div className="space-y-2">
                        <Label>Trigger Event</Label>
                        <Select>
                          <SelectTrigger>
                            <SelectValue placeholder="Select trigger" />
                          </SelectTrigger>
                          <SelectContent>
                            {triggerOptions.map((trigger) => (
                              <SelectItem key={trigger.value} value={trigger.value}>{trigger.label}</SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    </div>

                    <div className="space-y-2">
                      <Label>Delivery Channels</Label>
                      <div className="flex flex-wrap gap-2">
                        {channels.map((channel) => (
                          <Button
                            key={channel.id}
                            type="button"
                            variant={selectedChannels.includes(channel.id) ? "default" : "outline"}
                            size="sm"
                            onClick={() => toggleChannel(channel.id)}
                            disabled={!channel.enabled}
                          >
                            <channel.icon className="h-4 w-4 mr-2" />
                            {channel.name}
                          </Button>
                        ))}
                      </div>
                    </div>

                    <div className="space-y-2">
                      <Label>Subject Line</Label>
                      <Input placeholder="e.g., Welcome to Gamify, {{user_name}}!" />
                      <p className="text-xs text-muted-foreground">Use {"{{variable}}"} for dynamic content</p>
                    </div>

                    <div className="space-y-2">
                      <Label>Email Body</Label>
                      <Textarea 
                        placeholder="Write your notification content here..."
                        className="min-h-[150px]"
                      />
                    </div>

                    <div className="space-y-2">
                      <Label>Push Notification Text</Label>
                      <Input placeholder="Short message for push notification" />
                      <p className="text-xs text-muted-foreground">Max 100 characters recommended</p>
                    </div>

                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label>Delay (optional)</Label>
                        <div className="flex gap-2">
                          <Input type="number" placeholder="0" className="w-20" />
                          <Select defaultValue="minutes">
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="minutes">Minutes</SelectItem>
                              <SelectItem value="hours">Hours</SelectItem>
                              <SelectItem value="days">Days</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                      <div className="space-y-2">
                        <Label>Priority</Label>
                        <Select defaultValue="normal">
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="low">Low</SelectItem>
                            <SelectItem value="normal">Normal</SelectItem>
                            <SelectItem value="high">High</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    </div>

                    <div className="space-y-2">
                      <Label>Available Variables</Label>
                      <div className="flex flex-wrap gap-2">
                        {["user_name", "user_email", "points", "badge_name", "level", "streak_days"].map((variable) => (
                          <Badge 
                            key={variable} 
                            variant="secondary" 
                            className="cursor-pointer"
                            onClick={() => {
                              navigator.clipboard.writeText(`{{${variable}}}`);
                              toast({ title: "Copied to clipboard" });
                            }}
                          >
                            <Copy className="h-3 w-3 mr-1" />
                            {`{{${variable}}}`}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  </div>
                </ScrollArea>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setIsTemplateDialogOpen(false)}>Cancel</Button>
                  <Button onClick={() => {
                    setIsTemplateDialogOpen(false);
                    toast({ title: "Template created", description: "Your notification template has been saved" });
                  }}>
                    Save Template
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </div>

          <Card>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Template</TableHead>
                    <TableHead>Trigger</TableHead>
                    <TableHead>Channels</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Last Edited</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {templates.map((template) => (
                    <TableRow key={template.id}>
                      <TableCell>
                        <div>
                          <p className="font-medium">{template.name}</p>
                          <p className="text-xs text-muted-foreground truncate max-w-[200px]">{template.subject}</p>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">{template.trigger}</Badge>
                      </TableCell>
                      <TableCell>
                        <div className="flex gap-1">
                          {template.channels.map((channel) => {
                            const Icon = channelIcons[channel];
                            return Icon ? <Icon key={channel} className="h-4 w-4 text-muted-foreground" /> : null;
                          })}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant={template.status === "active" ? "default" : "secondary"}>
                          {template.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-muted-foreground">{template.lastEdited}</TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-2">
                          <Button variant="ghost" size="icon" className="h-8 w-8">
                            <Eye className="h-4 w-4" />
                          </Button>
                          <Button variant="ghost" size="icon" className="h-8 w-8">
                            <Edit className="h-4 w-4" />
                          </Button>
                          <Button variant="ghost" size="icon" className="h-8 w-8">
                            <Send className="h-4 w-4" />
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
        </TabsContent>

        {/* Channels Tab */}
        <TabsContent value="channels" className="space-y-4">
          <div>
            <h2 className="text-xl font-semibold">Delivery Channels</h2>
            <p className="text-sm text-muted-foreground">Configure notification delivery channels and providers</p>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            {channels.map((channel) => (
              <Card key={channel.id}>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="p-2 rounded-lg bg-muted">
                        <channel.icon className="h-5 w-5" />
                      </div>
                      <div>
                        <CardTitle className="text-lg">{channel.name}</CardTitle>
                        <CardDescription>{channel.provider}</CardDescription>
                      </div>
                    </div>
                    <Switch checked={channel.enabled} />
                  </div>
                </CardHeader>
                <CardContent>
                  <div className="space-y-4">
                    <div className="grid grid-cols-2 gap-4 text-sm">
                      <div>
                        <p className="text-muted-foreground">Sent (24h)</p>
                        <p className="text-xl font-semibold">{channel.sent24h.toLocaleString()}</p>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Daily Limit</p>
                        <p className="text-xl font-semibold">{channel.dailyLimit?.toLocaleString() || "Unlimited"}</p>
                      </div>
                    </div>
                    {channel.dailyLimit && (
                      <div className="space-y-1">
                        <div className="flex justify-between text-xs text-muted-foreground">
                          <span>Usage</span>
                          <span>{((channel.sent24h / channel.dailyLimit) * 100).toFixed(1)}%</span>
                        </div>
                        <div className="h-2 bg-muted rounded-full overflow-hidden">
                          <div 
                            className="h-full bg-primary rounded-full" 
                            style={{ width: `${(channel.sent24h / channel.dailyLimit) * 100}%` }} 
                          />
                        </div>
                      </div>
                    )}
                    <Button variant="outline" size="sm" className="w-full">
                      Configure
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Channel Settings</CardTitle>
              <CardDescription>Global settings for all notification channels</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Quiet Hours</p>
                  <p className="text-sm text-muted-foreground">Do not send notifications during these hours</p>
                </div>
                <div className="flex items-center gap-2">
                  <Input type="time" defaultValue="22:00" className="w-32" />
                  <span>to</span>
                  <Input type="time" defaultValue="08:00" className="w-32" />
                  <Switch defaultChecked />
                </div>
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Frequency Capping</p>
                  <p className="text-sm text-muted-foreground">Maximum notifications per user per day</p>
                </div>
                <div className="flex items-center gap-2">
                  <Input type="number" defaultValue="10" className="w-20" />
                  <span className="text-sm text-muted-foreground">per day</span>
                </div>
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Batch Notifications</p>
                  <p className="text-sm text-muted-foreground">Group similar notifications together</p>
                </div>
                <Switch defaultChecked />
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* History Tab */}
        <TabsContent value="history" className="space-y-4">
          <div className="flex justify-between items-center">
            <div>
              <h2 className="text-xl font-semibold">Send History</h2>
              <p className="text-sm text-muted-foreground">Track notification delivery and engagement</p>
            </div>
            <div className="flex gap-2">
              <Select value={historyFilter} onValueChange={setHistoryFilter}>
                <SelectTrigger className="w-40">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Status</SelectItem>
                  <SelectItem value="delivered">Delivered</SelectItem>
                  <SelectItem value="pending">Pending</SelectItem>
                  <SelectItem value="failed">Failed</SelectItem>
                </SelectContent>
              </Select>
              <Input placeholder="Search recipient..." className="w-64" />
            </div>
          </div>

          <Card>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Status</TableHead>
                    <TableHead>Template</TableHead>
                    <TableHead>Recipient</TableHead>
                    <TableHead>Channel</TableHead>
                    <TableHead>Sent At</TableHead>
                    <TableHead>Opened At</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredHistory.map((item) => {
                    const ChannelIcon = channelIcons[item.channel];
                    return (
                      <TableRow key={item.id}>
                        <TableCell>
                          {item.status === "delivered" ? (
                            <CheckCircle2 className="h-5 w-5 text-green-500" />
                          ) : item.status === "failed" ? (
                            <XCircle className="h-5 w-5 text-destructive" />
                          ) : (
                            <Clock className="h-5 w-5 text-yellow-500" />
                          )}
                        </TableCell>
                        <TableCell className="font-medium">{item.template}</TableCell>
                        <TableCell className="font-mono text-sm">{item.recipient}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            {ChannelIcon && <ChannelIcon className="h-4 w-4 text-muted-foreground" />}
                            <span className="capitalize">{item.channel}</span>
                          </div>
                        </TableCell>
                        <TableCell className="text-muted-foreground">{item.timestamp}</TableCell>
                        <TableCell>
                          {item.openedAt ? (
                            <span className="text-green-600">{item.openedAt}</span>
                          ) : item.status === "failed" ? (
                            <span className="text-destructive text-sm">{item.error}</span>
                          ) : (
                            <span className="text-muted-foreground">-</span>
                          )}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

export default function Notifications() {
  return (
    <div className="animate-fade-in">
      <FeatureUnavailable feature="Notifications">
        <NotificationsPreview />
      </FeatureUnavailable>
    </div>
  );
}

import { useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Plus, Trash2, Users, Filter, Save, Play, X, GripVertical } from "lucide-react";
import { toast } from "@/hooks/use-toast";

interface Condition {
  id: string;
  field: string;
  operator: string;
  value: string;
}

interface ConditionGroup {
  id: string;
  logic: "AND" | "OR";
  conditions: Condition[];
}

const fieldOptions = [
  { value: "points_balance", label: "Points Balance", type: "number" },
  { value: "level", label: "Level", type: "number" },
  { value: "badges_count", label: "Badges Count", type: "number" },
  { value: "missions_completed", label: "Missions Completed", type: "number" },
  { value: "streak_days", label: "Current Streak (days)", type: "number" },
  { value: "total_purchases", label: "Total Purchases", type: "number" },
  { value: "last_active", label: "Last Active", type: "date" },
  { value: "signup_date", label: "Signup Date", type: "date" },
  { value: "country", label: "Country", type: "string" },
  { value: "tier", label: "Tier", type: "select", options: ["Bronze", "Silver", "Gold", "Platinum"] },
  { value: "has_badge", label: "Has Badge", type: "select", options: ["early_adopter", "power_user", "streak_master", "social_butterfly"] },
];

const operatorsByType: Record<string, { value: string; label: string }[]> = {
  number: [
    { value: "eq", label: "equals" },
    { value: "neq", label: "not equals" },
    { value: "gt", label: "greater than" },
    { value: "gte", label: "greater than or equal" },
    { value: "lt", label: "less than" },
    { value: "lte", label: "less than or equal" },
    { value: "between", label: "between" },
  ],
  string: [
    { value: "eq", label: "equals" },
    { value: "neq", label: "not equals" },
    { value: "contains", label: "contains" },
    { value: "starts_with", label: "starts with" },
    { value: "ends_with", label: "ends with" },
  ],
  date: [
    { value: "eq", label: "on" },
    { value: "before", label: "before" },
    { value: "after", label: "after" },
    { value: "within_last", label: "within last (days)" },
    { value: "not_within_last", label: "not within last (days)" },
  ],
  select: [
    { value: "eq", label: "is" },
    { value: "neq", label: "is not" },
  ],
};

const existingSegments = [
  { id: "1", name: "High Value Users", description: "Users with 1000+ points and level 5+", userCount: 1234, createdAt: "2024-01-15" },
  { id: "2", name: "At-Risk Users", description: "Users inactive for 7+ days", userCount: 567, createdAt: "2024-01-18" },
  { id: "3", name: "Power Users", description: "Top tier users with high engagement", userCount: 234, createdAt: "2024-01-20" },
  { id: "4", name: "New Users", description: "Signed up in last 30 days", userCount: 890, createdAt: "2024-01-22" },
];

const previewUsers = [
  { id: "1", name: "John Doe", email: "john@example.com", points: 1250, level: 5, tier: "Gold", lastActive: "2024-01-20" },
  { id: "2", name: "Jane Smith", email: "jane@example.com", points: 2100, level: 7, tier: "Platinum", lastActive: "2024-01-20" },
  { id: "3", name: "Mike Johnson", email: "mike@example.com", points: 1500, level: 6, tier: "Gold", lastActive: "2024-01-19" },
  { id: "4", name: "Sarah Wilson", email: "sarah@example.com", points: 1800, level: 6, tier: "Gold", lastActive: "2024-01-20" },
  { id: "5", name: "Chris Brown", email: "chris@example.com", points: 3200, level: 8, tier: "Platinum", lastActive: "2024-01-20" },
];

export default function Segments() {
  const [isBuilderOpen, setIsBuilderOpen] = useState(false);
  const [segmentName, setSegmentName] = useState("");
  const [segmentDescription, setSegmentDescription] = useState("");
  const [conditionGroups, setConditionGroups] = useState<ConditionGroup[]>([
    { id: "group_1", logic: "AND", conditions: [{ id: "cond_1", field: "", operator: "", value: "" }] }
  ]);
  const [showPreview, setShowPreview] = useState(false);
  const [estimatedCount, setEstimatedCount] = useState<number | null>(null);

  const generateId = () => Math.random().toString(36).substr(2, 9);

  const addConditionGroup = () => {
    setConditionGroups([...conditionGroups, {
      id: `group_${generateId()}`,
      logic: "AND",
      conditions: [{ id: `cond_${generateId()}`, field: "", operator: "", value: "" }]
    }]);
  };

  const removeConditionGroup = (groupId: string) => {
    if (conditionGroups.length > 1) {
      setConditionGroups(conditionGroups.filter(g => g.id !== groupId));
    }
  };

  const addCondition = (groupId: string) => {
    setConditionGroups(conditionGroups.map(group => {
      if (group.id === groupId) {
        return {
          ...group,
          conditions: [...group.conditions, { id: `cond_${generateId()}`, field: "", operator: "", value: "" }]
        };
      }
      return group;
    }));
  };

  const removeCondition = (groupId: string, conditionId: string) => {
    setConditionGroups(conditionGroups.map(group => {
      if (group.id === groupId && group.conditions.length > 1) {
        return {
          ...group,
          conditions: group.conditions.filter(c => c.id !== conditionId)
        };
      }
      return group;
    }));
  };

  const updateCondition = (groupId: string, conditionId: string, field: keyof Condition, value: string) => {
    setConditionGroups(conditionGroups.map(group => {
      if (group.id === groupId) {
        return {
          ...group,
          conditions: group.conditions.map(cond => {
            if (cond.id === conditionId) {
              if (field === "field") {
                return { ...cond, [field]: value, operator: "", value: "" };
              }
              return { ...cond, [field]: value };
            }
            return cond;
          })
        };
      }
      return group;
    }));
  };

  const updateGroupLogic = (groupId: string, logic: "AND" | "OR") => {
    setConditionGroups(conditionGroups.map(group => {
      if (group.id === groupId) {
        return { ...group, logic };
      }
      return group;
    }));
  };

  const getFieldType = (fieldValue: string) => {
    const field = fieldOptions.find(f => f.value === fieldValue);
    return field?.type || "string";
  };

  const getFieldOptions = (fieldValue: string) => {
    const field = fieldOptions.find(f => f.value === fieldValue);
    return field?.options || [];
  };

  const runPreview = () => {
    setShowPreview(true);
    setEstimatedCount(previewUsers.length);
  };

  const saveSegment = () => {
    if (!segmentName) {
      toast({ title: "Error", description: "Please enter a segment name", variant: "destructive" });
      return;
    }
    toast({ title: "Segment saved", description: `"${segmentName}" has been created successfully` });
    setIsBuilderOpen(false);
    resetBuilder();
  };

  const resetBuilder = () => {
    setSegmentName("");
    setSegmentDescription("");
    setConditionGroups([{ id: "group_1", logic: "AND", conditions: [{ id: "cond_1", field: "", operator: "", value: "" }] }]);
    setShowPreview(false);
    setEstimatedCount(null);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Segments</h1>
          <p className="text-muted-foreground">Create and manage user segments for targeted campaigns</p>
        </div>
        <Dialog open={isBuilderOpen} onOpenChange={(open) => { setIsBuilderOpen(open); if (!open) resetBuilder(); }}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Create Segment
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-4xl max-h-[90vh] overflow-hidden flex flex-col">
            <DialogHeader>
              <DialogTitle>Segment Builder</DialogTitle>
              <DialogDescription>Define conditions to create a user segment</DialogDescription>
            </DialogHeader>
            
            <ScrollArea className="flex-1 pr-4">
              <div className="space-y-6 py-4">
                {/* Segment Info */}
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Segment Name</Label>
                    <Input 
                      placeholder="e.g., High Value Users" 
                      value={segmentName}
                      onChange={(e) => setSegmentName(e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>Description (optional)</Label>
                    <Input 
                      placeholder="Brief description of this segment" 
                      value={segmentDescription}
                      onChange={(e) => setSegmentDescription(e.target.value)}
                    />
                  </div>
                </div>

                <Separator />

                {/* Condition Groups */}
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <Label className="text-base font-semibold">Conditions</Label>
                    <Button variant="outline" size="sm" onClick={addConditionGroup}>
                      <Plus className="h-4 w-4 mr-2" />
                      Add Group (OR)
                    </Button>
                  </div>

                  {conditionGroups.map((group, groupIndex) => (
                    <Card key={group.id} className="relative">
                      {groupIndex > 0 && (
                        <div className="absolute -top-3 left-1/2 -translate-x-1/2 bg-background px-2">
                          <Badge variant="secondary">OR</Badge>
                        </div>
                      )}
                      <CardContent className="pt-6 space-y-4">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <GripVertical className="h-4 w-4 text-muted-foreground" />
                            <span className="text-sm font-medium">Group {groupIndex + 1}</span>
                            <Select value={group.logic} onValueChange={(v) => updateGroupLogic(group.id, v as "AND" | "OR")}>
                              <SelectTrigger className="w-24 h-8">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="AND">Match ALL</SelectItem>
                                <SelectItem value="OR">Match ANY</SelectItem>
                              </SelectContent>
                            </Select>
                          </div>
                          {conditionGroups.length > 1 && (
                            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => removeConditionGroup(group.id)}>
                              <Trash2 className="h-4 w-4 text-destructive" />
                            </Button>
                          )}
                        </div>

                        {group.conditions.map((condition, condIndex) => (
                          <div key={condition.id} className="flex items-center gap-2">
                            {condIndex > 0 && (
                              <Badge variant="outline" className="shrink-0">{group.logic}</Badge>
                            )}
                            <div className="flex-1 grid gap-2 md:grid-cols-3">
                              <Select 
                                value={condition.field} 
                                onValueChange={(v) => updateCondition(group.id, condition.id, "field", v)}
                              >
                                <SelectTrigger>
                                  <SelectValue placeholder="Select field" />
                                </SelectTrigger>
                                <SelectContent>
                                  {fieldOptions.map((field) => (
                                    <SelectItem key={field.value} value={field.value}>{field.label}</SelectItem>
                                  ))
                                  }
                                </SelectContent>
                              </Select>

                              <Select 
                                value={condition.operator} 
                                onValueChange={(v) => updateCondition(group.id, condition.id, "operator", v)}
                                disabled={!condition.field}
                              >
                                <SelectTrigger>
                                  <SelectValue placeholder="Select operator" />
                                </SelectTrigger>
                                <SelectContent>
                                  {operatorsByType[getFieldType(condition.field)]?.map((op) => (
                                    <SelectItem key={op.value} value={op.value}>{op.label}</SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>

                              {getFieldType(condition.field) === "select" ? (
                                <Select 
                                  value={condition.value} 
                                  onValueChange={(v) => updateCondition(group.id, condition.id, "value", v)}
                                  disabled={!condition.operator}
                                >
                                  <SelectTrigger>
                                    <SelectValue placeholder="Select value" />
                                  </SelectTrigger>
                                  <SelectContent>
                                    {getFieldOptions(condition.field).map((opt) => (
                                      <SelectItem key={opt} value={opt}>{opt}</SelectItem>
                                    ))}
                                  </SelectContent>
                                </Select>
                              ) : (
                                <Input 
                                  placeholder="Enter value" 
                                  value={condition.value}
                                  onChange={(e) => updateCondition(group.id, condition.id, "value", e.target.value)}
                                  disabled={!condition.operator}
                                  type={getFieldType(condition.field) === "number" ? "number" : "text"}
                                />
                              )}
                            </div>
                            {group.conditions.length > 1 && (
                              <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0" onClick={() => removeCondition(group.id, condition.id)}>
                                <X className="h-4 w-4" />
                              </Button>
                            )}
                          </div>
                        ))}

                        <Button variant="outline" size="sm" onClick={() => addCondition(group.id)}>
                          <Plus className="h-4 w-4 mr-2" />
                          Add Condition
                        </Button>
                      </CardContent>
                    </Card>
                  ))}
                </div>

                <Separator />

                {/* Preview Section */}
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <Label className="text-base font-semibold">Preview</Label>
                      <p className="text-sm text-muted-foreground">See which users match your conditions</p>
                    </div>
                    <Button variant="outline" onClick={runPreview}>
                      <Play className="h-4 w-4 mr-2" />
                      Run Preview
                    </Button>
                  </div>

                  {showPreview && (
                    <Card>
                      <CardHeader className="pb-3">
                        <div className="flex items-center justify-between">
                          <CardTitle className="text-lg flex items-center gap-2">
                            <Users className="h-5 w-5" />
                            {estimatedCount} users match
                          </CardTitle>
                          <Badge variant="secondary">Sample of 5 users</Badge>
                        </div>
                      </CardHeader>
                      <CardContent className="p-0">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead>User</TableHead>
                              <TableHead>Points</TableHead>
                              <TableHead>Level</TableHead>
                              <TableHead>Tier</TableHead>
                              <TableHead>Last Active</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {previewUsers.map((user) => (
                              <TableRow key={user.id}>
                                <TableCell>
                                  <div className="flex items-center gap-3">
                                    <Avatar className="h-8 w-8">
                                      <AvatarFallback className="text-xs">{user.name.split(" ").map(n => n[0]).join("")}</AvatarFallback>
                                    </Avatar>
                                    <div>
                                      <p className="font-medium">{user.name}</p>
                                      <p className="text-xs text-muted-foreground">{user.email}</p>
                                    </div>
                                  </div>
                                </TableCell>
                                <TableCell>{user.points.toLocaleString()}</TableCell>
                                <TableCell>{user.level}</TableCell>
                                <TableCell><Badge variant="outline">{user.tier}</Badge></TableCell>
                                <TableCell className="text-muted-foreground">{user.lastActive}</TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </CardContent>
                    </Card>
                  )}
                </div>
              </div>
            </ScrollArea>

            <DialogFooter className="mt-4">
              <Button variant="outline" onClick={() => setIsBuilderOpen(false)}>Cancel</Button>
              <Button onClick={saveSegment}>
                <Save className="h-4 w-4 mr-2" />
                Save Segment
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {/* Stats */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Total Segments</CardDescription>
            <CardTitle className="text-2xl">{existingSegments.length}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Total Users Segmented</CardDescription>
            <CardTitle className="text-2xl">2,925</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Avg. Segment Size</CardDescription>
            <CardTitle className="text-2xl">731</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Active Campaigns</CardDescription>
            <CardTitle className="text-2xl">8</CardTitle>
          </CardHeader>
        </Card>
      </div>

      {/* Existing Segments */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Your Segments</CardTitle>
              <CardDescription>Manage and edit your user segments</CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <div className="relative">
                <Filter className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input placeholder="Search segments..." className="pl-10 w-64" />
              </div>
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Description</TableHead>
                <TableHead>Users</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {existingSegments.map((segment) => (
                <TableRow key={segment.id}>
                  <TableCell className="font-medium">{segment.name}</TableCell>
                  <TableCell className="text-muted-foreground max-w-[300px] truncate">{segment.description}</TableCell>
                  <TableCell>
                    <Badge variant="secondary" className="flex items-center gap-1 w-fit">
                      <Users className="h-3 w-3" />
                      {segment.userCount.toLocaleString()}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{segment.createdAt}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button variant="ghost" size="sm">Edit</Button>
                      <Button variant="ghost" size="sm">Duplicate</Button>
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
    </div>
  );
}

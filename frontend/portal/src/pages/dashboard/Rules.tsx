import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { Plus, Search, MoreHorizontal, Play, Pause, Pencil, Archive, GitBranch, Zap, Clock, Copy } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { rules } from '@/lib/mockData';
import { cn } from '@/lib/utils';
import { useToast } from '@/hooks/use-toast';

export default function Rules() {
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [rulesList, setRulesList] = useState(rules);
  const [selectedRules, setSelectedRules] = useState<string[]>([]);
  const { toast } = useToast();

  const filteredRules = rulesList.filter(rule => {
    const matchesSearch = rule.name.toLowerCase().includes(searchQuery.toLowerCase());
    const matchesStatus = statusFilter === 'all' || rule.status === statusFilter;
    return matchesSearch && matchesStatus;
  });

  const toggleSelect = (id: string) => {
    setSelectedRules(prev =>
      prev.includes(id) ? prev.filter(i => i !== id) : [...prev, id]
    );
  };

  const toggleSelectAll = () => {
    if (selectedRules.length === filteredRules.length) {
      setSelectedRules([]);
    } else {
      setSelectedRules(filteredRules.map(r => r.id));
    }
  };

  const bulkAction = (action: 'publish' | 'archive') => {
    setRulesList(rules =>
      rules.map(r =>
        selectedRules.includes(r.id)
          ? { ...r, status: action === 'publish' ? 'active' : 'archived' }
          : r
      )
    );
    toast({
      title: `Rules ${action === 'publish' ? 'published' : 'archived'}`,
      description: `${selectedRules.length} rules have been ${action === 'publish' ? 'published' : 'archived'}.`,
    });
    setSelectedRules([]);
  };

  const getTriggerIcon = (trigger: string) => {
    switch (trigger) {
      case 'purchase_completed':
        return '🛒';
      case 'user_signup':
        return '👋';
      case 'user_login':
        return '🔐';
      case 'referral_completed':
        return '🔗';
      case 'subscription_upgraded':
        return '⬆️';
      default:
        return '⚡';
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Rules Engine</h1>
          <p className="text-muted-foreground mt-1">Create and manage your gamification rules.</p>
        </div>
        <Link to="/rules/new">
          <Button variant="glow">
            <Plus className="w-4 h-4" />
            Create Rule
          </Button>
        </Link>
      </div>

      {/* Search and Filter */}
      <Card>
        <CardContent className="p-4">
          <div className="flex flex-col sm:flex-row gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder="Search rules..."
                className="pl-9"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-full sm:w-[180px]">
                <SelectValue placeholder="Filter by status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Status</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="draft">Draft</SelectItem>
                <SelectItem value="archived">Archived</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Bulk Actions */}
      {selectedRules.length > 0 && (
        <Card className="bg-primary/5 border-primary/20">
          <CardContent className="p-4 flex items-center justify-between">
            <span className="text-sm font-medium">
              {selectedRules.length} rule{selectedRules.length > 1 ? 's' : ''} selected
            </span>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => bulkAction('publish')}>
                <Play className="w-4 h-4 mr-1" />
                Publish
              </Button>
              <Button variant="outline" size="sm" onClick={() => bulkAction('archive')}>
                <Archive className="w-4 h-4 mr-1" />
                Archive
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Rules Table */}
      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="p-4 w-12">
                    <Checkbox
                      checked={selectedRules.length === filteredRules.length && filteredRules.length > 0}
                      onCheckedChange={toggleSelectAll}
                    />
                  </th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Rule Name</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Trigger Event</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Version</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Last Modified</th>
                  <th className="text-right p-4 text-sm font-medium text-muted-foreground">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredRules.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="p-8 text-center">
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
                  filteredRules.map((rule) => (
                    <tr key={rule.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors">
                      <td className="p-4">
                        <Checkbox
                          checked={selectedRules.includes(rule.id)}
                          onCheckedChange={() => toggleSelect(rule.id)}
                        />
                      </td>
                      <td className="p-4">
                        <div className="flex items-center gap-3">
                          <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
                            <GitBranch className="w-5 h-5 text-primary" />
                          </div>
                          <span className="font-medium">{rule.name}</span>
                        </div>
                      </td>
                      <td className="p-4">
                        <div className="flex items-center gap-2">
                          <span className="text-lg">{getTriggerIcon(rule.trigger)}</span>
                          <code className="text-sm bg-secondary px-2 py-1 rounded">
                            {rule.trigger}
                          </code>
                        </div>
                      </td>
                      <td className="p-4">
                        <Badge
                          variant="outline"
                          className={cn(
                            "capitalize",
                            rule.status === 'active'
                              ? "border-green-500/50 text-green-500 bg-green-500/10"
                              : rule.status === 'draft'
                              ? "border-amber-500/50 text-amber-500 bg-amber-500/10"
                              : "border-muted-foreground/50 text-muted-foreground bg-muted"
                          )}
                        >
                          {rule.status}
                        </Badge>
                      </td>
                      <td className="p-4">
                        <span className="text-muted-foreground">v{rule.version}</span>
                      </td>
                      <td className="p-4">
                        <div className="flex items-center gap-2 text-muted-foreground">
                          <Clock className="w-4 h-4" />
                          <span className="text-sm">{rule.modifiedAt}</span>
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
                            <DropdownMenuItem>
                              <Zap className="w-4 h-4 mr-2" />
                              Test Rule
                            </DropdownMenuItem>
                            <DropdownMenuItem>
                              {rule.status === 'active' ? (
                                <>
                                  <Pause className="w-4 h-4 mr-2" />
                                  Pause
                                </>
                              ) : (
                                <>
                                  <Play className="w-4 h-4 mr-2" />
                                  Publish
                                </>
                              )}
                            </DropdownMenuItem>
                            <DropdownMenuItem>
                              <Pencil className="w-4 h-4 mr-2" />
                              Edit
                            </DropdownMenuItem>
                            <DropdownMenuItem>
                              <Copy className="w-4 h-4 mr-2" />
                              Duplicate
                            </DropdownMenuItem>
                            <DropdownMenuItem className="text-destructive">
                              <Archive className="w-4 h-4 mr-2" />
                              Archive
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

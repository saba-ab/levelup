import React, { useState } from 'react';
import { Plus, Search, Award } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { badges } from '@/lib/mockData';
import { useToast } from '@/hooks/use-toast';

export default function Badges() {
  const [searchQuery, setSearchQuery] = useState('');
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const { toast } = useToast();

  const filteredBadges = badges.filter(badge =>
    badge.name.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handleCreate = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    toast({
      title: 'Badge created',
      description: 'New badge has been created successfully.',
    });
    setIsDialogOpen(false);
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Badges</h1>
          <p className="text-muted-foreground mt-1">Create and manage achievement badges for your users.</p>
        </div>
        <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
          <DialogTrigger asChild>
            <Button variant="glow">
              <Plus className="w-4 h-4" />
              Create Badge
            </Button>
          </DialogTrigger>
          <DialogContent>
            <form onSubmit={handleCreate}>
              <DialogHeader>
                <DialogTitle>Create New Badge</DialogTitle>
                <DialogDescription>
                  Design a new achievement badge for your users.
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 py-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">Badge Name</label>
                  <Input placeholder="e.g., Super Achiever" required />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">Description</label>
                  <Input placeholder="What does the user need to do?" />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">Icon (emoji)</label>
                  <Input placeholder="e.g., 🏆" />
                </div>
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>
                  Cancel
                </Button>
                <Button type="submit" variant="glow">Create Badge</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      {/* Search */}
      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <Input
          placeholder="Search badges..."
          className="pl-9"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
        />
      </div>

      {/* Badges Grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
        {filteredBadges.map((badge, index) => (
          <Card
            key={badge.id}
            className="stat-card group cursor-pointer"
            style={{ animationDelay: `${index * 50}ms` }}
          >
            <CardContent className="p-6 text-center">
              <div
                className="w-20 h-20 rounded-full mx-auto mb-4 flex items-center justify-center text-4xl transition-transform group-hover:scale-110"
                style={{ backgroundColor: `${badge.color}20` }}
              >
                {badge.icon}
              </div>
              <h3 className="font-semibold text-lg mb-1">{badge.name}</h3>
              <p className="text-sm text-muted-foreground mb-3">{badge.description}</p>
              <Badge variant="outline" className="bg-secondary">
                <Award className="w-3 h-3 mr-1" />
                {badge.unlocks.toLocaleString()} unlocks
              </Badge>
            </CardContent>
          </Card>
        ))}
      </div>

      {filteredBadges.length === 0 && (
        <Card className="p-8 text-center">
          <div className="flex flex-col items-center gap-3">
            <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center">
              <Award className="w-8 h-8 text-muted-foreground" />
            </div>
            <div>
              <p className="font-medium">No badges found</p>
              <p className="text-sm text-muted-foreground">Create your first badge to reward your users.</p>
            </div>
          </div>
        </Card>
      )}
    </div>
  );
}

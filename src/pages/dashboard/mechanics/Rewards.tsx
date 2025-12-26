import React, { useState } from 'react';
import { Plus, Search, Gift, ShoppingCart, Package, Clock, MoreHorizontal, Pencil, Trash2 } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
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
import { rewards } from '@/lib/mockData';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';

const rewardsData = [
  { id: '1', name: 'Free Shipping', description: 'Free shipping on your next order', cost: 500, stock: 'unlimited', redemptions: 1234, image: '📦', category: 'shipping' },
  { id: '2', name: '10% Discount', description: '10% off your entire purchase', cost: 1000, stock: 'unlimited', redemptions: 890, image: '🏷️', category: 'discount' },
  { id: '3', name: 'Exclusive Merch', description: 'Limited edition branded merchandise', cost: 5000, stock: 50, redemptions: 23, image: '👕', category: 'physical' },
  { id: '4', name: 'VIP Access', description: 'Early access to new features', cost: 10000, stock: 10, redemptions: 5, image: '⭐', category: 'access' },
  { id: '5', name: 'Premium Upgrade', description: '1 month of premium subscription', cost: 7500, stock: 'unlimited', redemptions: 156, image: '💎', category: 'subscription' },
  { id: '6', name: 'Gift Card $25', description: '$25 store gift card', cost: 2500, stock: 100, redemptions: 67, image: '🎁', category: 'gift' },
];

const redemptionHistory = [
  { id: '1', userId: 'usr_001', reward: 'Free Shipping', cost: 500, timestamp: '2024-03-20 14:32:15', status: 'completed' },
  { id: '2', userId: 'usr_023', reward: '10% Discount', cost: 1000, timestamp: '2024-03-20 14:28:42', status: 'completed' },
  { id: '3', userId: 'usr_089', reward: 'Exclusive Merch', cost: 5000, timestamp: '2024-03-20 14:25:18', status: 'pending' },
  { id: '4', userId: 'usr_045', reward: 'Gift Card $25', cost: 2500, timestamp: '2024-03-20 14:18:33', status: 'completed' },
  { id: '5', userId: 'usr_112', reward: 'Free Shipping', cost: 500, timestamp: '2024-03-20 14:12:05', status: 'completed' },
  { id: '6', userId: 'usr_067', reward: 'VIP Access', cost: 10000, timestamp: '2024-03-20 14:05:41', status: 'completed' },
];

export default function Rewards() {
  const [searchQuery, setSearchQuery] = useState('');
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const { toast } = useToast();

  const filteredRewards = rewardsData.filter(reward =>
    reward.name.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handleCreate = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    toast({
      title: 'Reward created',
      description: 'New reward has been added to the catalog.',
    });
    setIsDialogOpen(false);
  };

  const totalRedemptions = rewardsData.reduce((sum, r) => sum + r.redemptions, 0);

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Rewards Catalog</h1>
          <p className="text-muted-foreground mt-1">Manage rewards and track redemptions.</p>
        </div>
        <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
          <DialogTrigger asChild>
            <Button variant="glow">
              <Plus className="w-4 h-4" />
              Add Reward
            </Button>
          </DialogTrigger>
          <DialogContent>
            <form onSubmit={handleCreate}>
              <DialogHeader>
                <DialogTitle>Add New Reward</DialogTitle>
                <DialogDescription>Create a new reward for your users to redeem.</DialogDescription>
              </DialogHeader>
              <div className="space-y-4 py-4">
                <div className="space-y-2">
                  <label className="text-sm font-medium">Reward Name</label>
                  <Input placeholder="e.g., Free Shipping" required />
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">Description</label>
                  <Input placeholder="What does the user get?" />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Cost (points)</label>
                    <Input type="number" placeholder="1000" required />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Stock</label>
                    <Select defaultValue="unlimited">
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="unlimited">Unlimited</SelectItem>
                        <SelectItem value="limited">Limited</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">Category</label>
                  <Select>
                    <SelectTrigger><SelectValue placeholder="Select category" /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="discount">Discount</SelectItem>
                      <SelectItem value="shipping">Shipping</SelectItem>
                      <SelectItem value="physical">Physical Item</SelectItem>
                      <SelectItem value="access">Access/Perks</SelectItem>
                      <SelectItem value="gift">Gift Card</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <label className="text-sm font-medium">Icon (emoji)</label>
                  <Input placeholder="e.g., 🎁" />
                </div>
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>Cancel</Button>
                <Button type="submit" variant="glow">Add Reward</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Card className="stat-card">
          <CardContent className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-purple-500/10 flex items-center justify-center">
                <Gift className="w-6 h-6 text-purple-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{rewardsData.length}</p>
                <p className="text-sm text-muted-foreground">Total Rewards</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="stat-card">
          <CardContent className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-green-500/10 flex items-center justify-center">
                <ShoppingCart className="w-6 h-6 text-green-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{totalRedemptions.toLocaleString()}</p>
                <p className="text-sm text-muted-foreground">Total Redemptions</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card className="stat-card">
          <CardContent className="p-6">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-xl bg-amber-500/10 flex items-center justify-center">
                <Package className="w-6 h-6 text-amber-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">160</p>
                <p className="text-sm text-muted-foreground">Limited Stock Items</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="catalog" className="space-y-6">
        <TabsList>
          <TabsTrigger value="catalog">Catalog</TabsTrigger>
          <TabsTrigger value="redemptions">Redemption History</TabsTrigger>
        </TabsList>

        <TabsContent value="catalog" className="space-y-4">
          {/* Search */}
          <div className="relative max-w-md">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
            <Input
              placeholder="Search rewards..."
              className="pl-9"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>

          {/* Rewards Grid */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {filteredRewards.map((reward, index) => (
              <Card key={reward.id} className="stat-card group" style={{ animationDelay: `${index * 50}ms` }}>
                <CardContent className="p-6">
                  <div className="flex items-start justify-between mb-4">
                    <div className="w-16 h-16 rounded-xl bg-secondary flex items-center justify-center text-3xl">
                      {reward.image}
                    </div>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="opacity-0 group-hover:opacity-100 transition-opacity">
                          <MoreHorizontal className="w-4 h-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem>
                          <Pencil className="w-4 h-4 mr-2" />
                          Edit
                        </DropdownMenuItem>
                        <DropdownMenuItem className="text-destructive">
                          <Trash2 className="w-4 h-4 mr-2" />
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                  <h3 className="font-semibold text-lg mb-1">{reward.name}</h3>
                  <p className="text-sm text-muted-foreground mb-4">{reward.description}</p>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Badge variant="secondary" className="font-mono">
                        {reward.cost.toLocaleString()} pts
                      </Badge>
                      {reward.stock !== 'unlimited' && (
                        <Badge variant="outline" className="border-amber-500/50 text-amber-500">
                          {reward.stock} left
                        </Badge>
                      )}
                    </div>
                    <span className="text-sm text-muted-foreground">
                      {reward.redemptions} redeemed
                    </span>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>

        <TabsContent value="redemptions">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Clock className="w-5 h-5" />
                Redemption History
              </CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-border">
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">User</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Reward</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Cost</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Timestamp</th>
                    </tr>
                  </thead>
                  <tbody>
                    {redemptionHistory.map((redemption) => (
                      <tr key={redemption.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors">
                        <td className="p-4">
                          <code className="text-sm bg-secondary px-2 py-1 rounded">{redemption.userId}</code>
                        </td>
                        <td className="p-4 font-medium">{redemption.reward}</td>
                        <td className="p-4">
                          <Badge variant="secondary" className="font-mono">
                            {redemption.cost.toLocaleString()} pts
                          </Badge>
                        </td>
                        <td className="p-4">
                          <Badge
                            variant="outline"
                            className={cn(
                              "capitalize",
                              redemption.status === 'completed'
                                ? "border-green-500/50 text-green-500 bg-green-500/10"
                                : "border-amber-500/50 text-amber-500 bg-amber-500/10"
                            )}
                          >
                            {redemption.status}
                          </Badge>
                        </td>
                        <td className="p-4 text-sm text-muted-foreground">{redemption.timestamp}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

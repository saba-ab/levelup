import React, { useState } from 'react';
import { Plus, Search, Coins, ArrowUpRight, ArrowDownRight, Wallet, Sparkles } from 'lucide-react';
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
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import { AIGenerateDialog } from '@/components/ai/AIGenerateDialog';
import { ItemActionsMenu } from '@/components/mechanics/ItemActionsMenu';

const initialCurrencies = [
  { id: '1', name: 'Experience Points', symbol: 'XP', totalIssued: 2450000, activeUsers: 8029, color: '#8B5CF6' },
  { id: '2', name: 'Loyalty Coins', symbol: 'LC', totalIssued: 890000, activeUsers: 4521, color: '#F59E0B' },
  { id: '3', name: 'Reward Stars', symbol: 'RS', totalIssued: 125000, activeUsers: 2340, color: '#10B981' },
];

const ledgerEntries = [
  { id: '1', userId: 'usr_001', currency: 'XP', amount: 150, type: 'credit', reason: 'First Purchase Bonus', timestamp: '2024-03-20 14:32:15', balance: 2450 },
  { id: '2', userId: 'usr_023', currency: 'XP', amount: 50, type: 'credit', reason: 'Daily Login', timestamp: '2024-03-20 14:28:42', balance: 1200 },
  { id: '3', userId: 'usr_089', currency: 'LC', amount: 100, type: 'debit', reason: 'Reward Redemption', timestamp: '2024-03-20 14:25:18', balance: 500 },
  { id: '4', userId: 'usr_045', currency: 'XP', amount: 500, type: 'credit', reason: 'Mission Completed', timestamp: '2024-03-20 14:18:33', balance: 8900 },
  { id: '5', userId: 'usr_112', currency: 'RS', amount: 25, type: 'credit', reason: 'Referral Bonus', timestamp: '2024-03-20 14:12:05', balance: 125 },
  { id: '6', userId: 'usr_067', currency: 'XP', amount: 200, type: 'credit', reason: 'Streak Milestone', timestamp: '2024-03-20 14:05:41', balance: 3400 },
  { id: '7', userId: 'usr_034', currency: 'LC', amount: 50, type: 'debit', reason: 'Store Purchase', timestamp: '2024-03-20 13:58:22', balance: 750 },
  { id: '8', userId: 'usr_098', currency: 'XP', amount: 75, type: 'credit', reason: 'Review Submitted', timestamp: '2024-03-20 13:45:18', balance: 1575 },
];

export default function Points() {
  const [searchQuery, setSearchQuery] = useState('');
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [currencies, setCurrencies] = useState(initialCurrencies);
  const [editingCurrency, setEditingCurrency] = useState<typeof initialCurrencies[0] | null>(null);
  const { toast } = useToast();

  const filteredEntries = ledgerEntries.filter(entry =>
    entry.userId.toLowerCase().includes(searchQuery.toLowerCase()) ||
    entry.reason.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handleCreate = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    toast({
      title: editingCurrency ? 'Currency updated' : 'Currency created',
      description: editingCurrency ? 'Currency has been updated successfully.' : 'New currency has been added successfully.',
    });
    setIsDialogOpen(false);
    setEditingCurrency(null);
  };

  const handleEdit = (currency: typeof initialCurrencies[0]) => {
    setEditingCurrency(currency);
    setIsDialogOpen(true);
  };

  const handleDelete = (id: string) => {
    setCurrencies(prev => prev.filter(c => c.id !== id));
    toast({
      title: 'Currency deleted',
      description: 'Currency has been removed successfully.',
    });
  };

  // Connect this to your MySQL backend
  const handleAIGenerate = async (prompt: string): Promise<string> => {
    await new Promise(resolve => setTimeout(resolve, 1500));
    return `Generated Currency Idea:\n\n"${prompt}"\n\nName: Achievement Tokens\nSymbol: AT\nDescription: Premium currency earned through exceptional achievements.\n\nUse cases:\n- Exclusive reward redemptions\n- VIP store access\n- Special event participation`;
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Points & Wallets</h1>
          <p className="text-muted-foreground mt-1">Manage currencies and view transaction ledger.</p>
        </div>
        <div className="flex gap-2">
          <AIGenerateDialog
            trigger={
              <Button variant="outline" className="gap-2">
                <Sparkles className="w-4 h-4" />
                Generate with AI
              </Button>
            }
            title="Generate Currency Ideas"
            placeholder="E.g., Create a premium currency for VIP users that feels exclusive..."
            context="Generate currency names, symbols, and use cases"
            onGenerate={handleAIGenerate}
          />
          <Dialog open={isDialogOpen} onOpenChange={(open) => { setIsDialogOpen(open); if (!open) setEditingCurrency(null); }}>
            <DialogTrigger asChild>
              <Button variant="glow">
                <Plus className="w-4 h-4" />
                Create Currency
              </Button>
            </DialogTrigger>
            <DialogContent>
              <form onSubmit={handleCreate}>
                <DialogHeader>
                  <DialogTitle>{editingCurrency ? 'Edit Currency' : 'Create New Currency'}</DialogTitle>
                  <DialogDescription>
                    {editingCurrency ? 'Update the currency details.' : 'Define a new point type for your gamification system.'}
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Currency Name</label>
                    <Input placeholder="e.g., Gold Coins" defaultValue={editingCurrency?.name} required />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Symbol</label>
                    <Input placeholder="e.g., GC" maxLength={4} defaultValue={editingCurrency?.symbol} required />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Description</label>
                    <Input placeholder="What is this currency used for?" />
                  </div>
                </div>
                <DialogFooter>
                  <Button type="button" variant="outline" onClick={() => { setIsDialogOpen(false); setEditingCurrency(null); }}>Cancel</Button>
                  <Button type="submit" variant="glow">{editingCurrency ? 'Save Changes' : 'Create Currency'}</Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        </div>
      </div>

      {/* Currency Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {currencies.map((currency, index) => (
          <Card key={currency.id} className="stat-card group" style={{ animationDelay: `${index * 100}ms` }}>
            <CardContent className="p-6">
              <div className="flex items-center justify-between mb-4">
                <div 
                  className="w-12 h-12 rounded-xl flex items-center justify-center"
                  style={{ backgroundColor: `${currency.color}20` }}
                >
                  <Coins className="w-6 h-6" style={{ color: currency.color }} />
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="outline" style={{ borderColor: currency.color, color: currency.color }}>
                    {currency.symbol}
                  </Badge>
                  <ItemActionsMenu
                    itemName={currency.name}
                    onEdit={() => handleEdit(currency)}
                    onDelete={() => handleDelete(currency.id)}
                    showInGroup
                  />
                </div>
              </div>
              <h3 className="font-semibold text-lg mb-1">{currency.name}</h3>
              <div className="grid grid-cols-2 gap-4 mt-4">
                <div>
                  <p className="text-2xl font-bold">{(currency.totalIssued / 1000000).toFixed(2)}M</p>
                  <p className="text-xs text-muted-foreground">Total Issued</p>
                </div>
                <div>
                  <p className="text-2xl font-bold">{currency.activeUsers.toLocaleString()}</p>
                  <p className="text-xs text-muted-foreground">Active Users</p>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Ledger */}
      <Card>
        <CardHeader>
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
            <CardTitle className="flex items-center gap-2">
              <Wallet className="w-5 h-5" />
              Transaction Ledger
            </CardTitle>
            <div className="relative w-full sm:w-64">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder="Search transactions..."
                className="pl-9"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">User</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Currency</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Amount</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Reason</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Balance</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Timestamp</th>
                </tr>
              </thead>
              <tbody>
                {filteredEntries.map((entry) => (
                  <tr key={entry.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors">
                    <td className="p-4">
                      <code className="text-sm bg-secondary px-2 py-1 rounded">{entry.userId}</code>
                    </td>
                    <td className="p-4">
                      <Badge variant="outline">{entry.currency}</Badge>
                    </td>
                    <td className="p-4">
                      <div className={cn(
                        "flex items-center gap-1 font-mono font-medium",
                        entry.type === 'credit' ? "text-green-500" : "text-red-500"
                      )}>
                        {entry.type === 'credit' ? (
                          <ArrowUpRight className="w-4 h-4" />
                        ) : (
                          <ArrowDownRight className="w-4 h-4" />
                        )}
                        {entry.type === 'credit' ? '+' : '-'}{entry.amount}
                      </div>
                    </td>
                    <td className="p-4 text-muted-foreground">{entry.reason}</td>
                    <td className="p-4 font-mono">{entry.balance.toLocaleString()}</td>
                    <td className="p-4 text-sm text-muted-foreground">{entry.timestamp}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

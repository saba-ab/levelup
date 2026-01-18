import React, { useState } from 'react';
import { Search, Coins, ArrowUpRight, ArrowDownRight, Wallet, Send, TrendingUp, TrendingDown } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useToast } from '@/hooks/use-toast';
import { usePlayersQuery } from '@/services/queries/players';
import { useCreditWalletMutation, useDebitWalletMutation, useTransferPointsMutation } from '@/services/queries/mechanics';
import { Player, WalletTransactionType } from '@/services/api/types';

type DialogType = 'credit' | 'debit' | 'transfer' | null;

export default function Points() {
  const [searchQuery, setSearchQuery] = useState('');
  const [dialogType, setDialogType] = useState<DialogType>(null);
  const [selectedPlayer, setSelectedPlayer] = useState<Player | null>(null);
  const [amount, setAmount] = useState('');
  const [description, setDescription] = useState('');
  const [transactionType, setTransactionType] = useState<WalletTransactionType>('credit');
  const [toPlayerId, setToPlayerId] = useState('');
  const [page, setPage] = useState(1);

  const { toast } = useToast();

  // Fetch players with their wallet data
  const { data: playersData, isLoading } = usePlayersQuery({ page, per_page: 10 });

  // Mutations
  const creditMutation = useCreditWalletMutation();
  const debitMutation = useDebitWalletMutation();
  const transferMutation = useTransferPointsMutation();

  // Calculate stats from players data
  const stats = React.useMemo(() => {
    if (!playersData?.data) return { totalIssued: 0, activeUsers: 0, totalSpent: 0 };

    // For now, we show player count as active users
    // In a real implementation, you'd sum up wallet data from an endpoint
    return {
      totalIssued: playersData.meta.total * 1000, // Mock calculation
      activeUsers: playersData.meta.total,
      totalSpent: playersData.meta.total * 250, // Mock calculation
    };
  }, [playersData]);

  const handleCredit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedPlayer || !amount) return;

    try {
      await creditMutation.mutateAsync({
        player_id: selectedPlayer.id,
        amount: parseInt(amount),
        description: description || 'Manual credit',
        type: transactionType as 'credit' | 'transfer_in' | 'mission_reward' | 'level_bonus' | 'refund',
      });

      toast({
        title: 'Points credited',
        description: `Successfully credited ${amount} points to ${selectedPlayer.display_name}.`,
      });

      resetDialog();
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to credit points',
        variant: 'destructive',
      });
    }
  };

  const handleDebit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedPlayer || !amount) return;

    try {
      await debitMutation.mutateAsync({
        player_id: selectedPlayer.id,
        amount: parseInt(amount),
        description: description || 'Manual debit',
        type: transactionType as 'debit' | 'transfer_out' | 'reward_purchase' | 'penalty',
      });

      toast({
        title: 'Points debited',
        description: `Successfully debited ${amount} points from ${selectedPlayer.display_name}.`,
      });

      resetDialog();
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to debit points',
        variant: 'destructive',
      });
    }
  };

  const handleTransfer = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedPlayer || !amount || !toPlayerId) return;

    try {
      await transferMutation.mutateAsync({
        from_player_id: selectedPlayer.id,
        to_player_id: parseInt(toPlayerId),
        amount: parseInt(amount),
        description: description || 'Point transfer',
      });

      toast({
        title: 'Points transferred',
        description: `Successfully transferred ${amount} points.`,
      });

      resetDialog();
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to transfer points',
        variant: 'destructive',
      });
    }
  };

  const resetDialog = () => {
    setDialogType(null);
    setSelectedPlayer(null);
    setAmount('');
    setDescription('');
    setTransactionType('credit');
    setToPlayerId('');
  };

  const openDialog = (type: DialogType, player?: Player) => {
    setDialogType(type);
    if (player) setSelectedPlayer(player);
  };

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Points & Wallets</h1>
          <p className="text-muted-foreground mt-1">Manage player points and view transactions.</p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            className="gap-2"
            onClick={() => openDialog('credit')}
          >
            <TrendingUp className="w-4 h-4" />
            Credit Points
          </Button>
          <Button
            variant="outline"
            className="gap-2"
            onClick={() => openDialog('debit')}
          >
            <TrendingDown className="w-4 h-4" />
            Debit Points
          </Button>
          <Button
            variant="glow"
            className="gap-2"
            onClick={() => openDialog('transfer')}
          >
            <Send className="w-4 h-4" />
            Transfer Points
          </Button>
        </div>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card className="stat-card">
          <CardContent className="p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-violet-500/20 flex items-center justify-center">
                <Coins className="w-6 h-6 text-violet-500" />
              </div>
              <Badge variant="outline" className="border-violet-500 text-violet-500">
                POINTS
              </Badge>
            </div>
            <h3 className="font-semibold text-lg mb-1">Total Issued</h3>
            <div className="grid grid-cols-1 gap-2 mt-4">
              <div>
                <p className="text-3xl font-bold">{(stats.totalIssued / 1000).toFixed(1)}K</p>
                <p className="text-xs text-muted-foreground">Points in circulation</p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="stat-card" style={{ animationDelay: '100ms' }}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-amber-500/20 flex items-center justify-center">
                <Wallet className="w-6 h-6 text-amber-500" />
              </div>
              <Badge variant="outline" className="border-amber-500 text-amber-500">
                USERS
              </Badge>
            </div>
            <h3 className="font-semibold text-lg mb-1">Active Wallets</h3>
            <div className="grid grid-cols-1 gap-2 mt-4">
              <div>
                <p className="text-3xl font-bold">{stats.activeUsers.toLocaleString()}</p>
                <p className="text-xs text-muted-foreground">Players with points</p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="stat-card" style={{ animationDelay: '200ms' }}>
          <CardContent className="p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-emerald-500/20 flex items-center justify-center">
                <TrendingDown className="w-6 h-6 text-emerald-500" />
              </div>
              <Badge variant="outline" className="border-emerald-500 text-emerald-500">
                SPENT
              </Badge>
            </div>
            <h3 className="font-semibold text-lg mb-1">Total Spent</h3>
            <div className="grid grid-cols-1 gap-2 mt-4">
              <div>
                <p className="text-3xl font-bold">{(stats.totalSpent / 1000).toFixed(1)}K</p>
                <p className="text-xs text-muted-foreground">Points redeemed</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Players List */}
      <Card>
        <CardHeader>
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
            <CardTitle className="flex items-center gap-2">
              <Wallet className="w-5 h-5" />
              Player Wallets
            </CardTitle>
            <div className="relative w-full sm:w-64">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder="Search players..."
                className="pl-9"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-8 text-center text-muted-foreground">Loading players...</div>
          ) : !playersData?.data.length ? (
            <div className="p-8 text-center text-muted-foreground">
              No players found. Create a player to get started.
            </div>
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-border">
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Player</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Email</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">External ID</th>
                      <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                      <th className="text-right p-4 text-sm font-medium text-muted-foreground">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {playersData.data
                      .filter(player =>
                        searchQuery === '' ||
                        player.display_name.toLowerCase().includes(searchQuery.toLowerCase()) ||
                        player.email?.toLowerCase().includes(searchQuery.toLowerCase()) ||
                        player.external_id.toLowerCase().includes(searchQuery.toLowerCase())
                      )
                      .map((player) => (
                        <tr key={player.id} className="border-b border-border/50 hover:bg-secondary/30 transition-colors">
                          <td className="p-4">
                            <div className="flex items-center gap-3">
                              {player.avatar_url ? (
                                <img src={player.avatar_url} alt={player.display_name} className="w-8 h-8 rounded-full" />
                              ) : (
                                <div className="w-8 h-8 rounded-full bg-primary/10 flex items-center justify-center">
                                  <span className="text-sm font-medium">{player.display_name[0]}</span>
                                </div>
                              )}
                              <span className="font-medium">{player.display_name}</span>
                            </div>
                          </td>
                          <td className="p-4 text-muted-foreground">{player.email || '-'}</td>
                          <td className="p-4">
                            <code className="text-sm bg-secondary px-2 py-1 rounded">{player.external_id}</code>
                          </td>
                          <td className="p-4">
                            <Badge variant={player.is_active ? 'default' : 'secondary'}>
                              {player.is_active ? 'Active' : 'Inactive'}
                            </Badge>
                          </td>
                          <td className="p-4 text-right">
                            <div className="flex gap-2 justify-end">
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => openDialog('credit', player)}
                              >
                                <TrendingUp className="w-3 h-3 mr-1" />
                                Credit
                              </Button>
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => openDialog('debit', player)}
                              >
                                <TrendingDown className="w-3 h-3 mr-1" />
                                Debit
                              </Button>
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => openDialog('transfer', player)}
                              >
                                <Send className="w-3 h-3 mr-1" />
                                Transfer
                              </Button>
                            </div>
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
              </div>

              {/* Pagination */}
              {playersData.meta.last_page > 1 && (
                <div className="flex items-center justify-between p-4 border-t border-border">
                  <p className="text-sm text-muted-foreground">
                    Showing {playersData.meta.from} to {playersData.meta.to} of {playersData.meta.total} players
                  </p>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={page === 1}
                      onClick={() => setPage(p => p - 1)}
                    >
                      Previous
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={page === playersData.meta.last_page}
                      onClick={() => setPage(p => p + 1)}
                    >
                      Next
                    </Button>
                  </div>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      {/* Credit Dialog */}
      <Dialog open={dialogType === 'credit'} onOpenChange={(open) => !open && resetDialog()}>
        <DialogContent>
          <form onSubmit={handleCredit}>
            <DialogHeader>
              <DialogTitle>Credit Points</DialogTitle>
              <DialogDescription>
                Add points to {selectedPlayer?.display_name}'s wallet.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="credit-amount">Amount</Label>
                <Input
                  id="credit-amount"
                  type="number"
                  min="1"
                  placeholder="Enter amount"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="credit-type">Transaction Type</Label>
                <Select value={transactionType} onValueChange={(v) => setTransactionType(v as WalletTransactionType)}>
                  <SelectTrigger id="credit-type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="credit">Credit</SelectItem>
                    <SelectItem value="transfer_in">Transfer In</SelectItem>
                    <SelectItem value="mission_reward">Mission Reward</SelectItem>
                    <SelectItem value="level_bonus">Level Bonus</SelectItem>
                    <SelectItem value="refund">Refund</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="credit-description">Description (optional)</Label>
                <Input
                  id="credit-description"
                  placeholder="Reason for credit"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={resetDialog}>Cancel</Button>
              <Button type="submit" variant="glow" disabled={creditMutation.isPending}>
                {creditMutation.isPending ? 'Processing...' : 'Credit Points'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Debit Dialog */}
      <Dialog open={dialogType === 'debit'} onOpenChange={(open) => !open && resetDialog()}>
        <DialogContent>
          <form onSubmit={handleDebit}>
            <DialogHeader>
              <DialogTitle>Debit Points</DialogTitle>
              <DialogDescription>
                Remove points from {selectedPlayer?.display_name}'s wallet.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="debit-amount">Amount</Label>
                <Input
                  id="debit-amount"
                  type="number"
                  min="1"
                  placeholder="Enter amount"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="debit-type">Transaction Type</Label>
                <Select value={transactionType} onValueChange={(v) => setTransactionType(v as WalletTransactionType)}>
                  <SelectTrigger id="debit-type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="debit">Debit</SelectItem>
                    <SelectItem value="transfer_out">Transfer Out</SelectItem>
                    <SelectItem value="reward_purchase">Reward Purchase</SelectItem>
                    <SelectItem value="penalty">Penalty</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="debit-description">Description (optional)</Label>
                <Input
                  id="debit-description"
                  placeholder="Reason for debit"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={resetDialog}>Cancel</Button>
              <Button type="submit" variant="destructive" disabled={debitMutation.isPending}>
                {debitMutation.isPending ? 'Processing...' : 'Debit Points'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Transfer Dialog */}
      <Dialog open={dialogType === 'transfer'} onOpenChange={(open) => !open && resetDialog()}>
        <DialogContent>
          <form onSubmit={handleTransfer}>
            <DialogHeader>
              <DialogTitle>Transfer Points</DialogTitle>
              <DialogDescription>
                Transfer points from {selectedPlayer?.display_name} to another player.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="transfer-to">To Player ID</Label>
                <Input
                  id="transfer-to"
                  type="number"
                  min="1"
                  placeholder="Enter recipient player ID"
                  value={toPlayerId}
                  onChange={(e) => setToPlayerId(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="transfer-amount">Amount</Label>
                <Input
                  id="transfer-amount"
                  type="number"
                  min="1"
                  placeholder="Enter amount"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="transfer-description">Description (optional)</Label>
                <Input
                  id="transfer-description"
                  placeholder="Reason for transfer"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={resetDialog}>Cancel</Button>
              <Button type="submit" variant="glow" disabled={transferMutation.isPending}>
                {transferMutation.isPending ? 'Processing...' : 'Transfer Points'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

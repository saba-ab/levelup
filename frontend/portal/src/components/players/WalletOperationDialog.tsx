import { useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useToast } from '@/hooks/use-toast';
import {
  PlayerApiError,
  useCreditWalletMutation,
  useDebitWalletMutation,
  useTransferPointsMutation,
  usePlayerWalletQuery,
} from '@/services/queries/players';
import {
  WALLET_CREDIT_KINDS,
  WALLET_DEBIT_KINDS,
  type Player,
  type WalletCreditKind,
  type WalletDebitKind,
} from '@/services/api/types';
import { getPlayerName, getTransactionKindLabel } from '@/lib/player-utils';
import { PlayerPicker } from './PlayerPicker';

export type WalletOperation = 'credit' | 'debit' | 'transfer';

interface WalletOperationDialogProps {
  operation: WalletOperation | null;
  onClose: () => void;
  /** Player the operation starts from; when omitted the dialog asks for one. */
  player?: Player | null;
}

const TITLES: Record<WalletOperation, string> = {
  credit: 'Credit Points',
  debit: 'Debit Points',
  transfer: 'Transfer Points',
};

/**
 * Credit, debit or transfer points. Each submit carries an Idempotency-Key,
 * and wallet error codes (insufficient_balance, wallet_inactive, ...) are
 * shown inline instead of a generic toast.
 */
export function WalletOperationDialog({ operation, onClose, player }: WalletOperationDialogProps) {
  const { toast } = useToast();
  const creditMutation = useCreditWalletMutation();
  const debitMutation = useDebitWalletMutation();
  const transferMutation = useTransferPointsMutation();

  const [source, setSource] = useState<Player | null>(player ?? null);
  const [target, setTarget] = useState<Player | null>(null);
  const [amount, setAmount] = useState('');
  const [description, setDescription] = useState('');
  const [creditKind, setCreditKind] = useState<WalletCreditKind>('adjustment');
  const [debitKind, setDebitKind] = useState<WalletDebitKind>('spend');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    if (!operation) return;
    setSource(player ?? null);
    setTarget(null);
    setAmount('');
    setDescription('');
    setCreditKind('adjustment');
    setDebitKind('spend');
    setErrorMessage(null);
  }, [operation, player]);

  const wallet = usePlayerWalletQuery(operation ? source?.id : undefined);
  const balance = wallet.data?.balance;

  const isPending = creditMutation.isPending || debitMutation.isPending || transferMutation.isPending;

  const describeError = async (err: unknown): Promise<string> => {
    if (!(err instanceof PlayerApiError)) return err instanceof Error ? err.message : 'Something went wrong';
    switch (err.code) {
      case 'insufficient_balance': {
        const fresh = await wallet.refetch();
        const available = fresh.data?.balance ?? balance;
        return available !== undefined
          ? `Not enough points: ${source ? getPlayerName(source) : 'the player'} has ${available.toLocaleString()} available.`
          : 'Not enough points in the wallet.';
      }
      case 'wallet_inactive':
        return 'This wallet is inactive, so its balance cannot change.';
      case 'player_inactive':
        return 'This player is inactive. Activate the player first.';
      case 'self_transfer':
        return 'Choose a different player to transfer to.';
      default:
        if (err.validationErrors) {
          return Object.entries(err.validationErrors)
            .map(([field, msgs]) => `${field}: ${msgs.join(', ')}`)
            .join('\n');
        }
        return err.message;
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!operation || !source) return;
    const value = Number(amount);
    if (!Number.isInteger(value) || value <= 0) {
      setErrorMessage('Amount must be a whole number greater than 0.');
      return;
    }
    setErrorMessage(null);
    const note = description.trim() || undefined;

    try {
      if (operation === 'credit') {
        await creditMutation.mutateAsync({ player_id: source.id, amount: value, kind: creditKind, description: note });
        toast({ title: 'Points credited', description: `${value.toLocaleString()} points added to ${getPlayerName(source)}.` });
      } else if (operation === 'debit') {
        await debitMutation.mutateAsync({ player_id: source.id, amount: value, kind: debitKind, description: note });
        toast({ title: 'Points debited', description: `${value.toLocaleString()} points removed from ${getPlayerName(source)}.` });
      } else {
        if (!target) {
          setErrorMessage('Choose the player to transfer to.');
          return;
        }
        await transferMutation.mutateAsync({ from_player_id: source.id, to_player_id: target.id, amount: value, description: note });
        toast({
          title: 'Points transferred',
          description: `${value.toLocaleString()} points moved from ${getPlayerName(source)} to ${getPlayerName(target)}.`,
        });
      }
      onClose();
    } catch (err) {
      setErrorMessage(await describeError(err));
    }
  };

  return (
    <Dialog open={operation !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        {operation && (
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>{TITLES[operation]}</DialogTitle>
              <DialogDescription>
                {operation === 'credit' && 'Add points to a player\'s wallet.'}
                {operation === 'debit' && 'Remove points from a player\'s wallet.'}
                {operation === 'transfer' && 'Move points from one player to another.'}
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="wallet-source">{operation === 'transfer' ? 'From player' : 'Player'}</Label>
                <PlayerPicker id="wallet-source" value={source} onChange={setSource} excludeId={target?.id} />
                {source && (
                  <p className="text-xs text-muted-foreground">
                    {wallet.isLoading
                      ? 'Loading balance...'
                      : wallet.data
                        ? wallet.data.opened
                          ? `Available balance: ${wallet.data.balance.toLocaleString()} points${wallet.data.is_active ? '' : ' (wallet inactive)'}`
                          : 'No wallet yet: it opens on the first credit.'
                        : null}
                  </p>
                )}
              </div>

              {operation === 'transfer' && (
                <div className="space-y-2">
                  <Label htmlFor="wallet-target">To player</Label>
                  <PlayerPicker id="wallet-target" value={target} onChange={setTarget} excludeId={source?.id} />
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="wallet-amount">Amount</Label>
                <Input
                  id="wallet-amount"
                  type="number"
                  min="1"
                  step="1"
                  placeholder="Enter amount"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  required
                />
              </div>

              {operation === 'credit' && (
                <div className="space-y-2">
                  <Label htmlFor="wallet-credit-kind">Kind</Label>
                  <Select value={creditKind} onValueChange={(v) => setCreditKind(v as WalletCreditKind)}>
                    <SelectTrigger id="wallet-credit-kind">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {WALLET_CREDIT_KINDS.map((k) => (
                        <SelectItem key={k} value={k}>
                          {getTransactionKindLabel(k)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}

              {operation === 'debit' && (
                <div className="space-y-2">
                  <Label htmlFor="wallet-debit-kind">Kind</Label>
                  <Select value={debitKind} onValueChange={(v) => setDebitKind(v as WalletDebitKind)}>
                    <SelectTrigger id="wallet-debit-kind">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {WALLET_DEBIT_KINDS.map((k) => (
                        <SelectItem key={k} value={k}>
                          {getTransactionKindLabel(k)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="wallet-description">Description (optional)</Label>
                <Input
                  id="wallet-description"
                  placeholder="Reason"
                  maxLength={255}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>

              {errorMessage && (
                <Alert variant="destructive">
                  <AlertDescription className="whitespace-pre-line">{errorMessage}</AlertDescription>
                </Alert>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                Cancel
              </Button>
              <Button
                type="submit"
                variant={operation === 'debit' ? 'destructive' : 'glow'}
                disabled={isPending || !source || (operation === 'transfer' && !target)}
              >
                {isPending ? 'Processing...' : TITLES[operation]}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

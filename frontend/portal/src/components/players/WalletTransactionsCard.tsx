import { useState } from 'react';
import { ArrowDownRight, ArrowUpRight, Receipt } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useWalletTransactionsQuery } from '@/services/queries/players';
import {
  WALLET_CREDIT_KINDS,
  WALLET_DEBIT_KINDS,
  type ID,
  type WalletDirection,
  type WalletTransactionType,
} from '@/services/api/types';
import { cn } from '@/lib/utils';
import { formatDateTime, getTransactionKindLabel } from '@/lib/player-utils';
import { PaginationControls } from './PaginationControls';

const ALL = 'all';
const KIND_OPTIONS: WalletTransactionType[] = [...WALLET_CREDIT_KINDS, ...WALLET_DEBIT_KINDS, 'transfer', 'refund'];

interface WalletTransactionsCardProps {
  playerId: ID;
}

/** Ledger of one wallet (GET /players/{id}/wallet/transactions), newest first. */
export function WalletTransactionsCard({ playerId }: WalletTransactionsCardProps) {
  const [direction, setDirection] = useState<WalletDirection | typeof ALL>(ALL);
  const [kind, setKind] = useState<WalletTransactionType | typeof ALL>(ALL);
  const pager = useCursorPagination(10);

  const { data, isLoading, isError } = useWalletTransactionsQuery(playerId, {
    limit: pager.limit,
    cursor: pager.cursor,
    direction: direction === ALL ? undefined : direction,
    kind: kind === ALL ? undefined : kind,
  });
  const entries = data?.data ?? [];

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <CardTitle className="text-lg flex items-center gap-2">
            <Receipt className="w-5 h-5" />
            Wallet Transactions
          </CardTitle>
          <div className="flex gap-2">
            <Select
              value={direction}
              onValueChange={(v) => {
                setDirection(v as WalletDirection | typeof ALL);
                pager.reset();
              }}
            >
              <SelectTrigger className="w-[130px] h-9" aria-label="Direction">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All directions</SelectItem>
                <SelectItem value="credit">Credits</SelectItem>
                <SelectItem value="debit">Debits</SelectItem>
              </SelectContent>
            </Select>
            <Select
              value={kind}
              onValueChange={(v) => {
                setKind(v as WalletTransactionType | typeof ALL);
                pager.reset();
              }}
            >
              <SelectTrigger className="w-[130px] h-9" aria-label="Kind">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>All kinds</SelectItem>
                {KIND_OPTIONS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {getTransactionKindLabel(k)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {isLoading ? (
          <p className="p-6 text-sm text-center text-muted-foreground">Loading transactions...</p>
        ) : isError ? (
          <p className="p-6 text-sm text-center text-destructive">Could not load transactions.</p>
        ) : entries.length === 0 ? (
          <p className="p-6 text-sm text-center text-muted-foreground">No transactions.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-3 font-medium text-muted-foreground">When</th>
                  <th className="text-left p-3 font-medium text-muted-foreground">Kind</th>
                  <th className="text-left p-3 font-medium text-muted-foreground">Description</th>
                  <th className="text-right p-3 font-medium text-muted-foreground">Amount</th>
                  <th className="text-right p-3 font-medium text-muted-foreground">Balance</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((t) => {
                  const isCredit = t.direction === 'credit';
                  return (
                    <tr key={t.id} className="border-b border-border/50">
                      <td className="p-3 text-muted-foreground whitespace-nowrap">{formatDateTime(t.occurred_at)}</td>
                      <td className="p-3">
                        <Badge variant="outline">{getTransactionKindLabel(t.kind)}</Badge>
                      </td>
                      <td className="p-3 text-muted-foreground">{t.description || '-'}</td>
                      <td
                        className={cn(
                          'p-3 text-right font-mono whitespace-nowrap',
                          isCredit ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive'
                        )}
                      >
                        <span className="inline-flex items-center gap-1">
                          {isCredit ? <ArrowUpRight className="w-3 h-3" /> : <ArrowDownRight className="w-3 h-3" />}
                          {isCredit ? '+' : '-'}
                          {t.amount.toLocaleString()}
                        </span>
                      </td>
                      <td className="p-3 text-right font-mono">{t.balance_after.toLocaleString()}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        <PaginationControls
          page={pager.page}
          hasPrevious={pager.hasPrevious}
          hasNext={!!data?.next_cursor}
          onPrevious={pager.previous}
          onNext={() => data && pager.next(data.next_cursor)}
        />
      </CardContent>
    </Card>
  );
}

import { useState } from 'react';
import { Search, X, Loader2 } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { usePlayersQuery } from '@/services/queries/players';
import type { ID, Player } from '@/services/api/types';
import { getPlayerName } from '@/lib/player-utils';
import { PlayerAvatar } from './PlayerAvatar';
import { useDebouncedValue } from './useDebouncedValue';

interface PlayerPickerProps {
  id?: string;
  value: Player | null;
  onChange: (player: Player | null) => void;
  /** Hide this player from the results (e.g. the transfer source). */
  excludeId?: ID;
  placeholder?: string;
}

/** Server-side player search (GET /players?search=) with a single selection. */
export function PlayerPicker({ id, value, onChange, excludeId, placeholder = 'Search players by name, email or ID...' }: PlayerPickerProps) {
  const [search, setSearch] = useState('');
  const debounced = useDebouncedValue(search.trim());
  const { data, isFetching } = usePlayersQuery({ search: debounced || undefined, limit: 8 }, { enabled: !value });
  const results = (data?.data ?? []).filter((p) => p.id !== excludeId);

  if (value) {
    return (
      <div className="flex items-center justify-between gap-3 rounded-md border border-border p-2">
        <div className="flex items-center gap-2 min-w-0">
          <PlayerAvatar name={getPlayerName(value)} size="sm" />
          <div className="min-w-0">
            <p className="text-sm font-medium truncate">{getPlayerName(value)}</p>
            <p className="text-xs text-muted-foreground truncate">{value.external_id}</p>
          </div>
        </div>
        <Button type="button" variant="ghost" size="icon" className="h-8 w-8" onClick={() => onChange(null)} aria-label="Clear player">
          <X className="h-4 w-4" />
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <Input id={id} className="pl-9" placeholder={placeholder} value={search} onChange={(e) => setSearch(e.target.value)} />
        {isFetching && <Loader2 className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 animate-spin text-muted-foreground" />}
      </div>
      <div className="max-h-48 overflow-y-auto rounded-md border border-border divide-y divide-border">
        {results.length === 0 ? (
          <p className="p-3 text-sm text-muted-foreground text-center">{isFetching ? 'Searching...' : 'No players found'}</p>
        ) : (
          results.map((p) => (
            <button
              key={p.id}
              type="button"
              className="w-full flex items-center gap-2 p-2 text-left hover:bg-secondary/50 transition-colors"
              onClick={() => onChange(p)}
            >
              <PlayerAvatar name={getPlayerName(p)} size="sm" />
              <div className="min-w-0">
                <p className="text-sm font-medium truncate">{getPlayerName(p)}</p>
                <p className="text-xs text-muted-foreground truncate">
                  {p.external_id}
                  {p.email ? ` · ${p.email}` : ''}
                </p>
              </div>
            </button>
          ))
        )}
      </div>
    </div>
  );
}

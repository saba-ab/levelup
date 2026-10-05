import { useEffect, useState } from 'react';
import { Search } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { usePlayersQuery } from '@/services/queries/players';

interface PlayerPickerProps {
  /** Selected player id (UUID string) or "". */
  value: string;
  onChange: (playerId: string) => void;
  label?: string;
  id?: string;
}

/** Searchable player select (server-side ?search=, first 50 matches). */
export function PlayerPicker({ value, onChange, label = 'Player', id = 'player-picker' }: PlayerPickerProps) {
  const [search, setSearch] = useState('');
  const [debounced, setDebounced] = useState('');

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search.trim()), 300);
    return () => clearTimeout(t);
  }, [search]);

  const { data, isLoading, error } = usePlayersQuery({ limit: 50, search: debounced || undefined });
  const players = data?.data ?? [];

  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label} *</Label>
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
        <Input
          className="pl-9"
          placeholder="Search by name, email or external id..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search players"
        />
      </div>
      <Select value={value || undefined} onValueChange={onChange}>
        <SelectTrigger id={id}>
          <SelectValue placeholder={isLoading ? 'Loading players...' : 'Select a player'} />
        </SelectTrigger>
        <SelectContent>
          {players.length === 0 ? (
            <SelectItem value="__none" disabled>
              {error ? 'Failed to load players' : 'No players match'}
            </SelectItem>
          ) : (
            players.map((player) => (
              <SelectItem key={player.id} value={player.id}>
                {player.display_name || player.external_id}
                <span className="ml-2 text-xs text-muted-foreground">{player.external_id}</span>
              </SelectItem>
            ))
          )}
        </SelectContent>
      </Select>
    </div>
  );
}

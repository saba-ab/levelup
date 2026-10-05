import React, { useState, useRef, useMemo } from 'react';
import { Upload, Search, Check, Loader2, FileSpreadsheet, Users, AlertCircle } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Checkbox } from '@/components/ui/checkbox';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { usePlayersQuery } from '@/services/queries/players';
import type { BulkEnrollResult, ID } from '@/services/api/types';

interface BulkEnrollDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Players known to be enrolled (hidden from the picker). */
  enrolledPlayerIds: Set<ID>;
  /** Enrolls the ids; resolves with per-player failures so the dialog can list them. */
  onEnroll: (playerIds: ID[]) => Promise<BulkEnrollResult | undefined>;
  isLoading: boolean;
  programName?: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const FAILURE_MESSAGES: Record<string, string> = {
  player_not_found: 'player not found',
  player_inactive: 'player is inactive',
  program_not_accepting_players: 'program is not accepting players',
};

interface CSVParseResult {
  playerIds: string[];
  errors: string[];
}

export function BulkEnrollDialog({
  open,
  onOpenChange,
  enrolledPlayerIds,
  onEnroll,
  isLoading,
  programName,
}: BulkEnrollDialogProps) {
  const [activeTab, setActiveTab] = useState<'select' | 'csv'>('select');
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedPlayerIds, setSelectedPlayerIds] = useState<Set<ID>>(new Set());
  const [failures, setFailures] = useState<BulkEnrollResult['failures']>([]);
  const [csvPlayerIds, setCsvPlayerIds] = useState<string[]>([]);
  const [csvErrors, setCsvErrors] = useState<string[]>([]);
  const [csvFileName, setCsvFileName] = useState<string>('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  const { data: playersData, isLoading: playersLoading } = usePlayersQuery({
    limit: 100,
    search: searchQuery || undefined,
  });

  const allPlayers = useMemo(() => playersData?.data ?? [], [playersData]);
  
  // Filter out already enrolled players
  const availablePlayers = useMemo(() => 
    allPlayers.filter(p => !enrolledPlayerIds.has(p.id)),
    [allPlayers, enrolledPlayerIds]
  );

  const playerLabel = (playerId: ID) => {
    const player = allPlayers.find(p => p.id === playerId);
    return player ? player.display_name || player.external_id : playerId;
  };

  const togglePlayer = (playerId: ID) => {
    setSelectedPlayerIds(prev => {
      const next = new Set(prev);
      if (next.has(playerId)) {
        next.delete(playerId);
      } else {
        next.add(playerId);
      }
      return next;
    });
  };

  const toggleAll = () => {
    if (selectedPlayerIds.size === availablePlayers.length) {
      setSelectedPlayerIds(new Set());
    } else {
      setSelectedPlayerIds(new Set(availablePlayers.map(p => p.id)));
    }
  };

  const parseCSV = (content: string): CSVParseResult => {
    const lines = content.split(/[\r\n]+/).filter(line => line.trim());
    const playerIds: string[] = [];
    const errors: string[] = [];

    lines.forEach((line, index) => {
      const trimmed = line.trim();
      // Skip header if it looks like one
      if (index === 0 && (trimmed.toLowerCase().includes('id') || trimmed.toLowerCase().includes('player'))) {
        return;
      }
      // Handle CSV with multiple columns - take first column
      const firstColumn = trimmed.split(',')[0].trim().replace(/["']/g, '');
      if (firstColumn) {
        playerIds.push(firstColumn);
      }
    });

    return { playerIds, errors };
  };

  const handleFileUpload = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    setCsvFileName(file.name);
    const reader = new FileReader();
    reader.onload = (e) => {
      const content = e.target?.result as string;
      const { playerIds, errors } = parseCSV(content);
      setCsvPlayerIds(playerIds);
      setCsvErrors(errors);
    };
    reader.readAsText(file);
  };

  /** CSV rows are player UUIDs, or external ids matched against the loaded players. */
  const resolveCsvIds = (): { ids: ID[]; unmatched: string[] } => {
    const ids: ID[] = [];
    const unmatched: string[] = [];
    csvPlayerIds.forEach(value => {
      if (UUID_RE.test(value)) {
        ids.push(value);
        return;
      }
      const match = allPlayers.find(p => p.external_id === value);
      if (match) ids.push(match.id);
      else unmatched.push(value);
    });
    return { ids: Array.from(new Set(ids)), unmatched };
  };

  const handleEnroll = async () => {
    let playerIdsToEnroll: ID[];

    if (activeTab === 'select') {
      playerIdsToEnroll = Array.from(selectedPlayerIds);
    } else {
      const { ids, unmatched } = resolveCsvIds();
      playerIdsToEnroll = ids;
      setCsvErrors(unmatched.map(v => `${v} (not a player id or a loaded external id)`));
    }

    if (playerIdsToEnroll.length === 0) return;

    const result = await onEnroll(playerIdsToEnroll);
    const failed = result?.failures ?? [];
    setFailures(failed);
    if (failed.length > 0) {
      // Keep only the failed players selected so they can be retried.
      setSelectedPlayerIds(new Set(failed.map(f => f.playerId)));
      return;
    }

    // Reset state
    setSelectedPlayerIds(new Set());
    setCsvPlayerIds([]);
    setCsvErrors([]);
    setCsvFileName('');
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  const handleClose = () => {
    setSelectedPlayerIds(new Set());
    setCsvPlayerIds([]);
    setCsvErrors([]);
    setCsvFileName('');
    setSearchQuery('');
    setFailures([]);
    onOpenChange(false);
  };

  const enrollCount = activeTab === 'select' ? selectedPlayerIds.size : csvPlayerIds.length;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>Bulk Enroll Players</DialogTitle>
          <DialogDescription>
            Add multiple players to {programName ? `"${programName}"` : 'this program'} at once
          </DialogDescription>
        </DialogHeader>

        <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v as 'select' | 'csv')} className="flex-1 flex flex-col min-h-0">
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="select">
              <Users className="w-4 h-4 mr-2" />
              Select Players
            </TabsTrigger>
            <TabsTrigger value="csv">
              <FileSpreadsheet className="w-4 h-4 mr-2" />
              CSV Import
            </TabsTrigger>
          </TabsList>

          <TabsContent value="select" className="flex-1 flex flex-col min-h-0 mt-4">
            <div className="relative mb-3">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder="Search players..."
                className="pl-9"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>

            {availablePlayers.length > 0 && (
              <div className="flex items-center justify-between mb-2 px-1">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={toggleAll}
                  className="text-xs"
                >
                  {selectedPlayerIds.size === availablePlayers.length ? 'Deselect All' : 'Select All'}
                </Button>
                <span className="text-sm text-muted-foreground">
                  {selectedPlayerIds.size} selected
                </span>
              </div>
            )}

            <ScrollArea className="flex-1 border rounded-lg">
              <div className="p-2 space-y-1">
                {playersLoading ? (
                  Array.from({ length: 5 }).map((_, i) => (
                    <div key={i} className="flex items-center gap-3 p-2">
                      <Skeleton className="w-5 h-5 rounded" />
                      <Skeleton className="w-8 h-8 rounded-full" />
                      <div className="space-y-1 flex-1">
                        <Skeleton className="h-4 w-32" />
                        <Skeleton className="h-3 w-24" />
                      </div>
                    </div>
                  ))
                ) : availablePlayers.length === 0 ? (
                  <div className="text-center py-8 text-muted-foreground">
                    {searchQuery ? 'No matching players found' : 'All players are already enrolled'}
                  </div>
                ) : (
                  availablePlayers.map((player) => (
                    <div
                      key={player.id}
                      className={cn(
                        'flex items-center gap-3 p-2 rounded-lg cursor-pointer transition-colors',
                        selectedPlayerIds.has(player.id)
                          ? 'bg-primary/10 border border-primary/20'
                          : 'hover:bg-secondary/50'
                      )}
                      onClick={() => togglePlayer(player.id)}
                    >
                      <Checkbox
                        checked={selectedPlayerIds.has(player.id)}
                        onCheckedChange={() => togglePlayer(player.id)}
                      />
                      <Avatar className="w-8 h-8">
                        <AvatarFallback className="text-xs">
                          {player.display_name?.substring(0, 2).toUpperCase() || player.external_id.substring(0, 2).toUpperCase()}
                        </AvatarFallback>
                      </Avatar>
                      <div className="flex-1 min-w-0">
                        <p className="font-medium text-sm truncate">
                          {player.display_name || player.external_id}
                        </p>
                        <p className="text-xs text-muted-foreground truncate">{player.email || player.external_id}</p>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </ScrollArea>
          </TabsContent>

          <TabsContent value="csv" className="flex-1 flex flex-col min-h-0 mt-4 space-y-4">
            <div
              className={cn(
                'border-2 border-dashed rounded-lg p-8 text-center transition-colors cursor-pointer',
                'hover:border-primary/50 hover:bg-secondary/30'
              )}
              onClick={() => fileInputRef.current?.click()}
            >
              <input
                ref={fileInputRef}
                type="file"
                accept=".csv,.txt"
                onChange={handleFileUpload}
                className="hidden"
              />
              <Upload className="w-10 h-10 mx-auto text-muted-foreground mb-3" />
              <p className="font-medium">
                {csvFileName || 'Click to upload CSV'}
              </p>
              <p className="text-sm text-muted-foreground mt-1">
                CSV with player IDs (UUIDs) or external IDs (one per row)
              </p>
            </div>

            {csvPlayerIds.length > 0 && (
              <Alert>
                <Check className="h-4 w-4" />
                <AlertDescription>
                  Found <strong>{csvPlayerIds.length}</strong> player IDs in the file
                </AlertDescription>
              </Alert>
            )}

            {csvErrors.length > 0 && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>
                  {csvErrors.length} not matched: {csvErrors.slice(0, 3).join(', ')}
                  {csvErrors.length > 3 && ` and ${csvErrors.length - 3} more`}
                </AlertDescription>
              </Alert>
            )}

            {csvPlayerIds.length > 0 && (
              <ScrollArea className="flex-1 border rounded-lg">
                <div className="p-3">
                  <p className="text-sm font-medium mb-2">Player IDs to enroll:</p>
                  <div className="flex flex-wrap gap-2">
                    {csvPlayerIds.slice(0, 50).map((id, i) => (
                      <Badge key={i} variant="secondary" className="text-xs">
                        {id}
                      </Badge>
                    ))}
                    {csvPlayerIds.length > 50 && (
                      <Badge variant="outline" className="text-xs">
                        +{csvPlayerIds.length - 50} more
                      </Badge>
                    )}
                  </div>
                </div>
              </ScrollArea>
            )}
          </TabsContent>
        </Tabs>

        {failures.length > 0 && (
          <Alert variant="destructive" className="mt-4">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>
              <p className="font-medium">{failures.length} player{failures.length !== 1 ? 's' : ''} could not be enrolled:</p>
              <ul className="mt-1 max-h-32 overflow-y-auto text-sm list-disc pl-4">
                {failures.map(f => (
                  <li key={f.playerId}>
                    {playerLabel(f.playerId)}: {(f.code && FAILURE_MESSAGES[f.code]) || f.error}
                  </li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        )}

        <DialogFooter className="mt-4">
          <Button variant="outline" onClick={handleClose}>
            Cancel
          </Button>
          <Button
            onClick={handleEnroll}
            disabled={isLoading || enrollCount === 0}
          >
            {isLoading ? (
              <>
                <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                Enrolling...
              </>
            ) : (
              <>
                Enroll {enrollCount} Player{enrollCount !== 1 ? 's' : ''}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

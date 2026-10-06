import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Pencil, RefreshCw, Trash2, Users } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import CursorPager from '@/components/CursorPager';
import { useToast } from '@/hooks/use-toast';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useAllBadgesQuery } from '@/services/queries/mechanics';
import {
  describeSegmentError,
  isSegmentRefreshPending,
  useDeleteSegmentMutation,
  useInvalidateSegmentPlayers,
  useRefreshSegmentMutation,
  useSegmentPlayersQuery,
  useSegmentQuery,
} from '@/services/queries/segments';
import type { ID } from '@/services/api/models/common';
import { ConditionSummary } from './ConditionSummary';
import { RefreshStatus } from './RefreshStatus';
import { SegmentFormDialog } from './SegmentFormDialog';

interface SegmentDetailProps {
  segmentId: ID;
  canManage: boolean;
  onBack: () => void;
}

export function SegmentDetail({ segmentId, canManage, onBack }: SegmentDetailProps) {
  const { toast } = useToast();
  const pager = useCursorPagination(25);
  const [editing, setEditing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const segmentQuery = useSegmentQuery(segmentId);
  const membersQuery = useSegmentPlayersQuery(segmentId, { limit: pager.limit, cursor: pager.cursor });
  const badgesQuery = useAllBadgesQuery();
  const refreshMutation = useRefreshSegmentMutation();
  const deleteMutation = useDeleteSegmentMutation();
  const invalidatePlayers = useInvalidateSegmentPlayers();

  const segment = segmentQuery.data;
  const pending = segment ? isSegmentRefreshPending(segment) : false;

  // When a refresh lands, reload members from the first page.
  const wasPending = useRef(pending);
  const { reset: resetPager } = pager;
  useEffect(() => {
    if (wasPending.current && !pending) {
      resetPager();
      invalidatePlayers(segmentId);
    }
    wasPending.current = pending;
  }, [pending, segmentId, resetPager, invalidatePlayers]);

  const badgeNames = useMemo(() => new Map((badgesQuery.data ?? []).map((b) => [b.id, b.name])), [badgesQuery.data]);

  const handleRefresh = async () => {
    try {
      await refreshMutation.mutateAsync(segmentId);
      toast({ title: 'Refresh queued', description: 'Membership will update shortly.' });
    } catch (err) {
      toast({ title: 'Refresh failed', description: describeSegmentError(err, 'Failed to refresh'), variant: 'destructive' });
    }
  };

  const handleDelete = async () => {
    try {
      await deleteMutation.mutateAsync(segmentId);
      toast({ title: 'Segment deleted' });
      onBack();
    } catch (err) {
      toast({ title: 'Delete failed', description: describeSegmentError(err, 'Failed to delete'), variant: 'destructive' });
    }
  };

  if (segmentQuery.isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (segmentQuery.error || !segment) {
    return (
      <div className="flex flex-col items-start gap-4">
        <Button variant="ghost" onClick={onBack}>
          <ArrowLeft className="mr-2 h-4 w-4" />
          All segments
        </Button>
        <Card className="w-full">
          <CardContent className="py-12 text-center text-muted-foreground">
            {describeSegmentError(segmentQuery.error, 'This segment could not be loaded.')}
          </CardContent>
        </Card>
      </div>
    );
  }

  const members = membersQuery.data?.data ?? [];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex flex-col gap-1">
          <Button variant="ghost" size="sm" className="-ml-3 w-fit" onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            All segments
          </Button>
          <h1 className="text-3xl font-bold text-foreground">{segment.name}</h1>
          {segment.description && <p className="text-muted-foreground">{segment.description}</p>}
        </div>
        {canManage && (
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={handleRefresh} disabled={refreshMutation.isPending || pending}>
              <RefreshCw className={`mr-2 h-4 w-4 ${pending ? 'animate-spin' : ''}`} />
              Refresh now
            </Button>
            <Button variant="outline" onClick={() => setEditing(true)}>
              <Pencil className="mr-2 h-4 w-4" />
              Edit
            </Button>
            <Button variant="outline" className="text-destructive" onClick={() => setConfirmDelete(true)}>
              <Trash2 className="mr-2 h-4 w-4" />
              Delete
            </Button>
          </div>
        )}
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Members</CardDescription>
            <CardTitle className="text-3xl">{segment.member_count.toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Last refreshed</CardDescription>
            <div className="pt-1">
              <RefreshStatus segment={segment} />
            </div>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Created</CardDescription>
            <CardTitle className="text-base font-medium">{new Date(segment.created_at).toLocaleDateString()}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Conditions</CardTitle>
        </CardHeader>
        <CardContent>
          <ConditionSummary conditions={segment.conditions} badgeName={(id) => badgeNames.get(id)} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Members</CardTitle>
          <CardDescription>As of the last refresh, newest first.</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {membersQuery.isLoading ? (
            <div className="flex flex-col gap-2 p-6">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </div>
          ) : membersQuery.error ? (
            <p className="p-6 text-sm text-destructive">
              {describeSegmentError(membersQuery.error, 'Failed to load members')}
            </p>
          ) : members.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-12 text-muted-foreground">
              <Users className="h-8 w-8" />
              <p>{pending ? 'Membership is being calculated…' : 'No players are in this segment.'}</p>
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Player</TableHead>
                    <TableHead>External ID</TableHead>
                    <TableHead className="text-right">Added</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {members.map((m) => (
                    <TableRow key={m.player_id}>
                      <TableCell>
                        <Link to={`/players/${m.player_id}`} className="font-medium hover:underline">
                          {m.display_name || m.external_id || m.player_id}
                        </Link>
                      </TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{m.external_id || '—'}</TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">
                        {new Date(m.added_at).toLocaleString()}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <CursorPager
                page={pager.page}
                hasPrevious={pager.hasPrevious}
                nextCursor={membersQuery.data?.next_cursor}
                onPrevious={pager.previous}
                onNext={pager.next}
                isFetching={membersQuery.isFetching}
              />
            </>
          )}
        </CardContent>
      </Card>

      {canManage && <SegmentFormDialog open={editing} onOpenChange={setEditing} segment={segment} />}

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete "{segment.name}"?</AlertDialogTitle>
            <AlertDialogDescription>
              The segment and its membership are removed. Anything targeting this segment stops matching.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

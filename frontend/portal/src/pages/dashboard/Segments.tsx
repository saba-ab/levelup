import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Plus, RefreshCw, Users } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import CursorPager from '@/components/CursorPager';
import { useAuth } from '@/contexts/AuthContext';
import { useToast } from '@/hooks/use-toast';
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { RefreshStatus } from '@/components/segments/RefreshStatus';
import { SegmentDetail } from '@/components/segments/SegmentDetail';
import { SegmentFormDialog } from '@/components/segments/SegmentFormDialog';
import {
  describeSegmentError,
  isSegmentRefreshPending,
  useRefreshSegmentMutation,
  useSegmentsQuery,
} from '@/services/queries/segments';
import type { Segment } from '@/services/api/models/segments';

/** Selected segment lives in ?segment=<id> so the detail view is linkable. */
const SEGMENT_PARAM = 'segment';

export default function Segments() {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get(SEGMENT_PARAM);
  const { hasPermission } = useAuth();
  const canManage = hasPermission('manage:segments');

  const select = (id: string | null) => {
    const next = new URLSearchParams(searchParams);
    if (id) next.set(SEGMENT_PARAM, id);
    else next.delete(SEGMENT_PARAM);
    setSearchParams(next);
  };

  if (selectedId) {
    return <SegmentDetail segmentId={selectedId} canManage={canManage} onBack={() => select(null)} />;
  }
  return <SegmentList canManage={canManage} onSelect={select} />;
}

function SegmentList({ canManage, onSelect }: { canManage: boolean; onSelect: (id: string) => void }) {
  const { toast } = useToast();
  const pager = useCursorPagination(25);
  const [creating, setCreating] = useState(false);
  const { data, isLoading, isFetching, error } = useSegmentsQuery({ limit: pager.limit, cursor: pager.cursor });
  const refreshMutation = useRefreshSegmentMutation();
  const segments = data?.data ?? [];

  const handleRefresh = async (segment: Segment) => {
    try {
      await refreshMutation.mutateAsync(segment.id);
      toast({ title: 'Refresh queued', description: `"${segment.name}" will update shortly.` });
    } catch (err) {
      toast({ title: 'Refresh failed', description: describeSegmentError(err, 'Failed to refresh'), variant: 'destructive' });
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold text-foreground">Segments</h1>
          <p className="mt-1 text-muted-foreground">Group players by attributes, progress and activity</p>
        </div>
        {canManage && (
          <Button onClick={() => setCreating(true)}>
            <Plus className="mr-2 h-4 w-4" />
            New segment
          </Button>
        )}
      </div>

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex flex-col gap-2 p-6">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : error ? (
            <p className="p-6 text-sm text-destructive">{describeSegmentError(error, 'Failed to load segments')}</p>
          ) : segments.length === 0 ? (
            <div className="flex flex-col items-center gap-3 py-16 text-center">
              <Users className="h-10 w-10 text-muted-foreground" />
              <div>
                <p className="font-medium">No segments yet</p>
                <p className="text-sm text-muted-foreground">
                  Segments group players by level, points, badges, activity or custom attributes.
                </p>
              </div>
              {canManage && (
                <Button onClick={() => setCreating(true)}>
                  <Plus className="mr-2 h-4 w-4" />
                  Create your first segment
                </Button>
              )}
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead className="text-right">Members</TableHead>
                    <TableHead>Last refreshed</TableHead>
                    <TableHead>Updated</TableHead>
                    {canManage && <TableHead className="w-[60px]" />}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {segments.map((s) => (
                    <TableRow key={s.id} className="cursor-pointer" onClick={() => onSelect(s.id)}>
                      <TableCell>
                        <div className="font-medium">{s.name}</div>
                        {s.description && (
                          <div className="line-clamp-1 text-sm text-muted-foreground">{s.description}</div>
                        )}
                      </TableCell>
                      <TableCell className="text-right font-medium tabular-nums">
                        {s.member_count.toLocaleString()}
                      </TableCell>
                      <TableCell>
                        <RefreshStatus segment={s} />
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {new Date(s.updated_at).toLocaleDateString()}
                      </TableCell>
                      {canManage && (
                        <TableCell>
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={`Refresh ${s.name}`}
                            disabled={isSegmentRefreshPending(s) || refreshMutation.isPending}
                            onClick={(e) => {
                              e.stopPropagation();
                              handleRefresh(s);
                            }}
                          >
                            <RefreshCw className="h-4 w-4" />
                          </Button>
                        </TableCell>
                      )}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <CursorPager
                page={pager.page}
                hasPrevious={pager.hasPrevious}
                nextCursor={data?.next_cursor}
                onPrevious={pager.previous}
                onNext={pager.next}
                isFetching={isFetching}
              />
            </>
          )}
        </CardContent>
      </Card>

      {canManage && (
        <SegmentFormDialog open={creating} onOpenChange={setCreating} onSaved={(s) => onSelect(s.id)} />
      )}
    </div>
  );
}

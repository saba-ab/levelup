import { useState } from 'react';
import { Bell, Edit, FileText, Loader2, Mail, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Skeleton } from '@/components/ui/skeleton';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
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
import { useCursorPagination } from '@/hooks/useCursorPagination';
import { useToast } from '@/hooks/use-toast';
import { formatDateTime } from '@/lib/player-utils';
import {
  useNotificationTemplatesQuery,
  useUpdateNotificationTemplateMutation,
  useDeleteNotificationTemplateMutation,
} from '@/services/queries/notifications';
import { NOTIFICATION_TRIGGERS, type NotificationTemplate, type NotificationTrigger } from '@/services/api/models/notifications';
import { CHANNEL_LABELS, TRIGGER_LABELS } from './templateVariables';
import { TemplateEditorDialog } from './TemplateEditorDialog';

const ALL = 'all';

interface TemplatesTabProps {
  canCreate: boolean;
  canUpdate: boolean;
  canDelete: boolean;
}

/** Template list with filters, activate toggle, editor and delete. */
export function TemplatesTab({ canCreate, canUpdate, canDelete }: TemplatesTabProps) {
  const { toast } = useToast();
  const pager = useCursorPagination(25);
  const [triggerFilter, setTriggerFilter] = useState<string>(ALL);
  const [activeFilter, setActiveFilter] = useState<string>(ALL);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<NotificationTemplate | null>(null);
  const [deleting, setDeleting] = useState<NotificationTemplate | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);

  const { data, isLoading, isError, error, refetch, isFetching } = useNotificationTemplatesQuery({
    trigger: triggerFilter === ALL ? undefined : (triggerFilter as NotificationTrigger),
    is_active: activeFilter === ALL ? undefined : activeFilter === 'active',
    limit: pager.limit,
    cursor: pager.cursor,
  });
  const updateMutation = useUpdateNotificationTemplateMutation();
  const deleteMutation = useDeleteNotificationTemplateMutation();
  const templates = data?.data ?? [];
  const hasFilters = triggerFilter !== ALL || activeFilter !== ALL;

  const openEditor = (template: NotificationTemplate | null) => {
    setEditing(template);
    setEditorOpen(true);
  };

  const toggleActive = (t: NotificationTemplate, active: boolean) => {
    setTogglingId(t.id);
    updateMutation.mutate(
      { templateId: t.id, data: { is_active: active } },
      {
        onSuccess: () => toast({ title: active ? 'Template activated' : 'Template deactivated', description: t.name }),
        onError: (err) => toast({ title: 'Could not update template', description: err.message, variant: 'destructive' }),
        onSettled: () => setTogglingId(null),
      },
    );
  };

  const confirmDelete = () => {
    if (!deleting) return;
    const t = deleting;
    deleteMutation.mutate(t.id, {
      onSuccess: () => {
        toast({ title: 'Template deleted', description: `${t.name}. Its history is kept.` });
        setDeleting(null);
      },
      onError: (err) => toast({ title: 'Could not delete template', description: err.message, variant: 'destructive' }),
    });
  };

  return (
    <Card>
      <CardHeader className="flex flex-col gap-4 space-y-0 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <CardTitle>Templates</CardTitle>
          <CardDescription>Each active template sends on its trigger to every channel it lists.</CardDescription>
        </div>
        {canCreate && (
          <Button variant="glow" onClick={() => openEditor(null)}>
            <Plus className="h-4 w-4" />
            New template
          </Button>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap gap-3">
          <Select value={triggerFilter} onValueChange={(v) => { setTriggerFilter(v); pager.reset(); }}>
            <SelectTrigger className="w-[220px]">
              <SelectValue placeholder="Trigger" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All triggers</SelectItem>
              {NOTIFICATION_TRIGGERS.map((t) => (
                <SelectItem key={t} value={t}>{TRIGGER_LABELS[t]}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={activeFilter} onValueChange={(v) => { setActiveFilter(v); pager.reset(); }}>
            <SelectTrigger className="w-[160px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All statuses</SelectItem>
              <SelectItem value="active">Active</SelectItem>
              <SelectItem value="inactive">Inactive</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-12" />)}
          </div>
        ) : isError ? (
          <div className="flex flex-col items-center gap-3 py-10 text-center">
            <p className="text-sm text-muted-foreground">Could not load templates: {error.message}</p>
            <Button variant="outline" size="sm" className="gap-2" onClick={() => refetch()}>
              <RefreshCw className="h-4 w-4" />
              Retry
            </Button>
          </div>
        ) : templates.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-12 text-center">
            <FileText className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{hasFilters ? 'No templates match these filters' : 'No notification templates yet'}</p>
            <p className="text-sm text-muted-foreground">
              {hasFilters ? 'Try other filters.' : 'Create one to notify players when they earn badges, level up and more.'}
            </p>
            {!hasFilters && canCreate && (
              <Button variant="outline" size="sm" className="mt-2 gap-2" onClick={() => openEditor(null)}>
                <Plus className="h-4 w-4" />
                New template
              </Button>
            )}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Trigger</TableHead>
                  <TableHead>Channels</TableHead>
                  <TableHead>Active</TableHead>
                  <TableHead>Updated</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {templates.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell>
                      <p className="font-medium">{t.name}</p>
                      <p className="max-w-[280px] truncate font-mono text-xs text-muted-foreground">{t.title_template}</p>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline" className="whitespace-nowrap font-normal">{TRIGGER_LABELS[t.trigger] ?? t.trigger}</Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        {t.channels.map((ch) => (
                          <Badge key={ch} variant="secondary" className="gap-1 text-xs">
                            {ch === 'email' ? <Mail className="h-3 w-3" /> : <Bell className="h-3 w-3" />}
                            {CHANNEL_LABELS[ch] ?? ch}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Switch
                          checked={t.is_active}
                          disabled={!canUpdate || togglingId === t.id}
                          onCheckedChange={(c) => toggleActive(t, c)}
                          aria-label={t.is_active ? 'Deactivate template' : 'Activate template'}
                        />
                        {togglingId === t.id && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
                      </div>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-sm text-muted-foreground">{formatDateTime(t.updated_at)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openEditor(t)} aria-label="Edit template" disabled={!canUpdate}>
                          <Edit className="h-4 w-4" />
                        </Button>
                        {canDelete && (
                          <Button variant="ghost" size="icon" className="h-8 w-8 text-destructive" onClick={() => setDeleting(t)} aria-label="Delete template">
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <CursorPager
          page={pager.page}
          hasPrevious={pager.hasPrevious}
          nextCursor={data?.next_cursor}
          onPrevious={pager.previous}
          onNext={pager.next}
          isFetching={isFetching}
        />
      </CardContent>

      <TemplateEditorDialog open={editorOpen} onOpenChange={setEditorOpen} template={editing} />

      <AlertDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete “{deleting?.name}”?</AlertDialogTitle>
            <AlertDialogDescription>
              It stops sending immediately. Notifications already sent stay in the history.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => { e.preventDefault(); confirmDelete(); }}
              disabled={deleteMutation.isPending}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteMutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

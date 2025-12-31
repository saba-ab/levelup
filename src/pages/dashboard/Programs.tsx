import React, { useState } from 'react';
import { Plus, Search, MoreHorizontal, Play, Pause, Pencil, Trash2, FolderOpen, StopCircle, Loader2 } from 'lucide-react';
import { format } from 'date-fns';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { useToast } from '@/hooks/use-toast';
import { ProgramFormDialog } from '@/components/programs/ProgramFormDialog';
import {
  useProgramsQuery,
  useCreateProgramMutation,
  useUpdateProgramMutation,
  useDeleteProgramMutation,
  useActivateProgramMutation,
  usePauseProgramMutation,
  useEndProgramMutation,
} from '@/services/queries/programs';
import type { Program, ProgramStatus, CreateProgramData, UpdateProgramData } from '@/services/api/types';

const statusColors: Record<ProgramStatus, string> = {
  draft: 'border-amber-500/50 text-amber-500 bg-amber-500/10',
  active: 'border-green-500/50 text-green-500 bg-green-500/10',
  paused: 'border-blue-500/50 text-blue-500 bg-blue-500/10',
  ended: 'border-muted-foreground/50 text-muted-foreground bg-muted/50',
};

export default function Programs() {
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [isFormOpen, setIsFormOpen] = useState(false);
  const [editingProgram, setEditingProgram] = useState<Program | null>(null);
  const [deletingProgram, setDeletingProgram] = useState<Program | null>(null);

  const { toast } = useToast();

  // Queries
  const { data: programsData, isLoading, error } = useProgramsQuery({
    search: searchQuery || undefined,
    status: statusFilter !== 'all' ? (statusFilter as ProgramStatus) : undefined,
  });

  // Mutations
  const createMutation = useCreateProgramMutation();
  const updateMutation = useUpdateProgramMutation();
  const deleteMutation = useDeleteProgramMutation();
  const activateMutation = useActivateProgramMutation();
  const pauseMutation = usePauseProgramMutation();
  const endMutation = useEndProgramMutation();

  const programs = programsData?.data || [];

  const handleCreate = () => {
    setEditingProgram(null);
    setIsFormOpen(true);
  };

  const handleEdit = (program: Program) => {
    setEditingProgram(program);
    setIsFormOpen(true);
  };

  const handleFormSubmit = async (data: CreateProgramData | UpdateProgramData) => {
    try {
      if (editingProgram) {
        await updateMutation.mutateAsync({ programId: editingProgram.id, data });
        toast({
          title: 'Program updated',
          description: `"${data.name || editingProgram.name}" has been updated successfully.`,
        });
      } else {
        await createMutation.mutateAsync(data as CreateProgramData);
        toast({
          title: 'Program created',
          description: `"${data.name}" has been created successfully.`,
        });
      }
      setIsFormOpen(false);
      setEditingProgram(null);
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Something went wrong',
        variant: 'destructive',
      });
    }
  };

  const handleDelete = async () => {
    if (!deletingProgram) return;
    try {
      await deleteMutation.mutateAsync(deletingProgram.id);
      toast({
        title: 'Program deleted',
        description: `"${deletingProgram.name}" has been deleted.`,
      });
      setDeletingProgram(null);
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to delete program',
        variant: 'destructive',
      });
    }
  };

  const handleActivate = async (program: Program) => {
    try {
      await activateMutation.mutateAsync(program.id);
      toast({
        title: 'Program activated',
        description: `"${program.name}" is now active.`,
      });
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to activate program',
        variant: 'destructive',
      });
    }
  };

  const handlePause = async (program: Program) => {
    try {
      await pauseMutation.mutateAsync(program.id);
      toast({
        title: 'Program paused',
        description: `"${program.name}" has been paused.`,
      });
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to pause program',
        variant: 'destructive',
      });
    }
  };

  const handleEnd = async (program: Program) => {
    try {
      await endMutation.mutateAsync(program.id);
      toast({
        title: 'Program ended',
        description: `"${program.name}" has been ended.`,
      });
    } catch (err) {
      toast({
        title: 'Error',
        description: err instanceof Error ? err.message : 'Failed to end program',
        variant: 'destructive',
      });
    }
  };

  const isAnyMutating =
    createMutation.isPending ||
    updateMutation.isPending ||
    activateMutation.isPending ||
    pauseMutation.isPending ||
    endMutation.isPending;

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">Programs</h1>
          <p className="text-muted-foreground mt-1">
            Manage your gamification programs and their settings.
          </p>
        </div>
        <Button variant="glow" onClick={handleCreate}>
          <Plus className="w-4 h-4" />
          Create Program
        </Button>
      </div>

      {/* Search and Filter */}
      <Card>
        <CardContent className="p-4">
          <div className="flex flex-col sm:flex-row gap-4">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder="Search programs..."
                className="pl-9"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-full sm:w-[180px]">
                <SelectValue placeholder="Filter by status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Status</SelectItem>
                <SelectItem value="draft">Draft</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="paused">Paused</SelectItem>
                <SelectItem value="ended">Ended</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Programs Table */}
      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Name</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Status</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Players</th>
                  <th className="text-left p-4 text-sm font-medium text-muted-foreground">Dates</th>
                  <th className="text-right p-4 text-sm font-medium text-muted-foreground">Actions</th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i} className="border-b border-border/50">
                      <td className="p-4">
                        <div className="flex items-center gap-3">
                          <Skeleton className="w-10 h-10 rounded-lg" />
                          <Skeleton className="h-4 w-32" />
                        </div>
                      </td>
                      <td className="p-4"><Skeleton className="h-5 w-16" /></td>
                      <td className="p-4"><Skeleton className="h-4 w-12" /></td>
                      <td className="p-4"><Skeleton className="h-4 w-24" /></td>
                      <td className="p-4"><Skeleton className="h-8 w-8 ml-auto" /></td>
                    </tr>
                  ))
                ) : error ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center">
                      <p className="text-destructive">Failed to load programs. Please try again.</p>
                    </td>
                  </tr>
                ) : programs.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="p-8 text-center">
                      <div className="flex flex-col items-center gap-3">
                        <div className="w-16 h-16 rounded-full bg-secondary flex items-center justify-center">
                          <FolderOpen className="w-8 h-8 text-muted-foreground" />
                        </div>
                        <div>
                          <p className="font-medium">No programs found</p>
                          <p className="text-sm text-muted-foreground">
                            Create your first program to get started.
                          </p>
                        </div>
                      </div>
                    </td>
                  </tr>
                ) : (
                  programs.map((program) => (
                    <tr
                      key={program.id}
                      className="border-b border-border/50 hover:bg-secondary/30 transition-colors"
                    >
                      <td className="p-4">
                        <div className="flex items-center gap-3">
                          <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
                            <FolderOpen className="w-5 h-5 text-primary" />
                          </div>
                          <div>
                            <span className="font-medium">{program.name}</span>
                            {program.description && (
                              <p className="text-sm text-muted-foreground truncate max-w-[200px]">
                                {program.description}
                              </p>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="p-4">
                        <Badge variant="outline" className={cn('capitalize', statusColors[program.status])}>
                          {program.status}
                        </Badge>
                      </td>
                      <td className="p-4 text-muted-foreground">
                        {program.player_count?.toLocaleString() || 0}
                      </td>
                      <td className="p-4 text-muted-foreground text-sm">
                        {program.start_date || program.end_date ? (
                          <span>
                            {program.start_date ? format(new Date(program.start_date), 'MMM d, yyyy') : '—'}
                            {' → '}
                            {program.end_date ? format(new Date(program.end_date), 'MMM d, yyyy') : '—'}
                          </span>
                        ) : (
                          <span className="text-muted-foreground/60">No dates set</span>
                        )}
                      </td>
                      <td className="p-4 text-right">
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" disabled={isAnyMutating}>
                              {isAnyMutating ? (
                                <Loader2 className="w-4 h-4 animate-spin" />
                              ) : (
                                <MoreHorizontal className="w-4 h-4" />
                              )}
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            {program.status === 'draft' && (
                              <DropdownMenuItem onClick={() => handleActivate(program)}>
                                <Play className="w-4 h-4 mr-2" />
                                Activate
                              </DropdownMenuItem>
                            )}
                            {program.status === 'active' && (
                              <DropdownMenuItem onClick={() => handlePause(program)}>
                                <Pause className="w-4 h-4 mr-2" />
                                Pause
                              </DropdownMenuItem>
                            )}
                            {program.status === 'paused' && (
                              <>
                                <DropdownMenuItem onClick={() => handleActivate(program)}>
                                  <Play className="w-4 h-4 mr-2" />
                                  Resume
                                </DropdownMenuItem>
                                <DropdownMenuItem onClick={() => handleEnd(program)}>
                                  <StopCircle className="w-4 h-4 mr-2" />
                                  End Program
                                </DropdownMenuItem>
                              </>
                            )}
                            <DropdownMenuSeparator />
                            <DropdownMenuItem onClick={() => handleEdit(program)}>
                              <Pencil className="w-4 h-4 mr-2" />
                              Edit
                            </DropdownMenuItem>
                            {program.status !== 'ended' && (
                              <DropdownMenuItem
                                className="text-destructive"
                                onClick={() => setDeletingProgram(program)}
                              >
                                <Trash2 className="w-4 h-4 mr-2" />
                                Delete
                              </DropdownMenuItem>
                            )}
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* Create/Edit Dialog */}
      <ProgramFormDialog
        open={isFormOpen}
        onOpenChange={setIsFormOpen}
        program={editingProgram}
        onSubmit={handleFormSubmit}
        isLoading={createMutation.isPending || updateMutation.isPending}
      />

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={!!deletingProgram} onOpenChange={() => setDeletingProgram(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Program</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete "{deletingProgram?.name}"? This action cannot be undone
              and will remove all associated data.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

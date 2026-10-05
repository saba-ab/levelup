import React from 'react';
import { MoreHorizontal, Pencil, Trash2, type LucideIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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

export interface ItemAction {
  label: string;
  icon?: LucideIcon;
  onClick: () => void;
  disabled?: boolean;
}

interface ItemActionsMenuProps {
  itemName: string;
  onEdit: () => void;
  onDelete: () => void;
  /** Extra entries shown above Edit (e.g. "Award to player"). */
  actions?: ItemAction[];
  showInGroup?: boolean;
}

export function ItemActionsMenu({ itemName, onEdit, onDelete, actions = [], showInGroup = false }: ItemActionsMenuProps) {
  const [showDeleteDialog, setShowDeleteDialog] = React.useState(false);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            aria-label={`Actions for ${itemName}`}
            className={showInGroup ? "opacity-0 group-hover:opacity-100 focus-visible:opacity-100 transition-opacity" : ""}
          >
            <MoreHorizontal className="w-4 h-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {actions.map(({ label, icon: Icon, onClick, disabled }) => (
            <DropdownMenuItem key={label} onClick={onClick} disabled={disabled}>
              {Icon && <Icon className="w-4 h-4 mr-2" />}
              {label}
            </DropdownMenuItem>
          ))}
          {actions.length > 0 && <DropdownMenuSeparator />}
          <DropdownMenuItem onClick={onEdit}>
            <Pencil className="w-4 h-4 mr-2" />
            Edit
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => setShowDeleteDialog(true)}
            className="text-destructive focus:text-destructive"
          >
            <Trash2 className="w-4 h-4 mr-2" />
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {itemName}?</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. This will permanently delete "{itemName}" and remove all associated data.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={onDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

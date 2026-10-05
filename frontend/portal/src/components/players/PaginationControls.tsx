import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

interface PaginationControlsProps {
  /** 1-based page number from useCursorPagination. */
  page: number;
  hasPrevious: boolean;
  hasNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
  /** Rows on the current page. */
  itemCount?: number;
  pageSize?: number;
  pageSizeOptions?: number[];
  onPageSizeChange?: (size: number) => void;
  itemLabel?: string;
}

/** Prev/next controls for cursor lists (there is no total count). */
export function PaginationControls({
  page,
  hasPrevious,
  hasNext,
  onPrevious,
  onNext,
  itemCount,
  pageSize,
  pageSizeOptions,
  onPageSizeChange,
  itemLabel = 'items',
}: PaginationControlsProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 p-4 border-t border-border">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        {pageSize !== undefined && pageSizeOptions && onPageSizeChange ? (
          <>
            <span>Per page</span>
            <Select value={pageSize.toString()} onValueChange={(value) => onPageSizeChange(Number(value))}>
              <SelectTrigger className="w-[70px] h-8">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pageSizeOptions.map((size) => (
                  <SelectItem key={size} value={size.toString()}>
                    {size}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </>
        ) : null}
        {itemCount !== undefined && (
          <span>
            {itemCount} {itemLabel} on this page
          </span>
        )}
      </div>

      <div className="flex items-center gap-2">
        <span className="text-sm text-muted-foreground">Page {page}</span>
        <div className="flex items-center gap-1">
          <Button
            variant="outline"
            size="icon"
            className="h-8 w-8"
            onClick={onPrevious}
            disabled={!hasPrevious}
            aria-label="Previous page"
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            className="h-8 w-8"
            onClick={onNext}
            disabled={!hasNext}
            aria-label="Next page"
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

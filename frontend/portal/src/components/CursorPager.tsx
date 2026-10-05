import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Button } from '@/components/ui/button';

interface CursorPagerProps {
  page: number;
  hasPrevious: boolean;
  /** next_cursor of the current page ("" on the last page). */
  nextCursor?: string;
  onPrevious: () => void;
  onNext: (cursor: string) => void;
  isFetching?: boolean;
}

/** Prev / Next controls for cursor lists (there is no total count: "Page N"). */
export default function CursorPager({ page, hasPrevious, nextCursor, onPrevious, onNext, isFetching }: CursorPagerProps) {
  if (!hasPrevious && !nextCursor) return null;
  return (
    <div className="flex items-center justify-end gap-2 p-4">
      <Button variant="outline" size="sm" disabled={!hasPrevious || isFetching} onClick={onPrevious}>
        <ChevronLeft className="w-4 h-4" />
        Previous
      </Button>
      <span className="text-sm text-muted-foreground px-2">Page {page}</span>
      <Button variant="outline" size="sm" disabled={!nextCursor || isFetching} onClick={() => nextCursor && onNext(nextCursor)}>
        Next
        <ChevronRight className="w-4 h-4" />
      </Button>
    </div>
  );
}

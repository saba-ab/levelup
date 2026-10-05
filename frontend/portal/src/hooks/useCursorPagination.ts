import { useCallback, useState } from 'react';

/**
 * Prev/next paging over a cursor API ({ data, next_cursor }). Cursors are
 * opaque and only go forward, so the hook keeps the cursor each visited
 * page started from; "previous" pops back to it. There is no total count:
 * show "Page N" and enable Next only when the last response had a cursor.
 *
 *   const pager = useCursorPagination(25);
 *   const { data } = usePlayersQuery({ limit: pager.limit, cursor: pager.cursor });
 *   <Button disabled={!pager.hasPrevious} onClick={pager.previous} />
 *   <Button disabled={!data?.next_cursor} onClick={() => pager.next(data!.next_cursor)} />
 */
export function useCursorPagination(limit = 25) {
  const [starts, setStarts] = useState<string[]>(['']);

  const next = useCallback((nextCursor: string) => {
    if (nextCursor) setStarts(s => [...s, nextCursor]);
  }, []);
  const previous = useCallback(() => setStarts(s => (s.length > 1 ? s.slice(0, -1) : s)), []);
  const reset = useCallback(() => setStarts(['']), []);

  return {
    limit,
    /** Undefined on the first page so it is omitted from the query string. */
    cursor: starts[starts.length - 1] || undefined,
    page: starts.length,
    hasPrevious: starts.length > 1,
    next,
    previous,
    reset,
  };
}

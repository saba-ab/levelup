import type { ApiResponse } from '@/hooks/useApi';
import type { CursorPage } from './models/common';

/**
 * Walks every page of a cursor list. For small catalogues the UI needs in
 * full (badges for a select, levels for a ladder), not for unbounded lists.
 * Stops at maxItems so a mistake cannot loop over an entire table.
 */
export async function fetchAllPages<T>(
  fetchPage: (cursor?: string) => Promise<ApiResponse<CursorPage<T>>>,
  maxItems = 1000,
): Promise<ApiResponse<CursorPage<T>>> {
  const all: T[] = [];
  let cursor: string | undefined;
  for (;;) {
    const res = await fetchPage(cursor);
    if (!res.success || !res.data) return res;
    all.push(...res.data.data);
    cursor = res.data.next_cursor || undefined;
    if (!cursor || all.length >= maxItems) {
      return { ...res, data: { data: all, next_cursor: cursor ?? '' } };
    }
  }
}

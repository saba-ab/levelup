import type { CursorPage } from "./types.js";

/**
 * One page of a cursor-paginated list.
 *
 * - `data` holds this page's items, `nextCursor` the cursor of the next page (null on the last page).
 * - `await page.nextPage()` fetches the following page (null when there is none).
 * - A Page is an async iterable over *all* items from this page onwards, fetching
 *   further pages lazily: `for await (const player of await client.players.list()) { ... }`.
 * - `page.pages()` iterates page by page instead.
 */
export class Page<T, B extends CursorPage<T> = CursorPage<T>> implements AsyncIterable<T> {
  /** The full response envelope (some lists carry extra fields, e.g. `period_start`). */
  readonly body: B;
  readonly data: T[];
  readonly nextCursor: string | null;

  readonly #fetchPage: (cursor: string) => Promise<Page<T, B>>;

  constructor(body: B, fetchPage: (cursor: string) => Promise<Page<T, B>>, requestedCursor?: string) {
    this.body = body;
    this.data = Array.isArray(body.data) ? body.data : [];
    // Guard against a server echoing the same cursor back, which would loop forever.
    const next = body.next_cursor || null;
    this.nextCursor = next !== null && next === requestedCursor ? null : next;
    this.#fetchPage = fetchPage;
  }

  hasNextPage(): boolean {
    return this.nextCursor !== null;
  }

  async nextPage(): Promise<Page<T, B> | null> {
    return this.nextCursor === null ? null : this.#fetchPage(this.nextCursor);
  }

  /** Iterates this page and every following page. */
  async *pages(): AsyncGenerator<Page<T, B>, void, undefined> {
    let page: Page<T, B> | null = this;
    while (page) {
      yield page;
      page = await page.nextPage();
    }
  }

  async *[Symbol.asyncIterator](): AsyncGenerator<T, void, undefined> {
    for await (const page of this.pages()) {
      yield* page.data;
    }
  }
}

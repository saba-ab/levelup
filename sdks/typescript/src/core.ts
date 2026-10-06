import { randomUUID } from "node:crypto";

import { LevelUpConnectionError, LevelUpError, LevelUpTimeoutError } from "./errors.js";
import { Page } from "./pagination.js";
import type { CursorPage } from "./types.js";
import { VERSION } from "./version.js";

export const DEFAULT_BASE_URL = "https://api.levelupos.ge";
export const API_PREFIX = "/api/v1";

/** Statuses that are retried (when the request is safe to retry). */
export const RETRYABLE_STATUSES: ReadonlySet<number> = new Set([429, 502, 503, 504]);

export interface ClientOptions {
  /** API key, `lvl_live_…`. Sent as `Authorization: Bearer <key>`. */
  apiKey: string;
  /** API origin. Default "https://api.levelupos.ge". The SDK appends "/api/v1". */
  baseUrl?: string;
  /** Per-attempt timeout in milliseconds. Default 30 000. */
  timeout?: number;
  /** Maximum retries after the first attempt. Default 2. Set 0 to disable. */
  retries?: number;
  /** Base delay of the exponential backoff in milliseconds. Default 500. */
  retryBackoff?: number;
  /** Upper bound for any single retry delay, including Retry-After, in milliseconds. Default 30 000. */
  maxRetryDelay?: number;
  /** Extra headers sent with every request. */
  headers?: Record<string, string>;
  /** Custom fetch implementation (defaults to the global fetch, Node >= 18). */
  fetch?: typeof fetch;
}

/** Per-call options accepted by every resource method as the last argument. */
export interface RequestOptions {
  /**
   * Idempotency-Key header. The server replays the stored response for a repeated key and
   * rejects a reused key with a different body. Requests carrying a key are retried on
   * 429/502/503/504 and network errors; other non-GET requests are never retried.
   */
  idempotencyKey?: string;
  /** Overrides the client timeout for this call (milliseconds). */
  timeout?: number;
  /** Overrides the client retry count for this call. */
  retries?: number;
  /** Abort the call (no retry follows an abort). */
  signal?: AbortSignal;
  /** Extra headers for this call. */
  headers?: Record<string, string>;
}

export type QueryValue = string | number | boolean | Date | null | undefined;
export type Query = Record<string, QueryValue>;

export interface RequestSpec {
  method: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  path: string;
  query?: object;
  body?: unknown;
  options?: RequestOptions;
}

/** Low-level transport shared by all resources. */
export class HttpClient {
  readonly baseUrl: string;
  readonly timeout: number;
  readonly retries: number;
  readonly retryBackoff: number;
  readonly maxRetryDelay: number;
  readonly #apiKey: string;
  readonly #headers: Record<string, string>;
  readonly #fetch: typeof fetch;

  constructor(options: ClientOptions) {
    if (!options || typeof options.apiKey !== "string" || options.apiKey.trim() === "") {
      throw new TypeError("LevelUp: `apiKey` is required");
    }
    this.#apiKey = options.apiKey.trim();
    this.baseUrl = (options.baseUrl ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
    this.timeout = options.timeout ?? 30_000;
    this.retries = Math.max(0, options.retries ?? 2);
    this.retryBackoff = Math.max(0, options.retryBackoff ?? 500);
    this.maxRetryDelay = Math.max(0, options.maxRetryDelay ?? 30_000);
    this.#headers = { ...(options.headers ?? {}) };
    const f = options.fetch ?? globalThis.fetch;
    if (typeof f !== "function") {
      throw new TypeError("LevelUp: global fetch is unavailable; use Node >= 18 or pass `fetch`");
    }
    this.#fetch = f;
  }

  async request<T>(spec: RequestSpec): Promise<T> {
    const opts = spec.options ?? {};
    const url = this.buildUrl(spec.path, spec.query);
    const headers: Record<string, string> = {
      Accept: "application/json, application/problem+json",
      "User-Agent": `levelup-typescript/${VERSION}`,
      ...this.#headers,
      ...(opts.headers ?? {}),
      Authorization: `Bearer ${this.#apiKey}`,
    };
    let payload: string | undefined;
    if (spec.body !== undefined) {
      headers["Content-Type"] = "application/json";
      payload = JSON.stringify(spec.body);
    }
    if (opts.idempotencyKey) {
      headers["Idempotency-Key"] = opts.idempotencyKey;
    }

    const safeToRetry = spec.method === "GET" || Boolean(opts.idempotencyKey);
    const maxRetries = safeToRetry ? Math.max(0, opts.retries ?? this.retries) : 0;
    const timeout = opts.timeout ?? this.timeout;

    for (let attempt = 0; ; attempt++) {
      const canRetry = attempt < maxRetries;
      let response: Response;
      let text: string;
      try {
        ({ response, text } = await this.#send(url, spec.method, headers, payload, timeout, opts.signal));
      } catch (err) {
        if (err instanceof LevelUpConnectionError && canRetry && !opts.signal?.aborted) {
          await sleep(this.#backoff(attempt));
          continue;
        }
        throw err;
      }

      if (response.ok) {
        return parseSuccess<T>(response, text);
      }
      if (canRetry && RETRYABLE_STATUSES.has(response.status)) {
        const retryAfter = parseRetryAfter(response.headers.get("Retry-After"));
        await sleep(retryAfter === null ? this.#backoff(attempt) : Math.min(retryAfter, this.maxRetryDelay));
        continue;
      }
      throw LevelUpError.fromResponse(response.status, response.headers, text);
    }
  }

  /** Fetches a cursor page and wires `nextPage()` to fetch the following ones with the same filters. */
  async page<T, B extends CursorPage<T> = CursorPage<T>>(
    path: string,
    query: object | undefined,
    options: RequestOptions | undefined,
  ): Promise<Page<T, B>> {
    const fetchPage = async (cursor: string | undefined): Promise<Page<T, B>> => {
      const q: Record<string, unknown> = { ...(query ?? {}) };
      if (cursor !== undefined) {
        q.cursor = cursor;
      }
      const body = await this.request<B>({ method: "GET", path, query: q, options });
      return new Page<T, B>(body, (next) => fetchPage(next), cursor);
    };
    return fetchPage((query as { cursor?: string } | undefined)?.cursor || undefined);
  }

  buildUrl(path: string, query?: object): string {
    let url = `${this.baseUrl}${API_PREFIX}${path}`;
    if (query) {
      const params = new URLSearchParams();
      for (const [key, value] of Object.entries(query)) {
        if (value === undefined || value === null || value === "") {
          continue;
        }
        params.append(key, value instanceof Date ? value.toISOString() : String(value));
      }
      const qs = params.toString();
      if (qs) {
        url += `?${qs}`;
      }
    }
    return url;
  }

  async #send(
    url: string,
    method: string,
    headers: Record<string, string>,
    body: string | undefined,
    timeout: number,
    signal: AbortSignal | undefined,
  ): Promise<{ response: Response; text: string }> {
    if (signal?.aborted) {
      throw new LevelUpConnectionError("request aborted", signal.reason, "aborted");
    }
    const controller = new AbortController();
    let timedOut = false;
    const timer = timeout > 0 ? setTimeout(() => ((timedOut = true), controller.abort()), timeout) : undefined;
    const onAbort = (): void => controller.abort();
    signal?.addEventListener("abort", onAbort, { once: true });
    try {
      const response = await this.#fetch(url, { method, headers, body, signal: controller.signal });
      // Read the body inside the timeout window so a stalled body also times out.
      const text = await response.text();
      return { response, text };
    } catch (err) {
      if (timedOut) {
        throw new LevelUpTimeoutError(`request timed out after ${timeout}ms`, err);
      }
      if (signal?.aborted) {
        throw new LevelUpConnectionError("request aborted", err, "aborted");
      }
      throw new LevelUpConnectionError(`network error: ${(err as Error)?.message ?? String(err)}`, err);
    } finally {
      if (timer !== undefined) {
        clearTimeout(timer);
      }
      signal?.removeEventListener("abort", onAbort);
    }
  }

  /** Exponential backoff with jitter: base * 2^attempt, scaled into [50%, 100%], capped. */
  #backoff(attempt: number): number {
    const exp = Math.min(this.maxRetryDelay, this.retryBackoff * 2 ** attempt);
    return Math.round(exp * (0.5 + Math.random() * 0.5));
  }
}

/** Idempotency key for money movements: the caller's, or a fresh UUIDv4. */
export function withIdempotencyKey(options: RequestOptions | undefined): RequestOptions {
  return { ...(options ?? {}), idempotencyKey: options?.idempotencyKey || randomUUID() };
}

/** Path segment encoder. */
export function seg(value: string): string {
  if (typeof value !== "string" || value === "") {
    throw new TypeError("LevelUp: path parameter must be a non-empty string");
  }
  return encodeURIComponent(value);
}

/** Parses Retry-After (delta-seconds or HTTP-date) into milliseconds. */
export function parseRetryAfter(value: string | null): number | null {
  if (value === null || value.trim() === "") {
    return null;
  }
  const trimmed = value.trim();
  if (/^\d+(\.\d+)?$/.test(trimmed)) {
    return Math.round(Number(trimmed) * 1000);
  }
  const date = Date.parse(trimmed);
  if (Number.isNaN(date)) {
    return null;
  }
  return Math.max(0, date - Date.now());
}

function parseSuccess<T>(response: Response, text: string): T {
  if (response.status === 204 || text.trim() === "") {
    return undefined as T;
  }
  try {
    return JSON.parse(text) as T;
  } catch (err) {
    throw new LevelUpError({
      status: response.status,
      code: "invalid_response",
      message: "LevelUp API returned a non-JSON success response",
      body: text,
      headers: response.headers,
      cause: err,
    });
  }
}

function sleep(ms: number): Promise<void> {
  return ms > 0 ? new Promise((resolve) => setTimeout(resolve, ms)) : Promise.resolve();
}

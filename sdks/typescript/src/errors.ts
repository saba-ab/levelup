import type { Problem } from "./types.js";

export interface LevelUpErrorInit {
  status: number;
  code: string;
  message?: string;
  title?: string;
  detail?: string;
  type?: string;
  fieldErrors?: Record<string, string>;
  traceId?: string;
  body?: unknown;
  headers?: Headers;
  cause?: unknown;
}

/**
 * Every failure surfaced by the SDK is a LevelUpError.
 *
 * For HTTP errors the fields come from the RFC 9457 problem+json body.
 * Branch on `code` (stable, snake_case), never on `detail` (human text).
 */
export class LevelUpError extends Error {
  /** HTTP status, or 0 when no response was received. */
  readonly status: number;
  /** Machine-readable error code, e.g. "insufficient_balance", "player_not_found". */
  readonly code: string;
  readonly title?: string;
  readonly detail?: string;
  readonly type?: string;
  /** Field-level validation messages keyed by field name (422 responses). */
  readonly fieldErrors: Record<string, string>;
  /** Server trace id: quote it when contacting support. */
  readonly traceId?: string;
  /** Parsed response body (or raw text when it was not JSON). */
  readonly body?: unknown;
  readonly headers?: Headers;

  constructor(init: LevelUpErrorInit) {
    super(init.message ?? defaultMessage(init), init.cause === undefined ? undefined : { cause: init.cause });
    this.name = "LevelUpError";
    this.status = init.status;
    this.code = init.code;
    this.title = init.title;
    this.detail = init.detail;
    this.type = init.type;
    this.fieldErrors = init.fieldErrors ?? {};
    this.traceId = init.traceId;
    this.body = init.body;
    this.headers = init.headers;
  }

  /** Builds the error for a non-2xx response. */
  static fromResponse(status: number, headers: Headers, rawBody: string): LevelUpError {
    let body: unknown = rawBody;
    let problem: Problem = {};
    if (rawBody) {
      try {
        body = JSON.parse(rawBody);
        if (body && typeof body === "object" && !Array.isArray(body)) {
          problem = body as Problem;
        }
      } catch {
        // Not JSON (e.g. a proxy's HTML error page): keep the text in `body`.
      }
    }
    return new LevelUpError({
      status,
      code: problem.code || codeFromTitle(problem.title) || fallbackCode(status),
      title: problem.title,
      detail: problem.detail,
      type: problem.type,
      fieldErrors: problem.errors,
      traceId: problem.trace_id,
      body,
      headers,
    });
  }
}

/** No HTTP response was received (DNS failure, connection reset, ...). `status` is 0. */
export class LevelUpConnectionError extends LevelUpError {
  constructor(message: string, cause?: unknown, code = "connection_error") {
    super({ status: 0, code, message, cause });
    this.name = "LevelUpConnectionError";
  }
}

/** The request did not complete within the configured timeout. `status` is 0, `code` is "timeout". */
export class LevelUpTimeoutError extends LevelUpConnectionError {
  constructor(message: string, cause?: unknown) {
    super(message, cause, "timeout");
    this.name = "LevelUpTimeoutError";
  }
}

export function isLevelUpError(err: unknown): err is LevelUpError {
  return err instanceof LevelUpError;
}

function defaultMessage(init: LevelUpErrorInit): string {
  const text = init.detail || init.title || "request failed";
  return init.status ? `LevelUp API error ${init.status} (${init.code}): ${text}` : `LevelUp API error (${init.code}): ${text}`;
}

/** Some platform responses (e.g. the rate limiter) carry a snake_case title but no code. */
function codeFromTitle(title: string | undefined): string | undefined {
  return title && /^[a-z][a-z0-9_]*$/.test(title) ? title : undefined;
}

function fallbackCode(status: number): string {
  switch (status) {
    case 400:
      return "bad_request";
    case 401:
      return "unauthenticated";
    case 403:
      return "permission_denied";
    case 404:
      return "not_found";
    case 409:
      return "conflict";
    case 422:
      return "invalid";
    case 429:
      return "rate_limited";
    default:
      return status >= 500 ? "server_error" : `http_${status}`;
  }
}

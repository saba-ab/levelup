import { useCallback, useMemo, useRef } from 'react';
import { useEnvironment } from '@/contexts/EnvironmentContext';
import { useToast } from '@/hooks/use-toast';
import { authStorage } from '@/lib/auth-storage';
import { AUTH_ENDPOINTS } from '@/lib/api-routes';
import type { ProblemDetails } from '@/services/api/models/common';

/** Field errors in the shape forms already render: field -> messages. */
export type ValidationErrors = Record<string, string[]>;

export interface ApiOptions extends Omit<RequestInit, 'body'> {
  skipAuth?: boolean;
  skipRetry?: boolean;
  maxRetries?: number;
  retryDelay?: number;
  showErrorToast?: boolean;
  body?: unknown;
  /**
   * Send an Idempotency-Key so the server applies the request at most once.
   * `true` generates a key per call (reused across this call's retries);
   * pass a string to make a user action idempotent across calls.
   * Non-GET requests are only retried when they carry a key.
   */
  idempotencyKey?: string | true;
}

export interface ApiResponse<T> {
  data: T | null;
  error: string | null;
  /** Machine-readable error code from problem+json, e.g. "insufficient_balance". */
  code: string | null;
  validationErrors: ValidationErrors | null;
  status: number;
  success: boolean;
  /** True when the server replayed a stored response for this Idempotency-Key. */
  replayed: boolean;
  traceId: string | null;
}

type RequestInterceptor = (config: RequestInit & { url: string }) => RequestInit & { url: string };
type ResponseInterceptor = <T>(response: ApiResponse<T>) => ApiResponse<T>;
type ErrorInterceptor = (error: Error, config: ApiOptions) => void;

const requestInterceptors: RequestInterceptor[] = [];
const responseInterceptors: ResponseInterceptor[] = [];
const errorInterceptors: ErrorInterceptor[] = [];

function register<T>(list: T[], item: T) {
  list.push(item);
  return () => {
    const i = list.indexOf(item);
    if (i > -1) list.splice(i, 1);
  };
}

export const apiInterceptors = {
  request: { use: (i: RequestInterceptor) => register(requestInterceptors, i) },
  response: { use: (i: ResponseInterceptor) => register(responseInterceptors, i) },
  error: { use: (i: ErrorInterceptor) => register(errorInterceptors, i) },
};

const DEFAULT_MAX_RETRIES = 3;
const DEFAULT_RETRY_DELAY = 1000;
const RETRYABLE_STATUS_CODES = [408, 429, 502, 503, 504];

const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));
const backoff = (attempt: number, base: number) => Math.min(base * 2 ** attempt, 30000);

function newKey(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function isProblem(body: unknown): body is ProblemDetails {
  return typeof body === 'object' && body !== null && 'status' in body && 'title' in body;
}

/** problem+json -> the fields the UI renders. */
export function parseProblem(body: unknown, fallback: string) {
  if (!isProblem(body)) {
    return { error: fallback, code: null as string | null, validationErrors: null as ValidationErrors | null, traceId: null as string | null };
  }
  const validationErrors: ValidationErrors | null = body.errors
    ? Object.fromEntries(Object.entries(body.errors).map(([field, msg]) => [field, [msg]]))
    : null;
  return {
    error: body.detail || body.title || fallback,
    code: body.code ?? null,
    validationErrors,
    traceId: body.trace_id ?? null,
  };
}

// ---- refresh-token rotation ----------------------------------------------

let refreshInFlight: Promise<boolean> | null = null;

/**
 * Exchanges the stored refresh token for a new token pair. Single-flight:
 * refresh tokens are single-use, so concurrent 401s share one refresh
 * instead of each burning (and invalidating) the token.
 */
export function refreshSession(baseUrl: string): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight;
  const refreshToken = authStorage.getRefreshToken();
  if (!refreshToken) return Promise.resolve(false);

  refreshInFlight = (async () => {
    try {
      const res = await fetch(`${baseUrl}${AUTH_ENDPOINTS.REFRESH}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
      if (!res.ok) return false;
      const session = await res.json();
      authStorage.setTokens(session.access_token, session.refresh_token);
      window.dispatchEvent(new CustomEvent('levelupos:session-refreshed', { detail: session }));
      return true;
    } catch {
      return false;
    } finally {
      refreshInFlight = null;
    }
  })();
  return refreshInFlight;
}

export function useApi() {
  const { getApiUrl, activeEnvironment, isProduction } = useEnvironment();
  const { toast } = useToast();
  const abortControllerRef = useRef<AbortController | null>(null);

  const request = useCallback(async <T = unknown>(
    path: string,
    options: ApiOptions = {}
  ): Promise<ApiResponse<T>> => {
    const {
      skipAuth = false,
      skipRetry = false,
      maxRetries = DEFAULT_MAX_RETRIES,
      retryDelay = DEFAULT_RETRY_DELAY,
      showErrorToast = true,
      idempotencyKey,
      body,
      ...fetchOptions
    } = options;

    const method = (fetchOptions.method || 'GET').toUpperCase();
    const key = idempotencyKey === true ? newKey() : idempotencyKey;
    // Retrying a write without a key could apply it twice.
    const mayRetry = !skipRetry && (method === 'GET' || !!key);

    const buildConfig = (): RequestInit & { url: string } => {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        Accept: 'application/json, application/problem+json',
        ...(fetchOptions.headers as Record<string, string>),
      };
      const token = skipAuth ? null : authStorage.getAccessToken();
      if (token) headers.Authorization = `Bearer ${token}`;
      if (key) headers['Idempotency-Key'] = key;

      let config: RequestInit & { url: string } = {
        ...fetchOptions,
        method,
        url: getApiUrl(path),
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
      };
      for (const interceptor of requestInterceptors) config = interceptor(config);
      return config;
    };

    const finish = (result: ApiResponse<T>) => {
      for (const interceptor of responseInterceptors) result = interceptor(result);
      return result;
    };

    const execute = async (attempt: number, refreshed: boolean): Promise<ApiResponse<T>> => {
      abortControllerRef.current = new AbortController();
      const config = buildConfig();
      try {
        const response = await fetch(config.url, { ...config, signal: abortControllerRef.current.signal });

        // Expired access token: rotate once, then replay the same request.
        if (response.status === 401 && !skipAuth && !refreshed && authStorage.getRefreshToken()) {
          if (await refreshSession(activeEnvironment.url)) return execute(attempt, true);
        }

        const replayed = response.headers.get('Idempotent-Replay') === 'true';
        if (response.ok) {
          let data: T | null = null;
          if (response.status !== 204 && response.headers.get('content-type')?.includes('json')) {
            data = await response.json();
          }
          return finish({ data, error: null, code: null, validationErrors: null, status: response.status, success: true, replayed, traceId: null });
        }

        let parsed = parseProblem(null, response.statusText || `HTTP Error ${response.status}`);
        try {
          parsed = parseProblem(await response.json(), parsed.error);
        } catch {
          // non-JSON error body: keep the status text
        }

        if (mayRetry && attempt < maxRetries && RETRYABLE_STATUS_CODES.includes(response.status)) {
          const retryAfter = response.headers.get('Retry-After');
          await sleep(retryAfter ? parseInt(retryAfter, 10) * 1000 : backoff(attempt, retryDelay));
          return execute(attempt + 1, refreshed);
        }

        if (showErrorToast) {
          toast({
            title: getErrorTitle(response.status),
            description: parsed.validationErrors
              ? Object.entries(parsed.validationErrors).map(([f, m]) => `${f}: ${m.join(', ')}`).join('\n')
              : parsed.error,
            variant: 'destructive',
          });
        }
        if (!isProduction && parsed.traceId) {
          console.error('[API Error]', { path, status: response.status, code: parsed.code, trace_id: parsed.traceId });
        }

        return finish({ data: null, ...parsed, status: response.status, success: false, replayed });
      } catch (err) {
        const error = err instanceof Error ? err : new Error('Unknown error');
        if (error.name === 'AbortError') {
          return { data: null, error: 'Request was cancelled', code: null, validationErrors: null, status: 0, success: false, replayed: false, traceId: null };
        }
        for (const interceptor of errorInterceptors) interceptor(error, options);

        if (mayRetry && attempt < maxRetries) {
          await sleep(backoff(attempt, retryDelay));
          return execute(attempt + 1, refreshed);
        }
        if (showErrorToast) {
          toast({
            title: 'Network Error',
            description: 'Unable to connect to the server. Please check your connection.',
            variant: 'destructive',
          });
        }
        return { data: null, error: error.message, code: null, validationErrors: null, status: 0, success: false, replayed: false, traceId: null };
      }
    };

    return execute(0, false);
  }, [getApiUrl, activeEnvironment, isProduction, toast]);

  const cancel = useCallback(() => {
    abortControllerRef.current?.abort();
  }, []);

  return useMemo(() => ({
    request,
    get: <T = unknown>(path: string, options?: ApiOptions) => request<T>(path, { ...options, method: 'GET' }),
    post: <T = unknown>(path: string, body?: unknown, options?: ApiOptions) => request<T>(path, { ...options, method: 'POST', body }),
    put: <T = unknown>(path: string, body?: unknown, options?: ApiOptions) => request<T>(path, { ...options, method: 'PUT', body }),
    patch: <T = unknown>(path: string, body?: unknown, options?: ApiOptions) => request<T>(path, { ...options, method: 'PATCH', body }),
    delete: <T = unknown>(path: string, options?: ApiOptions) => request<T>(path, { ...options, method: 'DELETE' }),
    cancel,
    baseUrl: activeEnvironment.url,
    environment: activeEnvironment,
    isProduction,
  }), [request, cancel, activeEnvironment, isProduction]);
}

function getErrorTitle(status: number): string {
  switch (status) {
    case 400: return 'Bad Request';
    case 401: return 'Unauthorized';
    case 403: return 'Forbidden';
    case 404: return 'Not Found';
    case 409: return 'Conflict';
    case 410: return 'Gone';
    case 422: return 'Validation Error';
    case 429: return 'Too Many Requests';
    case 500: return 'Server Error';
    case 502: return 'Bad Gateway';
    case 503: return 'Service Unavailable';
    default: return 'Error';
  }
}

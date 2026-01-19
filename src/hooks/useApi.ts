import { useCallback, useRef } from 'react';
import { useEnvironment } from '@/contexts/EnvironmentContext';
import { useToast } from '@/hooks/use-toast';

// Laravel validation error structure
interface LaravelValidationErrors {
  [field: string]: string[];
}

interface LaravelErrorResponse {
  message: string;
  errors?: LaravelValidationErrors;
  exception?: string;
  file?: string;
  line?: number;
}

interface ApiOptions extends Omit<RequestInit, 'body'> {
  skipAuth?: boolean;
  skipRetry?: boolean;
  maxRetries?: number;
  retryDelay?: number;
  showErrorToast?: boolean;
  body?: unknown;
}

interface ApiResponse<T> {
  data: T | null;
  error: string | null;
  validationErrors: LaravelValidationErrors | null;
  status: number;
  success: boolean;
}

type RequestInterceptor = (config: RequestInit & { url: string }) => RequestInit & { url: string };
type ResponseInterceptor = <T>(response: ApiResponse<T>) => ApiResponse<T>;
type ErrorInterceptor = (error: Error, config: ApiOptions) => void;

// Global interceptors storage
const requestInterceptors: RequestInterceptor[] = [];
const responseInterceptors: ResponseInterceptor[] = [];
const errorInterceptors: ErrorInterceptor[] = [];

// Add interceptors
export const apiInterceptors = {
  request: {
    use: (interceptor: RequestInterceptor) => {
      requestInterceptors.push(interceptor);
      return () => {
        const index = requestInterceptors.indexOf(interceptor);
        if (index > -1) requestInterceptors.splice(index, 1);
      };
    },
  },
  response: {
    use: (interceptor: ResponseInterceptor) => {
      responseInterceptors.push(interceptor);
      return () => {
        const index = responseInterceptors.indexOf(interceptor);
        if (index > -1) responseInterceptors.splice(index, 1);
      };
    },
  },
  error: {
    use: (interceptor: ErrorInterceptor) => {
      errorInterceptors.push(interceptor);
      return () => {
        const index = errorInterceptors.indexOf(interceptor);
        if (index > -1) errorInterceptors.splice(index, 1);
      };
    },
  },
};

// Retry configuration
const DEFAULT_MAX_RETRIES = 3;
const DEFAULT_RETRY_DELAY = 1000;
const RETRYABLE_STATUS_CODES = [408, 429, 500, 502, 503, 504];

// Helper to parse Laravel error response
function parseLaravelError(data: unknown): LaravelErrorResponse | null {
  if (typeof data === 'object' && data !== null && 'message' in data) {
    return data as LaravelErrorResponse;
  }
  return null;
}

// Format validation errors for display
function formatValidationErrors(errors: LaravelValidationErrors): string {
  return Object.entries(errors)
    .map(([field, messages]) => `${field}: ${messages.join(', ')}`)
    .join('\n');
}

// Sleep utility for retry delays
const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

// Calculate exponential backoff delay
function getRetryDelay(attempt: number, baseDelay: number): number {
  return Math.min(baseDelay * Math.pow(2, attempt), 30000); // Max 30 seconds
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
      body,
      ...fetchOptions
    } = options;

    const url = getApiUrl(path);

    // Get auth token from localStorage
    const token = !skipAuth ? localStorage.getItem('levelupos_token') : null;

    // Build headers
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'Accept': 'application/json',
      'X-Requested-With': 'XMLHttpRequest', // Laravel CSRF compatibility
      ...(fetchOptions.headers as Record<string, string>),
    };

    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    // Add environment header for debugging (non-production only)
    if (!isProduction) {
      headers['X-Environment'] = activeEnvironment.name;
    }

    // Build request config
    let config: RequestInit & { url: string } = {
      ...fetchOptions,
      url,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    };

    // Apply request interceptors
    for (const interceptor of requestInterceptors) {
      config = interceptor(config);
    }

    // Retry logic wrapper
    const executeRequest = async (attempt: number = 0): Promise<ApiResponse<T>> => {
      // Create new abort controller for this request
      abortControllerRef.current = new AbortController();

      try {
        const response = await fetch(config.url, {
          ...config,
          signal: abortControllerRef.current.signal,
        });

        // Handle different response statuses
        let data: T | null = null;
        let error: string | null = null;
        let validationErrors: LaravelValidationErrors | null = null;

        if (response.ok) {
          // Success response
          const contentType = response.headers.get('content-type');
          if (contentType?.includes('application/json')) {
            data = await response.json();
          }
        } else {
          // Error response - parse Laravel error format
          try {
            const errorData = await response.json();
            const laravelError = parseLaravelError(errorData);

            if (laravelError) {
              error = laravelError.message;
              validationErrors = laravelError.errors || null;

              // Show debug info in non-production
              if (!isProduction && laravelError.exception) {
                console.error('[API Error]', {
                  exception: laravelError.exception,
                  file: laravelError.file,
                  line: laravelError.line,
                });
              }
            } else {
              error = 'An unexpected error occurred';
            }
          } catch {
            error = response.statusText || `HTTP Error ${response.status}`;
          }

          // Check if we should retry
          if (!skipRetry && attempt < maxRetries && RETRYABLE_STATUS_CODES.includes(response.status)) {
            const delay = getRetryDelay(attempt, retryDelay);

            // Handle rate limiting (429) - check Retry-After header
            if (response.status === 429) {
              const retryAfter = response.headers.get('Retry-After');
              const waitTime = retryAfter ? parseInt(retryAfter, 10) * 1000 : delay;
              await sleep(waitTime);
            } else {
              await sleep(delay);
            }

            console.log(`[API] Retrying request (attempt ${attempt + 1}/${maxRetries}):`, path);
            return executeRequest(attempt + 1);
          }

          // Show error toast if enabled
          if (showErrorToast && error) {
            toast({
              title: getErrorTitle(response.status),
              description: validationErrors
                ? formatValidationErrors(validationErrors)
                : error,
              variant: 'destructive',
            });
          }
        }

        let result: ApiResponse<T> = {
          data,
          error,
          validationErrors,
          status: response.status,
          success: response.ok,
        };

        // Apply response interceptors
        for (const interceptor of responseInterceptors) {
          result = interceptor(result);
        }

        return result;
      } catch (err) {
        const error = err instanceof Error ? err : new Error('Unknown error');

        // Don't retry aborted requests
        if (error.name === 'AbortError') {
          return {
            data: null,
            error: 'Request was cancelled',
            validationErrors: null,
            status: 0,
            success: false,
          };
        }

        // Apply error interceptors
        for (const interceptor of errorInterceptors) {
          interceptor(error, options);
        }

        // Retry on network errors
        if (!skipRetry && attempt < maxRetries) {
          const delay = getRetryDelay(attempt, retryDelay);
          await sleep(delay);
          console.log(`[API] Retrying after network error (attempt ${attempt + 1}/${maxRetries}):`, path);
          return executeRequest(attempt + 1);
        }

        if (showErrorToast) {
          toast({
            title: 'Network Error',
            description: 'Unable to connect to the server. Please check your connection.',
            variant: 'destructive',
          });
        }

        return {
          data: null,
          error: error.message,
          validationErrors: null,
          status: 0,
          success: false,
        };
      }
    };

    return executeRequest();
  }, [getApiUrl, activeEnvironment, isProduction, toast]);

  // Cancel pending request
  const cancel = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
  }, []);

  // HTTP method shortcuts
  const get = useCallback(<T = unknown>(path: string, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'GET' });
  }, [request]);

  const post = useCallback(<T = unknown>(path: string, body?: unknown, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'POST', body });
  }, [request]);

  const put = useCallback(<T = unknown>(path: string, body?: unknown, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'PUT', body });
  }, [request]);

  const patch = useCallback(<T = unknown>(path: string, body?: unknown, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'PATCH', body });
  }, [request]);

  const del = useCallback(<T = unknown>(path: string, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'DELETE' });
  }, [request]);

  return {
    request,
    get,
    post,
    put,
    patch,
    delete: del,
    cancel,
    baseUrl: activeEnvironment.url,
    environment: activeEnvironment,
    isProduction,
  };
}

// Helper to get user-friendly error title
function getErrorTitle(status: number): string {
  switch (status) {
    case 400: return 'Bad Request';
    case 401: return 'Unauthorized';
    case 403: return 'Forbidden';
    case 404: return 'Not Found';
    case 419: return 'Session Expired'; // Laravel CSRF token mismatch
    case 422: return 'Validation Error';
    case 429: return 'Too Many Requests';
    case 500: return 'Server Error';
    case 502: return 'Bad Gateway';
    case 503: return 'Service Unavailable';
    default: return 'Error';
  }
}
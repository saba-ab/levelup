import { useCallback } from 'react';
import { useEnvironment } from '@/contexts/EnvironmentContext';

interface ApiOptions extends RequestInit {
  skipAuth?: boolean;
}

interface ApiResponse<T> {
  data: T | null;
  error: string | null;
  status: number;
}

export function useApi() {
  const { getApiUrl, activeEnvironment, isProduction } = useEnvironment();

  const request = useCallback(async <T = unknown>(
    path: string,
    options: ApiOptions = {}
  ): Promise<ApiResponse<T>> => {
    const { skipAuth = false, ...fetchOptions } = options;
    const url = getApiUrl(path);

    // Get auth token from localStorage if needed
    const token = !skipAuth ? localStorage.getItem('levelupos_token') : null;

    const headers: HeadersInit = {
      'Content-Type': 'application/json',
      ...fetchOptions.headers,
    };

    if (token) {
      (headers as Record<string, string>)['Authorization'] = `Bearer ${token}`;
    }

    // Add environment header for debugging
    if (!isProduction) {
      (headers as Record<string, string>)['X-Environment'] = activeEnvironment.name;
    }

    try {
      const response = await fetch(url, {
        ...fetchOptions,
        headers,
      });

      const data = response.ok ? await response.json() : null;
      const error = !response.ok ? await response.text() : null;

      return {
        data,
        error,
        status: response.status,
      };
    } catch (err) {
      return {
        data: null,
        error: err instanceof Error ? err.message : 'Network error',
        status: 0,
      };
    }
  }, [getApiUrl, activeEnvironment, isProduction]);

  const get = useCallback(<T = unknown>(path: string, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'GET' });
  }, [request]);

  const post = useCallback(<T = unknown>(path: string, body?: unknown, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'POST', body: JSON.stringify(body) });
  }, [request]);

  const put = useCallback(<T = unknown>(path: string, body?: unknown, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'PUT', body: JSON.stringify(body) });
  }, [request]);

  const patch = useCallback(<T = unknown>(path: string, body?: unknown, options?: ApiOptions) => {
    return request<T>(path, { ...options, method: 'PATCH', body: JSON.stringify(body) });
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
    baseUrl: activeEnvironment.url,
    environment: activeEnvironment,
    isProduction,
  };
}
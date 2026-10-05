import { useState, useEffect, useCallback, useRef } from 'react';
import { useEnvironment } from '@/contexts/EnvironmentContext';
import { HEALTH_ENDPOINTS } from '@/lib/api-routes';

/** GET /health: overall status plus one entry per backend module. */
export interface HealthResponse {
  status: string;
  modules?: Record<string, string>;
}

interface ConnectionStatus {
  isOnline: boolean;
  latency: number | null;
  lastChecked: Date | null;
  error: string | null;
  /** "ok" when every module is healthy; null until the first answer. */
  apiStatus: string | null;
  modules: Record<string, string>;
}

interface UseConnectionStatusOptions {
  /** ms between pings */
  pingInterval?: number;
  /** ms before a ping counts as failed */
  pingTimeout?: number;
  pingEndpoint?: string;
}

const DEFAULT_OPTIONS: Required<UseConnectionStatusOptions> = {
  pingInterval: 30000,
  pingTimeout: 5000,
  pingEndpoint: HEALTH_ENDPOINTS.CHECK,
};

/** Pings the active environment's /health and reports reachability, latency and module health. */
export function useConnectionStatus(options: UseConnectionStatusOptions = {}) {
  const { pingInterval, pingTimeout, pingEndpoint } = { ...DEFAULT_OPTIONS, ...options };
  const { baseUrl, activeEnvironment } = useEnvironment();

  const [status, setStatus] = useState<ConnectionStatus>({
    isOnline: true,
    latency: null,
    lastChecked: null,
    error: null,
    apiStatus: null,
    modules: {},
  });
  const [isChecking, setIsChecking] = useState(false);
  const inFlight = useRef<AbortController | null>(null);

  const check = useCallback(async () => {
    if (inFlight.current) return;
    const controller = new AbortController();
    inFlight.current = controller;
    setIsChecking(true);
    const timeout = setTimeout(() => controller.abort(), pingTimeout);
    const startTime = performance.now();

    try {
      const response = await fetch(`${baseUrl}${pingEndpoint}`, {
        method: 'GET',
        signal: controller.signal,
        headers: { Accept: 'application/json' },
      });
      const latency = Math.round(performance.now() - startTime);
      let body: HealthResponse | null = null;
      try {
        body = (await response.json()) as HealthResponse;
      } catch {
        // non-JSON body: reachability is still known from the status code
      }
      // /health answers 503 with a body when a module is down: reachable but degraded.
      const isOnline = response.status < 500 || !!body?.status;
      const apiStatus = body?.status ?? (response.ok ? 'ok' : null);
      const degraded = Object.entries(body?.modules ?? {}).filter(([, s]) => s !== 'ok').map(([m]) => m);
      setStatus({
        isOnline,
        latency,
        lastChecked: new Date(),
        error: !isOnline
          ? `Server error: ${response.status}`
          : degraded.length
            ? `Degraded: ${degraded.join(', ')}`
            : apiStatus && apiStatus !== 'ok'
              ? `API status: ${apiStatus}`
              : null,
        apiStatus,
        modules: body?.modules ?? {},
      });
    } catch (error) {
      const timedOut = error instanceof Error && error.name === 'AbortError';
      setStatus(prev => ({
        ...prev,
        isOnline: false,
        latency: null,
        lastChecked: new Date(),
        error: timedOut ? 'Health check timed out' : error instanceof Error ? error.message : 'Connection failed',
        apiStatus: null,
      }));
    } finally {
      clearTimeout(timeout);
      inFlight.current = null;
      setIsChecking(false);
    }
  }, [baseUrl, pingEndpoint, pingTimeout]);

  // Ping on mount, on environment change and every pingInterval.
  useEffect(() => {
    const first = setTimeout(check, 100);
    const interval = setInterval(check, pingInterval);
    return () => {
      clearTimeout(first);
      clearInterval(interval);
      inFlight.current?.abort();
      inFlight.current = null;
    };
  }, [check, pingInterval]);

  return {
    ...status,
    isChecking,
    refresh: check,
    environment: activeEnvironment,
  };
}

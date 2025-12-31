import { useState, useEffect, useCallback, useRef } from 'react';
import { useEnvironment } from '@/contexts/EnvironmentContext';

interface ConnectionStatus {
  isOnline: boolean;
  latency: number | null;
  lastChecked: Date | null;
  error: string | null;
}

interface UseConnectionStatusOptions {
  pingInterval?: number; // ms between pings
  pingTimeout?: number; // ms before considering request failed
  pingEndpoint?: string; // endpoint to ping
}

const DEFAULT_OPTIONS: UseConnectionStatusOptions = {
  pingInterval: 30000, // 30 seconds
  pingTimeout: 5000, // 5 seconds
  pingEndpoint: '/ping', // Laravel ping endpoint
};

export function useConnectionStatus(options: UseConnectionStatusOptions = {}) {
  const { pingInterval, pingTimeout, pingEndpoint } = { ...DEFAULT_OPTIONS, ...options };
  const { baseUrl, activeEnvironment } = useEnvironment();

  const [status, setStatus] = useState<ConnectionStatus>({
    isOnline: true, // Assume online initially
    latency: null,
    lastChecked: null,
    error: null,
  });

  const [isChecking, setIsChecking] = useState(false);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isCheckingRef = useRef(false); // Use ref to prevent concurrent checks without causing re-renders

  // Store latest values in refs to avoid dependency issues
  const baseUrlRef = useRef(baseUrl);
  const pingEndpointRef = useRef(pingEndpoint);

  // Update refs when values change
  useEffect(() => {
    baseUrlRef.current = baseUrl;
    pingEndpointRef.current = pingEndpoint;
  }, [baseUrl, pingEndpoint]);

  // Set up periodic ping
  useEffect(() => {
    // Clear any existing interval first
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }

    // Define checkConnection inside effect to avoid dependency issues
    const checkConnection = async () => {
      // Prevent concurrent checks using ref
      if (isCheckingRef.current) return;
      isCheckingRef.current = true;
      setIsChecking(true);

      // Abort any pending request
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      abortControllerRef.current = new AbortController();
      const startTime = performance.now();

      try {
        const response = await fetch(`${baseUrlRef.current}${pingEndpointRef.current}`, {
          method: 'GET',
          signal: abortControllerRef.current.signal,
          headers: {
            'Accept': 'application/json',
            'X-Requested-With': 'XMLHttpRequest',
          },
          // Don't follow redirects for health checks
          redirect: 'error',
        });

        const endTime = performance.now();
        const latency = Math.round(endTime - startTime);

        // Consider any 2xx or even 401/403 as "online" (server is responding)
        const isOnline = response.status < 500;

        setStatus({
          isOnline,
          latency,
          lastChecked: new Date(),
          error: isOnline ? null : `Server error: ${response.status}`,
        });
      } catch (error) {
        // Don't update status for aborted requests
        if (error instanceof Error && error.name === 'AbortError') {
          isCheckingRef.current = false;
          setIsChecking(false);
          return;
        }

        setStatus({
          isOnline: false,
          latency: null,
          lastChecked: new Date(),
          error: error instanceof Error ? error.message : 'Connection failed',
        });
      } finally {
        isCheckingRef.current = false;
        setIsChecking(false);
      }
    };

    // Initial check
    checkConnection();

    // Set up interval
    intervalRef.current = setInterval(() => {
      checkConnection();
    }, pingInterval);

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
        abortControllerRef.current = null;
      }
    };
  }, [pingInterval, baseUrl, pingEndpoint]); // Re-setup when these change

  // Check connection when environment changes (separate effect to avoid conflicts)
  useEffect(() => {
    // Small delay to ensure previous cleanup is complete
    const timeoutId = setTimeout(() => {
      // Trigger a check by clearing and resetting the interval
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
      // The main effect will handle the re-setup
      const checkConnection = async () => {
        if (isCheckingRef.current) return;
        isCheckingRef.current = true;
        setIsChecking(true);

        if (abortControllerRef.current) {
          abortControllerRef.current.abort();
        }

        abortControllerRef.current = new AbortController();
        const startTime = performance.now();

        try {
          const response = await fetch(`${baseUrlRef.current}${pingEndpointRef.current}`, {
            method: 'GET',
            signal: abortControllerRef.current.signal,
            headers: {
              'Accept': 'application/json',
              'X-Requested-With': 'XMLHttpRequest',
            },
            redirect: 'error',
          });

          const endTime = performance.now();
          const latency = Math.round(endTime - startTime);
          const isOnline = response.status < 500;

          setStatus({
            isOnline,
            latency,
            lastChecked: new Date(),
            error: isOnline ? null : `Server error: ${response.status}`,
          });
        } catch (error) {
          if (error instanceof Error && error.name === 'AbortError') {
            isCheckingRef.current = false;
            setIsChecking(false);
            return;
          }

          setStatus({
            isOnline: false,
            latency: null,
            lastChecked: new Date(),
            error: error instanceof Error ? error.message : 'Connection failed',
          });
        } finally {
          isCheckingRef.current = false;
          setIsChecking(false);
        }
      };
      checkConnection();
    }, 100);

    return () => clearTimeout(timeoutId);
  }, [activeEnvironment.id, baseUrl]);

  // Manual refresh function
  const refresh = useCallback(() => {
    // Trigger check by temporarily clearing interval
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }

    const checkConnection = async () => {
      if (isCheckingRef.current) return;
      isCheckingRef.current = true;
      setIsChecking(true);

      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }

      abortControllerRef.current = new AbortController();
      const startTime = performance.now();

      try {
        const response = await fetch(`${baseUrlRef.current}${pingEndpointRef.current}`, {
          method: 'GET',
          signal: abortControllerRef.current.signal,
          headers: {
            'Accept': 'application/json',
            'X-Requested-With': 'XMLHttpRequest',
          },
          redirect: 'error',
        });

        const endTime = performance.now();
        const latency = Math.round(endTime - startTime);
        const isOnline = response.status < 500;

        setStatus({
          isOnline,
          latency,
          lastChecked: new Date(),
          error: isOnline ? null : `Server error: ${response.status}`,
        });
      } catch (error) {
        if (error instanceof Error && error.name === 'AbortError') {
          isCheckingRef.current = false;
          setIsChecking(false);
          return;
        }

        setStatus({
          isOnline: false,
          latency: null,
          lastChecked: new Date(),
          error: error instanceof Error ? error.message : 'Connection failed',
        });
      } finally {
        isCheckingRef.current = false;
        setIsChecking(false);
      }
    };

    checkConnection();

    // Restart interval
    intervalRef.current = setInterval(() => {
      checkConnection();
    }, pingInterval);
  }, [pingInterval]);

  return {
    ...status,
    isChecking,
    refresh,
    environment: activeEnvironment,
  };
}
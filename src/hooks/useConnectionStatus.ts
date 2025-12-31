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
  pingEndpoint: '/api/v1/ping', // Laravel ping endpoint
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

  const checkConnection = useCallback(async () => {
    if (isChecking) return;
    
    setIsChecking(true);
    
    // Abort any pending request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    
    abortControllerRef.current = new AbortController();
    const startTime = performance.now();
    
    try {
      const response = await fetch(`${baseUrl}${pingEndpoint}`, {
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
      setIsChecking(false);
    }
  }, [baseUrl, pingEndpoint, isChecking]);

  // Check connection when environment changes
  useEffect(() => {
    checkConnection();
  }, [activeEnvironment.id]);

  // Set up periodic ping
  useEffect(() => {
    // Initial check
    checkConnection();
    
    // Set up interval
    intervalRef.current = setInterval(checkConnection, pingInterval);
    
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, [pingInterval, checkConnection]);

  // Manual refresh function
  const refresh = useCallback(() => {
    checkConnection();
  }, [checkConnection]);

  return {
    ...status,
    isChecking,
    refresh,
    environment: activeEnvironment,
  };
}
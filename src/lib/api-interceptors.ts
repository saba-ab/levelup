import { apiInterceptors } from '@/hooks/useApi';

/**
 * Setup default API interceptors for the application.
 * Call this once during app initialization.
 */
export function setupApiInterceptors() {
  // Request interceptor: Add CSRF token for Laravel Sanctum
  apiInterceptors.request.use((config) => {
    const csrfToken = document.querySelector('meta[name="csrf-token"]')?.getAttribute('content');
    if (csrfToken) {
      (config.headers as Record<string, string>)['X-CSRF-TOKEN'] = csrfToken;
    }
    return config;
  });

  // Request interceptor: Add request timestamp for debugging
  apiInterceptors.request.use((config) => {
    (config.headers as Record<string, string>)['X-Request-Start'] = Date.now().toString();
    return config;
  });

  // Response interceptor: Handle token expiration (401)
  apiInterceptors.response.use((response) => {
    if (response.status === 401) {
      // Clear stored credentials
      localStorage.removeItem('levelupos_token');
      localStorage.removeItem('levelupos_user');
      
      // Redirect to login (if not already there)
      if (window.location.pathname !== '/login') {
        window.location.href = '/login?expired=true';
      }
    }
    return response;
  });

  // Response interceptor: Handle Laravel session expired (419)
  apiInterceptors.response.use((response) => {
    if (response.status === 419) {
      // CSRF token mismatch - reload page to get fresh token
      console.warn('[API] CSRF token expired, reloading page...');
      window.location.reload();
    }
    return response;
  });

  // Error interceptor: Log errors in development
  apiInterceptors.error.use((error, config) => {
    if (import.meta.env.DEV) {
      console.error('[API Error]', {
        error: error.message,
        config,
        timestamp: new Date().toISOString(),
      });
    }
  });

  // Response interceptor: Log slow requests
  apiInterceptors.response.use((response) => {
    if (import.meta.env.DEV) {
      // Check if response was slow (> 2 seconds)
      const startTime = parseInt(
        (response as unknown as { headers?: Record<string, string> }).headers?.['X-Request-Start'] || '0'
      );
      if (startTime) {
        const duration = Date.now() - startTime;
        if (duration > 2000) {
          console.warn(`[API] Slow request detected: ${duration}ms`);
        }
      }
    }
    return response;
  });

  console.log('[API] Interceptors initialized');
}

/**
 * Example usage of custom interceptors:
 * 
 * // Add auth header dynamically
 * const removeAuthInterceptor = apiInterceptors.request.use((config) => {
 *   const token = getAuthToken();
 *   if (token) {
 *     config.headers['Authorization'] = `Bearer ${token}`;
 *   }
 *   return config;
 * });
 * 
 * // Later, to remove the interceptor:
 * removeAuthInterceptor();
 * 
 * // Handle specific errors globally
 * apiInterceptors.error.use((error, config) => {
 *   if (error.message.includes('Network Error')) {
 *     showOfflineNotification();
 *   }
 * });
 */
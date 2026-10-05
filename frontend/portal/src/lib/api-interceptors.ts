import { apiInterceptors } from '@/hooks/useApi';
import { authStorage } from '@/lib/auth-storage';

/**
 * Default API interceptors. Call once during app initialization.
 *
 * The Go API authenticates with bearer tokens only (no cookies, no CSRF).
 * An expired access token is refreshed transparently inside useApi; a 401
 * that still reaches here means the session is really over.
 */
export function setupApiInterceptors() {
  apiInterceptors.response.use((response) => {
    if (response.status === 401 && authStorage.getAccessToken()) {
      authStorage.clear();
      if (window.location.pathname !== '/login') {
        window.location.href = '/login?expired=true';
      }
    }
    return response;
  });

  apiInterceptors.error.use((error, config) => {
    if (import.meta.env.DEV) {
      console.error('[API Error]', { error: error.message, config, timestamp: new Date().toISOString() });
    }
  });
}

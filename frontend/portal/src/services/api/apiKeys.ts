import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import type { ApiKey, CreateApiKeyData, CreatedApiKey, CursorPage, CursorParams, ID } from './types';
import { API_KEY_ENDPOINTS, toQuery } from '@/lib/api-routes';

const QUIET = { showErrorToast: false } as const;

/** API keys for the tenant's backends (identity module, admin only). */
export function useApiKeysService() {
  const api = useApi();

  const listApiKeys = useCallback(
    (params?: CursorParams) => api.get<CursorPage<ApiKey>>(`${API_KEY_ENDPOINTS.LIST}${toQuery(params)}`),
    [api],
  );
  const createApiKey = useCallback(
    (data: CreateApiKeyData) => api.post<CreatedApiKey>(API_KEY_ENDPOINTS.CREATE, data, QUIET),
    [api],
  );
  const revokeApiKey = useCallback((id: ID) => api.delete<void>(API_KEY_ENDPOINTS.REVOKE(id), QUIET), [api]);

  return useMemo(() => ({ listApiKeys, createApiKey, revokeApiKey }), [listApiKeys, createApiKey, revokeApiKey]);
}

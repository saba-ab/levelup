import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useApiKeysService } from '../api/apiKeys';
import type { CreateApiKeyData, CursorParams, ID } from '../api/types';
import { unwrap } from './rules';

const apiKeyKeys = {
  all: ['api-keys'] as const,
  list: (params?: CursorParams) => ['api-keys', 'list', params] as const,
};

export function useApiKeysQuery(params?: CursorParams) {
  const { listApiKeys } = useApiKeysService();
  return useQuery({
    queryKey: apiKeyKeys.list(params),
    queryFn: async () => unwrap(await listApiKeys(params), 'Failed to load API keys'),
    placeholderData: keepPreviousData,
  });
}

export function useCreateApiKeyMutation() {
  const queryClient = useQueryClient();
  const { createApiKey } = useApiKeysService();
  return useMutation({
    mutationFn: async (data: CreateApiKeyData) => unwrap(await createApiKey(data), 'Failed to create API key'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: apiKeyKeys.all }),
  });
}

export function useRevokeApiKeyMutation() {
  const queryClient = useQueryClient();
  const { revokeApiKey } = useApiKeysService();
  return useMutation({
    mutationFn: async (id: ID) => unwrap(await revokeApiKey(id), 'Failed to revoke API key'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: apiKeyKeys.all }),
  });
}

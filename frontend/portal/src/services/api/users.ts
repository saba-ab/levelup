import { useApi } from '@/hooks/useApi';
import { useCallback } from 'react';
import {
  User,
  CreateUserData,
  UpdateUserData,
  PaginatedResponse,
  PaginationParams,
} from './types';
import { USER_ENDPOINTS } from '@/lib/api-routes';

export function useUsersService() {
  const api = useApi();

  const listUsers = useCallback(async (filters?: PaginationParams) => {
    const params = new URLSearchParams();
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          params.append(key, String(value));
        }
      });
    }
    const query = params.toString();
    return api.get<PaginatedResponse<User>>(`${USER_ENDPOINTS.LIST}${query ? `?${query}` : ''}`);
  }, [api]);

  const getUser = useCallback(async (userId: number) => {
    return api.get<User>(USER_ENDPOINTS.SHOW(userId));
  }, [api]);

  const createUser = useCallback(async (data: CreateUserData) => {
    return api.post<User>(USER_ENDPOINTS.CREATE, data);
  }, [api]);

  const updateUser = useCallback(async (userId: number, data: UpdateUserData) => {
    return api.put<User>(USER_ENDPOINTS.UPDATE(userId), data);
  }, [api]);

  const deleteUser = useCallback(async (userId: number) => {
    return api.delete(USER_ENDPOINTS.DELETE(userId));
  }, [api]);

  return {
    listUsers,
    getUser,
    createUser,
    updateUser,
    deleteUser,
  };
}

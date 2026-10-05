import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import type { User, CreateUserData, UpdateUserData, UserFilters, CursorPage, ID } from './types';
import { USER_ENDPOINTS, toQuery } from '@/lib/api-routes';

/** Writes report errors to the caller, not as toasts. */
const QUIET = { showErrorToast: false } as const;

/** Tenant team members (identity module). */
export function useUsersService() {
  const api = useApi();

  const listUsers = useCallback(
    (filters?: UserFilters) => api.get<CursorPage<User>>(`${USER_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );

  const getUser = useCallback((userId: ID) => api.get<User>(USER_ENDPOINTS.SHOW(userId)), [api]);

  const createUser = useCallback((data: CreateUserData) => api.post<User>(USER_ENDPOINTS.CREATE, data, QUIET), [api]);

  const updateUser = useCallback(
    (userId: ID, data: UpdateUserData) => api.patch<User>(USER_ENDPOINTS.UPDATE(userId), data, QUIET),
    [api],
  );

  const deleteUser = useCallback((userId: ID) => api.delete<void>(USER_ENDPOINTS.DELETE(userId), QUIET), [api]);

  /** Replaces the user's roles; role ids come from ROLE_IDS. */
  const assignRoles = useCallback(
    (userId: ID, roleIds: number[]) => api.put<User>(USER_ENDPOINTS.ROLES(userId), { role_ids: roleIds }, QUIET),
    [api],
  );

  return useMemo(
    () => ({ listUsers, getUser, createUser, updateUser, deleteUser, assignRoles }),
    [listUsers, getUser, createUser, updateUser, deleteUser, assignRoles],
  );
}

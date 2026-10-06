import { useApi } from '@/hooks/useApi';
import { useCallback, useMemo } from 'react';
import type { User, CreateUserData, UpdateUserData, UserFilters, CursorPage, ID } from './types';
import { API_VERSION, USER_ENDPOINTS, toQuery } from '@/lib/api-routes';
import type { CreateInvitationData, Invitation, InvitationFilters } from './models/identity';

/** Tenant invitations; need users.create and a signed-in user (not an API key). */
export const INVITATION_ENDPOINTS = {
  LIST: `${API_VERSION}/users/invitations`,
  CREATE: `${API_VERSION}/users/invitations`,
  REVOKE: (id: ID) => `${API_VERSION}/users/invitations/${id}`,
} as const;

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

  /** Open invitations (pending and expired), newest first. */
  const listInvitations = useCallback(
    (filters?: InvitationFilters) =>
      api.get<CursorPage<Invitation>>(`${INVITATION_ENDPOINTS.LIST}${toQuery(filters)}`),
    [api],
  );

  /** Emails an accept link valid 7 days; re-inviting an address revokes its earlier invitation. 409 email_taken. */
  const createInvitation = useCallback(
    (data: CreateInvitationData) => api.post<Invitation>(INVITATION_ENDPOINTS.CREATE, data, QUIET),
    [api],
  );

  /** Idempotent; 409 invitation_already_accepted. */
  const revokeInvitation = useCallback(
    (invitationId: ID) => api.delete<void>(INVITATION_ENDPOINTS.REVOKE(invitationId), QUIET),
    [api],
  );

  return useMemo(
    () => ({
      listUsers,
      getUser,
      createUser,
      updateUser,
      deleteUser,
      assignRoles,
      listInvitations,
      createInvitation,
      revokeInvitation,
    }),
    [listUsers, getUser, createUser, updateUser, deleteUser, assignRoles, listInvitations, createInvitation, revokeInvitation],
  );
}

import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useUsersService } from '../api/users';
import { queryKeys } from './keys';
import type { ID, CreateUserData, UpdateUserData, UserFilters } from '../api/types';
import type { CreateInvitationData, InvitationFilters } from '../api/models/identity';
import { unwrap } from './rules';
import { useAuthService } from '../api/auth';

export const invitationKeys = {
  all: ['invitations'] as const,
  lists: () => [...invitationKeys.all, 'list'] as const,
  list: (filters?: InvitationFilters) => [...invitationKeys.lists(), filters] as const,
  preview: (token: string) => [...invitationKeys.all, 'preview', token] as const,
};

/** The signed-in user's profile from GET /auth/me (email_verified_at lives here). */
export const meKeys = {
  all: ['auth', 'me'] as const,
};

/** One cursor page of team members: { data, next_cursor }. */
export function useUsersQuery(filters?: UserFilters) {
  const { listUsers } = useUsersService();
  return useQuery({
    queryKey: queryKeys.users.list(filters),
    queryFn: async () => unwrap(await listUsers(filters), 'Failed to fetch users'),
    placeholderData: keepPreviousData,
  });
}

export function useUserQuery(userId: ID | undefined) {
  const { getUser } = useUsersService();
  return useQuery({
    queryKey: queryKeys.users.detail(userId ?? ''),
    queryFn: async () => unwrap(await getUser(userId!), 'Failed to fetch user'),
    enabled: !!userId,
  });
}

export function useCreateUserMutation() {
  const queryClient = useQueryClient();
  const { createUser } = useUsersService();
  return useMutation({
    mutationFn: async (data: CreateUserData) => unwrap(await createUser(data), 'Failed to create user'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users.all }),
  });
}

/** PATCH with only the changed fields. */
export function useUpdateUserMutation() {
  const queryClient = useQueryClient();
  const { updateUser } = useUsersService();
  return useMutation({
    mutationFn: async ({ userId, data }: { userId: ID; data: UpdateUserData }) =>
      unwrap(await updateUser(userId, data), 'Failed to update user'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users.all }),
  });
}

/** PUT /users/{id}/roles: replaces the user's role set. */
export function useAssignUserRolesMutation() {
  const queryClient = useQueryClient();
  const { assignRoles } = useUsersService();
  return useMutation({
    mutationFn: async ({ userId, roleIds }: { userId: ID; roleIds: number[] }) =>
      unwrap(await assignRoles(userId, roleIds), 'Failed to update roles'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users.all }),
  });
}

export function useDeleteUserMutation() {
  const queryClient = useQueryClient();
  const { deleteUser } = useUsersService();
  return useMutation({
    mutationFn: async (userId: ID) => {
      unwrap(await deleteUser(userId), 'Failed to delete user');
      return userId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users.all }),
  });
}

// ==================== INVITATIONS ====================

/** One cursor page of open invitations: { data, next_cursor }. */
export function useInvitationsQuery(filters?: InvitationFilters, enabled = true) {
  const { listInvitations } = useUsersService();
  return useQuery({
    queryKey: invitationKeys.list(filters),
    queryFn: async () => unwrap(await listInvitations(filters), 'Failed to fetch invitations'),
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function useCreateInvitationMutation() {
  const queryClient = useQueryClient();
  const { createInvitation } = useUsersService();
  return useMutation({
    mutationFn: async (data: CreateInvitationData) => unwrap(await createInvitation(data), 'Failed to send invitation'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: invitationKeys.all }),
  });
}

export function useRevokeInvitationMutation() {
  const queryClient = useQueryClient();
  const { revokeInvitation } = useUsersService();
  return useMutation({
    mutationFn: async (invitationId: ID) => {
      unwrap(await revokeInvitation(invitationId), 'Failed to revoke invitation');
      return invitationId;
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: invitationKeys.all }),
  });
}

/** Anonymous preview for the accept-invite page; 404 means invalid, used or expired. */
export function useInvitationPreviewQuery(token: string | null) {
  const { previewInvitation } = useAuthService();
  return useQuery({
    queryKey: invitationKeys.preview(token ?? ''),
    queryFn: async () => unwrap(await previewInvitation(token!), 'Invitation not found'),
    enabled: !!token,
    retry: false,
  });
}

// ==================== EMAIL VERIFICATION ====================

/** GET /auth/me, for fields AuthContext does not keep (email_verified_at). */
export function useMeQuery(enabled = true) {
  const { me } = useAuthService();
  return useQuery({
    queryKey: meKeys.all,
    queryFn: async () => unwrap(await me(), 'Failed to fetch your profile'),
    enabled,
    staleTime: 60_000,
  });
}

export function useResendVerificationMutation() {
  const { resendVerification } = useAuthService();
  return useMutation({
    mutationFn: async () => {
      unwrap(await resendVerification(), 'Failed to resend the verification email');
    },
  });
}

import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useUsersService } from '../api/users';
import { queryKeys } from './keys';
import type { ID, CreateUserData, UpdateUserData, UserFilters } from '../api/types';
import { unwrap } from './rules';

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

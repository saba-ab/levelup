import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useUsersService } from '../api/users';
import { queryKeys } from './keys';
import {
  User,
  CreateUserData,
  UpdateUserData,
  PaginationParams,
} from '../api/types';

export function useUsersQuery(filters?: PaginationParams) {
  const { listUsers } = useUsersService();

  return useQuery({
    queryKey: queryKeys.users.list(filters),
    queryFn: async () => {
      const response = await listUsers(filters);
      if (!response.success) throw new Error(response.error || 'Failed to fetch users');
      return response.data!;
    },
  });
}

export function useUserQuery(userId: number) {
  const { getUser } = useUsersService();

  return useQuery({
    queryKey: queryKeys.users.detail(userId),
    queryFn: async () => {
      const response = await getUser(userId);
      if (!response.success) throw new Error(response.error || 'Failed to fetch user');
      return response.data!;
    },
    enabled: !!userId,
  });
}

export function useCreateUserMutation() {
  const queryClient = useQueryClient();
  const { createUser } = useUsersService();

  return useMutation({
    mutationFn: async (data: CreateUserData) => {
      const response = await createUser(data);
      if (!response.success) throw new Error(response.error || 'Failed to create user');
      return response.data!;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.users.lists() });
    },
  });
}

export function useUpdateUserMutation() {
  const queryClient = useQueryClient();
  const { updateUser } = useUsersService();

  return useMutation({
    mutationFn: async ({ userId, data }: { userId: number; data: UpdateUserData }) => {
      const response = await updateUser(userId, data);
      if (!response.success) throw new Error(response.error || 'Failed to update user');
      return response.data!;
    },
    onMutate: async ({ userId, data }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.users.detail(userId) });
      const previousUser = queryClient.getQueryData<User>(queryKeys.users.detail(userId));

      if (previousUser) {
        queryClient.setQueryData<User>(queryKeys.users.detail(userId), { ...previousUser, ...data });
      }

      return { previousUser };
    },
    onError: (err, { userId }, context) => {
      if (context?.previousUser) {
        queryClient.setQueryData(queryKeys.users.detail(userId), context.previousUser);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.users.all });
    },
  });
}

export function useDeleteUserMutation() {
  const queryClient = useQueryClient();
  const { deleteUser } = useUsersService();

  return useMutation({
    mutationFn: async (userId: number) => {
      const response = await deleteUser(userId);
      if (!response.success) throw new Error(response.error || 'Failed to delete user');
      return userId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.users.all });
    },
  });
}

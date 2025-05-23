import {
  deleteFetcher,
  getFetcher,
  postFetcher,
  patchFetcher
} from '@/lib/fetcher'
import { CreateUserType } from '@/types/users-type'
import {
  keepPreviousData,
  useMutation,
  UseMutationOptions,
  useQuery,
  useQueryClient
} from '@tanstack/react-query'

type UseUserByIdProps = {
  userId: string
}

type UseUpdateUserProps = {
  userId: string
} & UseMutationOptions<any, any, any>

type UsePasswordProps = UseUpdateUserProps

export const useCreateUser = ({ ...options }) => {
  return useMutation<unknown, Error, CreateUserType>({
    mutationKey: ['users'],
    mutationFn: postFetcher(`/api/identity/users`),
    ...options
  })
}

export const useListUsers = ({ ...options }) => {
  return useQuery<any>({
    queryKey: ['users'],
    queryFn: getFetcher(`/api/identity/users`),
    placeholderData: keepPreviousData,
    ...options
  })
}

export const useUserById = ({ userId, ...options }: UseUserByIdProps) => {
  return useQuery<any>({
    queryKey: ['users', userId],
    queryFn: getFetcher(`/api/identity/users/${userId}`),
    placeholderData: keepPreviousData,
    ...options
  })
}

export const useDeleteUser = ({ ...options }) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['users'],
    mutationFn: deleteFetcher(`/api/identity/users`),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['users']
      })
      options.onSuccess?.(...args)
    },
    ...options
  })
}

export const useUpdateUser = ({ userId, ...options }: UseUpdateUserProps) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['users', userId],
    mutationFn: patchFetcher(`/api/identity/users/${userId}`),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['users', userId]
      })
      options.onSuccess?.(...args)
    },
    ...options
  })
}

export const useUpdateUserPassword = ({
  userId,
  ...options
}: UsePasswordProps) => {
  return useMutation({
    mutationKey: ['users', 'update-password', userId],
    mutationFn: patchFetcher(`/api/identity/users/${userId}/password`),
    ...options
  })
}

export const useResetUserPassword = ({
  userId,
  ...options
}: UsePasswordProps) => {
  return useMutation({
    mutationKey: ['users', 'reset-password', userId],
    mutationFn: patchFetcher(`/api/identity/users/${userId}/password/admin`),
    ...options
  })
}

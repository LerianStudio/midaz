import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import {
  AliasEntity,
  CreateAliasEntity,
  UpdateAliasEntity
} from '@/core/domain/entities/alias-entity'
import {
  deleteFetcher,
  getFetcher,
  getPaginatedFetcher,
  patchFetcher,
  postFetcher
} from '@/lib/fetcher'
import { PaginationRequest } from '@/types/pagination-request-type'
import {
  useMutation,
  UseMutationOptions,
  useQuery,
  UseQueryOptions
} from '@tanstack/react-query'

type UseListAliasesProps = PaginationRequest & {
  holderId: string
  enabled?: boolean
}

export const useListAliases = ({
  holderId,
  page,
  limit,
  ...options
}: UseListAliasesProps) => {
  return useQuery<PaginationEntity<AliasEntity>>({
    queryKey: ['holders', holderId, 'aliases', page, limit],
    queryFn: getPaginatedFetcher(`/api/crm/holders/${holderId}/aliases`, {
      page,
      limit
    }),
    enabled: !!holderId,
    ...options
  })
}

type UseAliasbyIdProps = {
  holderId: string
  aliasId: string
  enabled?: boolean
}

export const useAliasById = ({
  holderId,
  aliasId,
  ...options
}: UseAliasbyIdProps) => {
  return useQuery<AliasEntity>({
    queryKey: ['holders', holderId, 'aliases', aliasId],
    queryFn: getFetcher(`/api/crm/holders/${holderId}/aliases/${aliasId}`),
    enabled: !!holderId && !!aliasId,
    ...options
  })
}

type UseCreateAliasProps = UseMutationOptions<
  AliasEntity,
  Error,
  { holderId: string; data: CreateAliasEntity }
>

export const useCreateAlias = (options?: UseCreateAliasProps) => {
  return useMutation<
    AliasEntity,
    Error,
    { holderId: string; data: CreateAliasEntity }
  >({
    mutationKey: ['create-alias'],
    mutationFn: ({ holderId, data }) =>
      postFetcher(`/api/crm/holders/${holderId}/aliases`)(data),
    ...options
  })
}

type UseUpdateAliasProps = UseMutationOptions<
  AliasEntity,
  Error,
  { holderId: string; aliasId: string; data: UpdateAliasEntity }
>

export const useUpdateAlias = (options?: UseUpdateAliasProps) => {
  return useMutation<
    AliasEntity,
    Error,
    { holderId: string; aliasId: string; data: UpdateAliasEntity }
  >({
    mutationKey: ['update-alias'],
    mutationFn: ({ holderId, aliasId, data }) =>
      patchFetcher(`/api/crm/holders/${holderId}/aliases/${aliasId}`)(data),
    ...options
  })
}

type UseDeleteAliasProps = UseMutationOptions<
  void,
  Error,
  { holderId: string; aliasId: string; isHardDelete?: boolean }
>

export const useDeleteAlias = (options?: UseDeleteAliasProps) => {
  return useMutation<
    void,
    Error,
    { holderId: string; aliasId: string; isHardDelete?: boolean }
  >({
    mutationKey: ['delete-alias'],
    mutationFn: async ({ holderId, aliasId, isHardDelete }) => {
      const url = isHardDelete
        ? `/api/crm/holders/${holderId}/aliases/${aliasId}?hard=true`
        : `/api/crm/holders/${holderId}/aliases/${aliasId}`

      const response = await fetch(url, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      })

      if (!response.ok) {
        const errorMessage = await response.json()
        throw new Error(errorMessage.message || 'Failed to delete alias')
      }

      // For DELETE requests, some APIs return 204 No Content
      if (response.status === 204) {
        return
      }

      return response.json()
    },
    ...options
  })
}

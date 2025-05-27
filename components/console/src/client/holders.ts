import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import {
  HolderEntity,
  CreateHolderEntity,
  UpdateHolderEntity
} from '@/core/domain/entities/holder-entity'
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
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'

type UseListHoldersProps = PaginationRequest & {
  enabled?: boolean
}

export const useListHolders = ({
  page,
  limit,
  ...options
}: UseListHoldersProps) => {
  const { currentOrganization } = useOrganization()
  
  return useQuery<PaginationEntity<HolderEntity>>({
    queryKey: ['holders', currentOrganization?.id, page, limit],
    queryFn: getPaginatedFetcher('/api/crm/holders', { 
      page, 
      limit,
      organizationId: currentOrganization?.id 
    }),
    enabled: !!currentOrganization?.id && (options.enabled !== false),
    ...options
  })
}

type UseHolderByIdProps = {
  holderId: string
  enabled?: boolean
}

export const useHolderById = ({ holderId, ...options }: UseHolderByIdProps) => {
  const { currentOrganization } = useOrganization()
  
  return useQuery<HolderEntity>({
    queryKey: ['holders', currentOrganization?.id, holderId],
    queryFn: getFetcher(`/api/crm/holders/${holderId}?organizationId=${currentOrganization?.id}`),
    enabled: !!holderId && !!currentOrganization?.id,
    ...options
  })
}

type UseCreateHolderProps = UseMutationOptions<
  HolderEntity,
  Error,
  CreateHolderEntity
>

export const useCreateHolder = (options?: UseCreateHolderProps) => {
  const { currentOrganization } = useOrganization()
  
  return useMutation<HolderEntity, Error, CreateHolderEntity>({
    mutationKey: ['create-holder', currentOrganization?.id],
    mutationFn: (data) => postFetcher('/api/crm/holders')({
      ...data,
      organizationId: currentOrganization?.id
    }),
    ...options
  })
}

type UseUpdateHolderProps = UseMutationOptions<
  HolderEntity,
  Error,
  { holderId: string; data: UpdateHolderEntity }
>

export const useUpdateHolder = (options?: UseUpdateHolderProps) => {
  const { currentOrganization } = useOrganization()
  
  return useMutation<
    HolderEntity,
    Error,
    { holderId: string; data: UpdateHolderEntity }
  >({
    mutationKey: ['update-holder', currentOrganization?.id],
    mutationFn: ({ holderId, data }) =>
      patchFetcher(`/api/crm/holders/${holderId}?organizationId=${currentOrganization?.id}`)(data),
    ...options
  })
}

type UseDeleteHolderProps = UseMutationOptions<
  void,
  Error,
  { holderId: string; isHardDelete?: boolean }
>

export const useDeleteHolder = (options?: UseDeleteHolderProps) => {
  const { currentOrganization } = useOrganization()
  
  return useMutation<void, Error, { holderId: string; isHardDelete?: boolean }>(
    {
      mutationKey: ['delete-holder', currentOrganization?.id],
      mutationFn: async ({ holderId, isHardDelete }) => {
        const queryParams = new URLSearchParams()
        queryParams.append('organizationId', currentOrganization?.id || '')
        if (isHardDelete) {
          queryParams.append('hard', 'true')
        }
        
        const url = `/api/crm/holders/${holderId}?${queryParams.toString()}`

        const response = await fetch(url, {
          method: 'DELETE',
          headers: {
            'Content-Type': 'application/json'
          }
        })

        if (!response.ok) {
          const errorMessage = await response.json()
          throw new Error(errorMessage.message || 'Failed to delete holder')
        }

        // For DELETE requests, some APIs return 204 No Content
        if (response.status === 204) {
          return
        }

        return response.json()
      },
      ...options
    }
  )
}

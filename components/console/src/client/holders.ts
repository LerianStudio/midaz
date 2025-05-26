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

type UseListHoldersProps = PaginationRequest & {
  enabled?: boolean
}

export const useListHolders = ({
  page,
  limit,
  ...options
}: UseListHoldersProps) => {
  return useQuery<PaginationEntity<HolderEntity>>({
    queryKey: ['holders', page, limit],
    queryFn: getPaginatedFetcher('/api/crm/holders', { page, limit }),
    ...options
  })
}

type UseHolderByIdProps = {
  holderId: string
  enabled?: boolean
}

export const useHolderById = ({ holderId, ...options }: UseHolderByIdProps) => {
  return useQuery<HolderEntity>({
    queryKey: ['holders', holderId],
    queryFn: getFetcher(`/api/crm/holders/${holderId}`),
    enabled: !!holderId,
    ...options
  })
}

type UseCreateHolderProps = UseMutationOptions<
  HolderEntity,
  Error,
  CreateHolderEntity
>

export const useCreateHolder = (options?: UseCreateHolderProps) => {
  return useMutation<HolderEntity, Error, CreateHolderEntity>({
    mutationKey: ['create-holder'],
    mutationFn: postFetcher('/api/crm/holders'),
    ...options
  })
}

type UseUpdateHolderProps = UseMutationOptions<
  HolderEntity,
  Error,
  { holderId: string; data: UpdateHolderEntity }
>

export const useUpdateHolder = (options?: UseUpdateHolderProps) => {
  return useMutation<
    HolderEntity,
    Error,
    { holderId: string; data: UpdateHolderEntity }
  >({
    mutationKey: ['update-holder'],
    mutationFn: ({ holderId, data }) =>
      patchFetcher(`/api/crm/holders/${holderId}`)(data),
    ...options
  })
}

type UseDeleteHolderProps = UseMutationOptions<
  void,
  Error,
  { holderId: string; isHardDelete?: boolean }
>

export const useDeleteHolder = (options?: UseDeleteHolderProps) => {
  return useMutation<void, Error, { holderId: string; isHardDelete?: boolean }>(
    {
      mutationKey: ['delete-holder'],
      mutationFn: ({ holderId, isHardDelete }) => {
        const url = isHardDelete
          ? `/api/crm/holders/${holderId}?hard=true`
          : `/api/crm/holders/${holderId}`
        return deleteFetcher(url)(holderId)
      },
      ...options
    }
  )
}

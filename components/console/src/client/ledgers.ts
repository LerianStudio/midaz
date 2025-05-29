import { LedgerResponseDto } from '@/core/application/dto/ledger-dto'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import {
  deleteFetcher,
  getFetcher,
  patchFetcher,
  postFetcher
} from '@/lib/fetcher'
import { PaginationRequest } from '@/types/pagination-request-type'
import {
  useMutation,
  UseMutationOptions,
  useQuery,
  useQueryClient
} from '@tanstack/react-query'

type UseCreateLedgerProps = UseMutationOptions & {
  organizationId: string
  onSuccess?: (...args: any[]) => void
}

type UseListLedgersProps = {
  organizationId: string
  enabled?: boolean
}

type UseLedgerByIdProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

type UseUpdateLedgerProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
}

type UseDeleteLedgerProps = UseMutationOptions & {
  organizationId: string
  onSuccess?: (...args: any[]) => void
}

const useCreateLedger = ({
  organizationId,
  onSuccess,
  ...options
}: UseCreateLedgerProps) => {
  const queryClient = useQueryClient()

  return useMutation<any, any, any>({
    mutationFn: postFetcher(`/api/organizations/${organizationId}/ledgers`),
    onSuccess: async (...args) => {
      await queryClient.invalidateQueries({
        queryKey: ['ledgers']
      })

      await queryClient.refetchQueries({
        queryKey: ['ledgers', organizationId]
      })

      await onSuccess?.(...args)
    },
    ...options
  })
}

const useListLedgers = ({
  organizationId,
  limit = 10,
  page = 1
}: UseListLedgersProps & PaginationRequest) => {
  return useQuery<PaginationDto<LedgerResponseDto>>({
    queryKey: ['ledgers', organizationId, { limit, page }],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/ledgers-assets?limit=${limit}&page=${page}`
    ),
    enabled: !!organizationId
  })
}

const useLedgerById = ({
  organizationId,
  ledgerId,
  ...options
}: UseLedgerByIdProps) => {
  return useQuery<LedgerResponseDto>({
    queryKey: ['ledger', organizationId, ledgerId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}`
    ),
    enabled: !!organizationId && !!ledgerId,
    ...options
  })
}

const useUpdateLedger = ({
  organizationId,
  ledgerId,
  onSuccess,
  ...options
}: UseUpdateLedgerProps) => {
  const queryClient = useQueryClient()

  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}`
    ),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['ledger', organizationId, ledgerId]
      })
      onSuccess?.(...args)
    },
    ...options
  })
}

const useDeleteLedger = ({
  organizationId,
  onSuccess,
  ...options
}: UseDeleteLedgerProps) => {
  const queryClient = useQueryClient()

  return useMutation<any, any, any>({
    mutationKey: [organizationId],
    mutationFn: deleteFetcher(`/api/organizations/${organizationId}/ledgers`),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['ledgers', organizationId]
      })
      onSuccess?.(...args)
    },
    ...options
  })
}

export {
  useCreateLedger,
  useUpdateLedger,
  useDeleteLedger,
  useListLedgers,
  useLedgerById
}

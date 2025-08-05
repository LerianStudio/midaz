import { useLayoutQueryClient } from '@lerianstudio/console-layout'
import {
  LedgerDto,
  LedgerSearchParamDto
} from '@/core/application/dto/ledger-dto'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import {
  deleteFetcher,
  getFetcher,
  getPaginatedFetcher,
  patchFetcher,
  postFetcher
} from '@/lib/fetcher'
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
  query: LedgerSearchParamDto
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
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['ledgers']
      })

      queryClient.refetchQueries({
        queryKey: ['ledgers', organizationId]
      })

      onSuccess?.(...args)
    },
    ...options
  })
}

const useListLedgers = ({ organizationId, query }: UseListLedgersProps) => {
  return useQuery<PaginationDto<LedgerDto>>({
    queryKey: ['ledgers', organizationId, Object.values(query)],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/ledgers-assets`,
      query
    )
  })
}

const useLedgerById = ({
  organizationId,
  ledgerId,
  ...options
}: UseLedgerByIdProps) => {
  return useQuery<LedgerDto>({
    queryKey: ['ledger', organizationId, ledgerId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}`
    ),
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
  const layoutQueryClient = useLayoutQueryClient()

  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}`
    ),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['ledger', organizationId, ledgerId]
      })
      queryClient.invalidateQueries({
        queryKey: ['ledgers', organizationId]
      })
      layoutQueryClient.invalidateQueries({
        queryKey: ['ledgers', organizationId]
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
  const layoutQueryClient = useLayoutQueryClient()

  return useMutation<any, any, any>({
    mutationKey: [organizationId],
    mutationFn: deleteFetcher(`/api/organizations/${organizationId}/ledgers`),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: ['ledgers', organizationId]
      })
      layoutQueryClient.invalidateQueries({
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

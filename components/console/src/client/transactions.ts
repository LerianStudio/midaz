import {
  getFetcher,
  patchFetcher,
  postFetcher,
  getPaginatedFetcher
} from '@/lib/fetcher'
import {
  useMutation,
  useQuery,
  useQueryClient,
  UseMutationOptions,
  UseQueryResult,
  UseMutationResult
} from '@tanstack/react-query'
import { PaginationRequest } from '@/types/pagination-request-type'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import {
  CreateTransactionDto,
  TransactionResponseDto
} from '@/core/application/dto/transaction-dto'

export type UseListTransactionsProps = {
  organizationId: string
  ledgerId: string | null
  enabled?: boolean
} & PaginationRequest

export type UseCreateTransactionProps = {
  organizationId: string
  ledgerId: string
  onSuccess?: (data: TransactionResponseDto) => void
  onError?: (message: string) => void
}

export const useCreateTransaction = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreateTransactionProps): UseMutationResult<
  TransactionResponseDto | CreateTransactionDto
> => {
  return useMutation<any, any, any>({
    mutationKey: ['transactions', 'create'],
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transactions/json`
    ),
    ...options
  })
}

type UseGetTransactionByIdProps = {
  organizationId: string
  ledgerId: string
  transactionId: string
}

export const useGetTransactionById = ({
  organizationId,
  ledgerId,
  transactionId,
  ...options
}: UseGetTransactionByIdProps): UseQueryResult<
  TransactionResponseDto,
  Error
> => {
  return useQuery({
    queryKey: ['transactions-by-id', transactionId, organizationId, ledgerId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`
    ),
    enabled: !!organizationId && !!ledgerId,
    ...options
  })
}

export const useListTransactions = ({
  organizationId,
  ledgerId,
  page,
  limit,
  ...options
}: UseListTransactionsProps) => {
  return useQuery<PaginationDto<any>>({
    queryKey: ['transactions-list', organizationId, ledgerId, page, limit],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transactions`,
      { page, limit }
    ),
    enabled: !!organizationId && !!ledgerId,
    ...options
  })
}

type UseUpdateTransactionProps = UseMutationOptions<
  any,
  Error,
  { metadata?: Record<string, any> | null; description?: string | null }
> & {
  organizationId: string
  ledgerId: string
  transactionId: string
}

export const useUpdateTransaction = ({
  organizationId,
  ledgerId,
  transactionId,
  onSuccess,
  ...options
}: UseUpdateTransactionProps) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: ['transactions', 'update'],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`
    ),
    onSuccess: (...args) => {
      queryClient.invalidateQueries({
        queryKey: [
          'transactions-by-id',
          transactionId,
          organizationId,
          ledgerId
        ]
      })
      onSuccess?.(...args)
    },
    ...options
  })
}

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
  TransactionDto,
  TransactionSearchDto
} from '@/core/application/dto/transaction-dto'

export type UseListTransactionsProps = {
  organizationId: string
  ledgerId: string | null
  query: TransactionSearchDto
}

export type UseCreateTransactionProps = {
  organizationId: string
  ledgerId: string
  onSuccess?: (data: TransactionDto) => void
  onError?: (message: string) => void
}

export const useCreateTransaction = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreateTransactionProps): UseMutationResult<
  TransactionDto | CreateTransactionDto
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
}: UseGetTransactionByIdProps): UseQueryResult<TransactionDto, Error> => {
  return useQuery({
    queryKey: ['transactions-by-id', transactionId, ledgerId, organizationId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`
    ),
    ...options
  })
}

export const useListTransactions = ({
  organizationId,
  ledgerId,
  query,
  ...options
}: UseListTransactionsProps) => {
  return useQuery<PaginationDto<TransactionDto>>({
    queryKey: [
      'transactions-list',
      organizationId,
      ledgerId,
      Object.values(query)
    ],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transactions`,
      query
    ),
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
          ledgerId,
          organizationId
        ]
      })
      onSuccess?.(...args)
    },
    ...options
  })
}

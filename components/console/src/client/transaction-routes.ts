import {
  TransactionRoutesDto,
  TransactionRoutesSearchParamDto,
  CreateTransactionRoutesDto,
  UpdateTransactionRoutesDto
} from '@/core/application/dto/transaction-routes-dto'
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
  UseQueryOptions
} from '@tanstack/react-query'

type UseListTransactionRoutesProps = {
  organizationId: string
  ledgerId: string
  query?: TransactionRoutesSearchParamDto
  enabled?: boolean
} & Omit<
  UseQueryOptions<PaginationDto<TransactionRoutesDto>>,
  'queryKey' | 'queryFn'
>

export const useListTransactionRoutes = ({
  organizationId,
  ledgerId,
  query,
  enabled = true,
  ...options
}: UseListTransactionRoutesProps) => {
  return useQuery<PaginationDto<TransactionRoutesDto>>({
    queryKey: [
      organizationId,
      ledgerId,
      'transaction-routes',
      ...Object.values(query ?? {})
    ],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes`,
      query
    ),
    enabled: !!organizationId && !!ledgerId && enabled,
    ...options
  })
}

type UseTransactionRouteProps = {
  organizationId: string
  ledgerId: string
  transactionRouteId: string
  enabled?: boolean
} & Omit<UseQueryOptions<TransactionRoutesDto>, 'queryKey' | 'queryFn'>

export const useGetTransactionRoute = ({
  organizationId,
  ledgerId,
  transactionRouteId,
  ...options
}: UseTransactionRouteProps) => {
  return useQuery<TransactionRoutesDto>({
    queryKey: [
      organizationId,
      ledgerId,
      'transaction-routes',
      transactionRouteId
    ],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes/${transactionRouteId}`
    ),
    ...options
  })
}

type UseCreateTransactionRouteProps = {
  organizationId: string
  ledgerId: string
} & UseMutationOptions<TransactionRoutesDto, any, CreateTransactionRoutesDto>

export const useCreateTransactionRoute = ({
  organizationId,
  ledgerId,
  ...options
}: UseCreateTransactionRouteProps) => {
  return useMutation<TransactionRoutesDto, any, CreateTransactionRoutesDto>({
    mutationKey: ['create-transaction-route', organizationId, ledgerId],
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes`
    ),
    ...options
  })
}

type UseUpdateTransactionRouteProps = {
  organizationId: string
  ledgerId: string
  transactionRouteId: string
} & UseMutationOptions<TransactionRoutesDto, any, UpdateTransactionRoutesDto>

export const useUpdateTransactionRoute = ({
  organizationId,
  ledgerId,
  transactionRouteId,
  ...options
}: UseUpdateTransactionRouteProps) => {
  return useMutation<TransactionRoutesDto, any, UpdateTransactionRoutesDto>({
    mutationKey: [
      'update-transaction-route',
      organizationId,
      ledgerId,
      transactionRouteId
    ],
    mutationFn: patchFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes/${transactionRouteId}`
    ),
    ...options
  })
}

type UseDeleteTransactionRouteProps = UseMutationOptions & {
  organizationId: string
  ledgerId: string
  transactionRouteId: string
}

export const useDeleteTransactionRoute = ({
  organizationId,
  ledgerId,
  transactionRouteId,
  ...options
}: UseDeleteTransactionRouteProps) => {
  return useMutation<any, any, any>({
    mutationKey: [organizationId, ledgerId, transactionRouteId],
    mutationFn: deleteFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes`
    ),
    ...options
  })
}

import { OperationRoutesDto } from '@/core/application/dto/operation-routes-dto'
import {
  TransactionRoutesDto,
  TransactionRoutesSearchParamDto
} from '@/core/application/dto/transaction-routes-dto'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { getFetcher, getPaginatedFetcher } from '@/lib/fetcher'
import { useQuery, UseQueryOptions } from '@tanstack/react-query'

type UseTransactionOperationRoutesProps = {
  organizationId: string
  ledgerId: string
  transactionRouteId: string
  enabled?: boolean
} & Omit<UseQueryOptions<OperationRoutesDto[]>, 'queryKey' | 'queryFn'>

export const useGetTransactionOperationRoutes = ({
  organizationId,
  ledgerId,
  transactionRouteId,
  enabled = true,
  ...options
}: UseTransactionOperationRoutesProps) => {
  return useQuery<OperationRoutesDto[]>({
    queryKey: [
      organizationId,
      ledgerId,
      'transaction-operation-routes',
      transactionRouteId
    ],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes/${transactionRouteId}/operation-routes`
    ),
    enabled: !!organizationId && !!ledgerId && !!transactionRouteId && enabled,
    ...options
  })
}

type UseTransactionRoutesWithOperationRoutesProps = {
  organizationId: string
  ledgerId: string
  query?: TransactionRoutesSearchParamDto
} & Omit<
  UseQueryOptions<PaginationDto<TransactionRoutesDto>>,
  'queryKey' | 'queryFn'
>

export const useListTransactionRoutesWithOperationRoutes = ({
  organizationId,
  ledgerId,
  query,
  ...options
}: UseTransactionRoutesWithOperationRoutesProps) => {
  return useQuery<PaginationDto<TransactionRoutesDto>>({
    queryKey: [
      organizationId,
      ledgerId,
      'transaction-routes-with-operation-routes',
      ...Object.values(query ?? {})
    ],
    queryFn: getPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transaction-operation-routes`,
      query
    ),
    ...options
  })
}

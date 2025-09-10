import {
  TransactionRoutesDto,
  TransactionRoutesSearchParamDto
} from '@/core/application/dto/transaction-routes-dto'
import { CursorPaginationDto } from '@/core/application/dto/pagination-dto'
import { getCursorPaginatedFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

type UseListTransactionRoutesCursorProps = {
  organizationId: string
  ledgerId: string
  cursor?: string
  limit?: number
  sortOrder?: 'desc' | 'asc'
  sortBy?: 'id' | 'title' | 'createdAt' | 'updatedAt'
  id?: string
  enabled?: boolean
}

export const useListTransactionRoutesCursor = ({
  organizationId,
  ledgerId,
  cursor,
  limit = 10,
  sortOrder = 'desc',
  sortBy = 'createdAt',
  id,
  enabled = true,
  ...options
}: UseListTransactionRoutesCursorProps) => {
  const params: TransactionRoutesSearchParamDto = {
    cursor,
    limit,
    sortOrder,
    sortBy,
    id
  }

  // Filter out undefined values
  const cleanParams = Object.fromEntries(
    Object.entries(params).filter(([_, value]) => value !== undefined)
  )

  return useQuery<CursorPaginationDto<TransactionRoutesDto>>({
    queryKey: [
      organizationId,
      ledgerId,
      'transaction-routes-cursor',
      cleanParams
    ],
    queryFn: getCursorPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes`,
      cleanParams
    ),
    enabled: !!organizationId && !!ledgerId && enabled,
    ...options
  })
}

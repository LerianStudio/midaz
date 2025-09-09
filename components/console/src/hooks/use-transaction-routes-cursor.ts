import { useMemo, useEffect } from 'react'
import { TransactionRoutesSearchParamDto } from '@/core/application/dto/transaction-routes-dto'
import {
  useCursorPagination,
  UseCursorPaginationProps
} from './use-cursor-pagination'
import { useListTransactionRoutesCursor } from '@/client/transaction-routes-cursor'

export type UseTransactionRoutesCursorProps = {
  organizationId: string
  ledgerId: string
  searchParams?: Omit<
    TransactionRoutesSearchParamDto,
    'limit' | 'cursor' | 'sortOrder'
  >
  enabled?: boolean
} & UseCursorPaginationProps

export function useTransactionRoutesCursor({
  organizationId,
  ledgerId,
  searchParams,
  enabled = true,
  ...paginationProps
}: UseTransactionRoutesCursorProps) {
  const pagination = useCursorPagination(paginationProps)

  const { data, isLoading, error, refetch } = useListTransactionRoutesCursor({
    organizationId,
    ledgerId,
    cursor: pagination.cursor,
    limit: pagination.limit,
    sortOrder: pagination.sortOrder,
    sortBy: searchParams?.sortBy,
    id: searchParams?.id,
    enabled: enabled && !!organizationId && !!ledgerId
  })

  // Update pagination state when data changes
  useEffect(() => {
    if (data) {
      pagination.updatePaginationState({
        next_cursor: data.nextCursor,
        prev_cursor: data.prevCursor
      })
    }
  }, [data, pagination])

  const queryParams = useMemo(
    (): TransactionRoutesSearchParamDto => ({
      ...searchParams,
      limit: pagination.limit,
      cursor: pagination.cursor,
      sortOrder: pagination.sortOrder
    }),
    [searchParams, pagination.limit, pagination.cursor, pagination.sortOrder]
  )

  return {
    // Data
    data,
    transactionRoutes: data?.items ?? [],
    isEmpty: data?.items?.length === 0,

    // Loading states
    isLoading,
    error,

    // Pagination controls
    ...pagination,

    // Actions
    refetch,

    // Debug info
    queryParams
  }
}

export type UseTransactionRoutesCursorReturn = ReturnType<
  typeof useTransactionRoutesCursor
>

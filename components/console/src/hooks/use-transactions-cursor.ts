import { useMemo, useEffect } from 'react'
import { TransactionSearchDto } from '@/core/application/dto/transaction-dto'
import {
  useCursorPagination,
  UseCursorPaginationProps
} from './use-cursor-pagination'
import { useListTransactionsCursor } from '@/client/transactions'

export type UseTransactionsCursorProps = {
  organizationId: string
  ledgerId: string
  searchParams?: Omit<TransactionSearchDto, 'limit' | 'cursor' | 'sortOrder'>
  enabled?: boolean
} & UseCursorPaginationProps

export function useTransactionsCursor({
  organizationId,
  ledgerId,
  searchParams,
  enabled = true,
  ...paginationProps
}: UseTransactionsCursorProps) {
  const pagination = useCursorPagination(paginationProps)
  const { data, isLoading, error, refetch } = useListTransactionsCursor({
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
    (): TransactionSearchDto => ({
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
    transactions: data?.items ?? [],
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

export type UseTransactionsCursorReturn = ReturnType<
  typeof useTransactionsCursor
>

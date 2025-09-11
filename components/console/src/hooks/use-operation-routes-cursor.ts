import { useMemo, useEffect } from 'react'
import { OperationRoutesSearchParamDto } from '@/core/application/dto/operation-routes-dto'
import {
  useCursorPagination,
  UseCursorPaginationProps
} from './use-cursor-pagination'
import { useListOperationRoutesCursor } from '@/client/operation-routes'

export type UseOperationRoutesCursorProps = {
  organizationId: string
  ledgerId: string
  searchParams?: Omit<
    OperationRoutesSearchParamDto,
    'limit' | 'cursor' | 'sortOrder'
  >
  enabled?: boolean
} & UseCursorPaginationProps

export function useOperationRoutesCursor({
  organizationId,
  ledgerId,
  searchParams,
  enabled = true,
  ...paginationProps
}: UseOperationRoutesCursorProps) {
  const pagination = useCursorPagination(paginationProps)
  const { data, isLoading, error, refetch } = useListOperationRoutesCursor({
    organizationId,
    ledgerId,
    cursor: pagination.cursor,
    limit: pagination.limit,
    sortOrder: pagination.sortOrder,
    sortBy: searchParams?.sortBy,
    id: searchParams?.id,
    title: searchParams?.title,
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
    (): OperationRoutesSearchParamDto => ({
      ...searchParams,
      limit: pagination.limit,
      cursor: pagination.cursor,
      sortOrder: pagination.sortOrder
    }),
    [searchParams, pagination.limit, pagination.cursor, pagination.sortOrder]
  )

  return {
    data,
    operationRoutes: data?.items ?? [],
    isEmpty: data?.items?.length === 0,
    isLoading,
    error,
    ...pagination,
    refetch,
    queryParams
  }
}

export type UseOperationRoutesCursorReturn = ReturnType<
  typeof useOperationRoutesCursor
>

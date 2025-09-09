import { useListAccountTypesCursor } from '@/client/account-types'
import { AccountTypesSearchParamDto } from '@/core/application/dto/account-types-dto'
import {
  useCursorPagination,
  UseCursorPaginationProps
} from './use-cursor-pagination'
import { useEffect, useMemo } from 'react'

export type UseAccountTypesCursorProps = {
  organizationId: string
  ledgerId: string
  searchParams?: Omit<
    AccountTypesSearchParamDto,
    'limit' | 'cursor' | 'sortOrder'
  >
  enabled?: boolean
} & UseCursorPaginationProps

export function useAccountTypesCursor({
  organizationId,
  ledgerId,
  searchParams,
  enabled = true,
  ...paginationProps
}: UseAccountTypesCursorProps) {
  const pagination = useCursorPagination(paginationProps)

  const { data, isLoading, error, refetch } = useListAccountTypesCursor({
    organizationId,
    ledgerId,
    cursor: pagination.cursor,
    limit: pagination.limit,
    sortOrder: pagination.sortOrder,
    sortBy: searchParams?.sortBy,
    id: searchParams?.id,
    enabled: enabled && !!organizationId && !!ledgerId
  })

  useEffect(() => {
    if (data) {
      pagination.updatePaginationState({
        next_cursor: data.nextCursor,
        prev_cursor: data.prevCursor
      })
    }
  }, [data, pagination])

  const queryParams = useMemo(
    (): AccountTypesSearchParamDto => ({
      ...searchParams,
      limit: pagination.limit,
      cursor: pagination.cursor,
      sortOrder: pagination.sortOrder
    }),
    [searchParams, pagination.limit, pagination.cursor, pagination.sortOrder]
  )

  return {
    data,
    accountTypes: data?.items ?? [],
    isEmpty: data?.items?.length === 0,
    isLoading,
    error,
    ...pagination,
    refetch,
    queryParams
  }
}

export type UseAccountTypesCursorReturn = ReturnType<
  typeof useAccountTypesCursor
>

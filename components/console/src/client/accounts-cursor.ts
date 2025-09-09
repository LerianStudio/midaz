import { AccountDto } from '@/core/application/dto/account-dto'
import { CursorPaginationDto } from '@/core/application/dto/pagination-dto'
import { getCursorPaginatedFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

type UseListAccountsCursorProps = {
  organizationId: string
  ledgerId: string
  cursor?: string
  limit?: number
  sortOrder?: 'asc' | 'desc'
  enabled?: boolean
}

export const useListAccountsCursor = ({
  organizationId,
  ledgerId,
  cursor,
  limit = 10,
  sortOrder = 'asc',
  enabled = true,
  ...options
}: UseListAccountsCursorProps) => {
  const params = {
    cursor,
    limit,
    sort_order: sortOrder
  }

  // Filter out undefined values
  const cleanParams = Object.fromEntries(
    Object.entries(params).filter(([_, value]) => value !== undefined)
  )

  return useQuery<CursorPaginationDto<AccountDto>>({
    queryKey: [organizationId, ledgerId, 'accounts-cursor', cleanParams],
    queryFn: getCursorPaginatedFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/accounts`,
      cleanParams
    ),
    enabled: !!organizationId && !!ledgerId && enabled,
    ...options
  })
}

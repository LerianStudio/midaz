import { BalanceDto } from '@/core/application/dto/balance-dto'
import { PaginationDto } from '@/core/application/dto/pagination-dto'
import { getFetcher } from '@/lib/fetcher'
import { useQuery } from '@tanstack/react-query'

type UseGetBalanceByAccountIdProps = {
  organizationId: string
  ledgerId: string
  accountId?: string
}

export const useGetBalanceByAccountId = ({
  organizationId,
  ledgerId,
  accountId
}: UseGetBalanceByAccountIdProps) => {
  return useQuery<PaginationDto<BalanceDto>>({
    queryKey: ['balances', organizationId, ledgerId, accountId],
    queryFn: getFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}/balances`
    )
  })
}

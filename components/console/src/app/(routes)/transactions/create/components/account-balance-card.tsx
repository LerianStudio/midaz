import { useGetBalanceByAccountId } from '@/client/balances'
import { Separator } from '@/components/ui/separator'
import { AccountDto } from '@/core/application/dto/account-dto'
import { BalanceDto } from '@/core/application/dto/balance-dto'
import {
  AccountBalanceCardContent,
  AccountBalanceCardDeleteButton,
  AccountBalanceCardEmpty,
  AccountBalanceCardHeader,
  AccountBalanceCardIcon,
  AccountBalanceCardInfo,
  AccountBalanceCardLoading,
  AccountBalanceCard as AccountBalanceCardPrimitive,
  AccountBalanceCardTitle,
  AccountBalanceCardTrigger,
  AccountBalanceCardUpdateButton
} from '@/components/transactions/primitives/account-balance-card'
import { useEffect, useState } from 'react'

export type AccountBalanceCardProps =
  React.HtmlHTMLAttributes<HTMLDivElement> & {
    open?: boolean
    onOpenChange?: (open: boolean) => void
    account: AccountDto
    icon?: boolean
    expand?: boolean
    onRefresh?: () => void
    onRefreshed?: (balances: BalanceDto[]) => void
    onDelete?: () => void
  }

export const AccountBalanceCard = ({
  className,
  open = false,
  onOpenChange,
  account,
  icon,
  expand,
  onRefresh,
  onRefreshed,
  onDelete
}: AccountBalanceCardProps) => {
  const [_open, _setOpen] = useState(open)

  const {
    data: balances,
    isFetched,
    isFetching: loading,
    refetch,
    dataUpdatedAt
  } = useGetBalanceByAccountId({
    organizationId: account.organizationId,
    ledgerId: account.ledgerId,
    accountId: account.id,
    enabled: !!expand
  })

  const handleOpen = (open: boolean) => {
    _setOpen(open)
    onOpenChange?.(open)
  }

  const handleRefresh = () => {
    refetch()
    onRefresh?.()
  }

  useEffect(() => {
    _setOpen(open)
  }, [open])

  useEffect(() => {
    if (isFetched && balances?.items?.length) {
      onRefreshed?.(balances.items)
    }
  }, [isFetched, balances, dataUpdatedAt])

  return (
    <AccountBalanceCardPrimitive
      className={className}
      open={_open}
      onOpenChange={handleOpen}
    >
      <AccountBalanceCardHeader>
        {icon && <AccountBalanceCardIcon />}
        <AccountBalanceCardTitle>{account.alias}</AccountBalanceCardTitle>
        <AccountBalanceCardDeleteButton onClick={onDelete} />
      </AccountBalanceCardHeader>
      <AccountBalanceCardContent>
        {loading && <AccountBalanceCardLoading />}
        {!loading &&
          balances?.items?.map((balance) => (
            <AccountBalanceCardInfo
              key={balance.id}
              assetCode={balance.assetCode}
              value={balance.available}
            />
          ))}
        {!loading && balances?.items?.length === 0 && (
          <AccountBalanceCardEmpty />
        )}

        <Separator className="mt-3 mb-2" />
        <AccountBalanceCardUpdateButton
          loading={loading}
          timestamp={dataUpdatedAt}
          onRefresh={handleRefresh}
        />
      </AccountBalanceCardContent>
      {expand && <AccountBalanceCardTrigger />}
    </AccountBalanceCardPrimitive>
  )
}

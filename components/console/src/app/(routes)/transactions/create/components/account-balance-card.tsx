import { useGetBalanceByAccountId } from '@/client/balances'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from '@/components/ui/collapsible'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { AccountDto } from '@/core/application/dto/account-dto'
import { BalanceDto } from '@/core/application/dto/balance-dto'
import { useTime } from '@/hooks/use-time'
import { cn } from '@/lib/utils'
import dayjs from 'dayjs'
import { CheckCircle2, RefreshCw, Trash } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useIntl } from 'react-intl'

const UpdateButton = ({
  loading,
  timestamp,
  onRefresh
}: {
  loading: boolean
  timestamp: number
  onRefresh: () => void
}) => {
  const intl = useIntl()
  const time = useTime({ interval: 1000 * 1 })

  const updated = useMemo(() => {
    return !dayjs(timestamp).isBefore(dayjs().subtract(1, 'second'))
  }, [time, timestamp])

  return (
    <div className="mb-3 flex flex-row items-center justify-end gap-2">
      <p className="text-xs font-medium text-shadcn-500">
        {loading &&
          intl.formatMessage({
            id: 'common.updating',
            defaultMessage: 'Updating...'
          })}
        {!loading &&
          !updated &&
          intl.formatMessage(
            {
              id: 'common.updatedIn',
              defaultMessage: 'Updated {time}'
            },
            {
              time: dayjs(timestamp).fromNow()
            }
          )}
        {!loading &&
          updated &&
          intl.formatMessage({
            id: 'common.updated',
            defaultMessage: 'Updated'
          })}
      </p>
      {updated && <CheckCircle2 className="h-4 w-4 text-green-600" />}
      {!updated && (
        <Button
          variant="link"
          className="h-3 p-0"
          onClick={onRefresh}
          disabled={loading}
        >
          <RefreshCw className="h-4 w-4" />
        </Button>
      )}
    </div>
  )
}

export type AccountBalanceCardProps =
  React.HtmlHTMLAttributes<HTMLDivElement> & {
    open?: boolean
    onOpenChange?: (open: boolean) => void
    account: AccountDto
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
  expand,
  onRefresh,
  onRefreshed,
  onDelete
}: AccountBalanceCardProps) => {
  const intl = useIntl()
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
    accountId: account.id
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
    <Collapsible
      className={cn('w-full', className)}
      open={_open}
      onOpenChange={handleOpen}
    >
      <Card className="relative gap-2 px-5 py-4">
        <div className="flex flex-row items-center justify-between">
          <p className="text-lg font-bold text-zinc-700">{account.alias}</p>
          <Button
            variant="plain"
            className="absolute right-3 top-3 h-8 w-8 p-0"
            onClick={onDelete}
          >
            <Trash className="h-4 w-4 text-zinc-600" />
          </Button>
        </div>
        <CollapsibleContent className="overflow-hidden data-[state=closed]:animate-accordion-up data-[state=open]:animate-accordion-down">
          {loading && (
            <Skeleton className="mt-3 h-3 w-full rounded-md bg-zinc-200" />
          )}
          {!loading &&
            balances?.items?.map((balance) => (
              <div
                key={balance.id}
                className="mt-3 flex flex-row items-center justify-between text-sm font-normal text-shadcn-500"
              >
                <p>{account.assetCode}</p>
                <p>{intl.formatNumber(balance.available)}</p>
              </div>
            ))}
          {!loading && balances?.items?.length === 0 && (
            <p className="mt-3 text-center text-sm font-normal text-shadcn-500">
              {intl.formatMessage({
                id: 'common.noBalance',
                defaultMessage: 'No balance'
              })}
            </p>
          )}

          <Separator className="mb-2 mt-3" />
          <UpdateButton
            loading={loading}
            timestamp={dataUpdatedAt}
            onRefresh={handleRefresh}
          />
        </CollapsibleContent>
        {expand && (
          <CollapsibleTrigger asChild>
            <Button variant="link" className="h-4 p-0 text-xs text-zinc-600">
              {!open
                ? intl.formatMessage({
                    id: 'common.showBalance',
                    defaultMessage: 'Show balance'
                  })
                : intl.formatMessage({
                    id: 'common.hide',
                    defaultMessage: 'Hide'
                  })}
            </Button>
          </CollapsibleTrigger>
        )}
      </Card>
    </Collapsible>
  )
}

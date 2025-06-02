import { Button } from '@/components/ui/button'
import { AccountBalanceCard } from './account-balance-card'
import { useIntl } from 'react-intl'
import { useTransactionForm } from '../transaction-form-provider'
import { Control, useWatch } from 'react-hook-form'
import { useEffect, useMemo, useState } from 'react'
import { TransactionFormSchema } from '../schemas'
import { cn } from '@/lib/utils'
import { BalanceDto } from '@/core/application/dto/balance-dto'

export type AccountBalanceListProps = {
  name: string
  control: Control<TransactionFormSchema>
  onRemove?: (alias: string) => void
  expand?: boolean
}

export const AccountBalanceList = ({
  name,
  control,
  onRemove,
  expand
}: AccountBalanceListProps) => {
  const intl = useIntl()

  const { accounts, addBalance } = useTransactionForm()
  const [opened, setOpened] = useState<boolean[]>([])

  const values = useWatch({ name: name as 'source' | 'destination', control })

  const open = useMemo(() => opened.every((item) => item), [opened.toString()])

  const handleOpenAll = () => {
    setOpened((prev) => prev.map(() => !open))
  }

  const handleOpenChange = (index: number, open: boolean) => {
    setOpened((prev) => {
      const newOpened = [...prev]
      newOpened[index] = open
      return newOpened
    })
  }

  const handleRefreshed = (alias: string, balances: BalanceDto[]) => {
    balances.forEach((balance) => {
      addBalance(alias, balance)
    })
  }

  useEffect(() => {
    if (values.length > opened.length) {
      setOpened((prev) => [
        ...prev,
        ...Array(values.length - prev.length).fill(false)
      ])
    }
  }, [values.length])

  return (
    <div className="flex h-full flex-col">
      {values.length > 0 && (
        <>
          <div
            className={cn(
              'mb-4 flex grow flex-col justify-center gap-2 rounded bg-zinc-200 p-4',
              {
                'mb-14': !expand
              }
            )}
          >
            {values?.map((source, index) => (
              <AccountBalanceCard
                key={source.account}
                open={opened[index]}
                onOpenChange={(open) => handleOpenChange(index, open)}
                account={accounts[source.account!]}
                onRefreshed={(balances) =>
                  handleRefreshed(source.account!, balances)
                }
                onDelete={() => {
                  onRemove?.(source.account!)
                  setOpened((prev) => [
                    ...prev.slice(0, index),
                    ...prev.slice(index + 1)
                  ])
                }}
                expand={expand}
              />
            ))}
          </div>
          {expand && (
            <Button
              variant="link"
              className="w-fit text-xs"
              onClick={handleOpenAll}
            >
              {!open
                ? intl.formatMessage({
                    id: 'transactions.create.showBalances',
                    defaultMessage: 'View all balances'
                  })
                : intl.formatMessage({
                    id: 'transactions.create.hideBalances',
                    defaultMessage: 'Hide all balances'
                  })}
            </Button>
          )}
        </>
      )}
    </div>
  )
}

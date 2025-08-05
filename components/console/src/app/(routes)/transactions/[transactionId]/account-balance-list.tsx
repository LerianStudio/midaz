import { TransactionOperationDto } from '@/core/application/dto/transaction-dto'
import { cn } from '@/lib/utils'
import {
  AccountBalanceCardHeader,
  AccountBalanceCard as AccountBalanceCardPrimitive,
  AccountBalanceCardTitle
} from '@/components/transactions/primitives/account-balance-card'

export type AccountBalanceListProps = {
  values?: TransactionOperationDto[] | []
}

export const AccountBalanceList = ({
  values = []
}: AccountBalanceListProps) => {
  return (
    <div className="flex h-full w-full flex-col">
      {values.length > 0 && (
        <div
          className={cn(
            'mb-4 flex grow flex-col justify-center gap-2 rounded bg-zinc-200 p-4'
          )}
        >
          {values?.map((source, index) => (
            <AccountBalanceCardPrimitive key={index}>
              <AccountBalanceCardHeader>
                <AccountBalanceCardTitle>
                  {source.accountAlias}
                </AccountBalanceCardTitle>
              </AccountBalanceCardHeader>
            </AccountBalanceCardPrimitive>
          ))}
        </div>
      )}
    </div>
  )
}

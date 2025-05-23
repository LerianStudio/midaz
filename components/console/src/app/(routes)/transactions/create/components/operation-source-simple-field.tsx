import { Control, useWatch } from 'react-hook-form'
import { TransactionFormSchema } from '../schemas'
import { AccountSearchField } from './account-search-field'
import { AccountBalanceCard } from './account-balance-card'
import { useTransactionForm } from '../transaction-form-provider'
import { AccountDto } from '@/core/application/dto/account-dto'
import { BalanceDto } from '@/core/application/dto/balance-dto'

export type OperationSourceSimpleFieldProps = {
  name: string
  onSubmit?: (value: string, account: AccountDto) => void
  onRefreshed?: () => void
  onRemove?: (alias: string) => void
  control: Control<TransactionFormSchema>
  expand?: boolean
}

export const OperationSourceSimpleField = ({
  name,
  onSubmit,
  onRefreshed,
  onRemove,
  control,
  expand,
  ...props
}: OperationSourceSimpleFieldProps) => {
  const { accounts, addBalance } = useTransactionForm()

  const values = useWatch({ name: name as 'source' | 'destination', control })

  const handleRefreshed = (balances: BalanceDto[]) => {
    balances.forEach((balance) => {
      addBalance(values[0].account, balance)
    })
  }

  return (
    <>
      {values.length > 0 && (
        <AccountBalanceCard
          account={accounts[values[0].account!]}
          onRefreshed={handleRefreshed}
          onDelete={() => onRemove?.(values[0].account)}
          icon
          expand={expand}
          {...props}
        />
      )}
      {values.length === 0 && (
        <AccountSearchField onSelect={onSubmit} {...props} />
      )}
    </>
  )
}

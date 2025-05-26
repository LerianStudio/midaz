import { AccountDto } from '@/core/application/dto/account-dto'
import { BalanceDto } from '@/core/application/dto/balance-dto'
import { useNormalize } from '@/hooks/use-normalize'

export type Account = AccountDto & {
  balances?: BalanceDto[]
}

/**
 * Hook used to manage and store accounts on
 * transaction context, sources and destinations.
 * With balances nested inside.
 * @returns
 */
export function useAccounts() {
  const { data: accounts, add, set, remove, clear } = useNormalize<Account>()

  const addBalance = (alias: string, balance: BalanceDto) => {
    set((prev) => {
      const account = prev[alias]

      if (!account) {
        return prev
      }

      const balances = [...(account.balances || [])]
      const index = balances.findIndex((b) => b.id === balance.id)
      if (index !== -1) {
        balances[index] = balance
      } else {
        balances.push(balance)
      }

      return {
        ...prev,
        [alias]: {
          ...account,
          balances
        }
      }
    })
  }

  return {
    accounts,
    add,
    remove,
    clear,
    addBalance
  }
}

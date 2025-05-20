import React from 'react'
import { useIntl } from 'react-intl'
import { TransactionFormSchema, TransactionSourceFormSchema } from '../schemas'
import { useCustomFormError } from '@/hooks/use-custom-form-error'
import { Account } from './use-account'
import { TransactionMode, useTransactionMode } from './use-transaction-mode'
import { externalAccountAliasPrefix } from '@/core/infrastructure/midaz/config/config'

export const useTransactionFormErrors = (
  values: TransactionFormSchema,
  accounts: Record<string, Account>
) => {
  const intl = useIntl()
  const [open, setOpen] = React.useState(false)
  const { mode } = useTransactionMode()
  const { errors, add, remove } = useCustomFormError()
  const { value, source, destination } = values

  const sum = (source: TransactionSourceFormSchema) =>
    source.reduce((acc, curr) => acc + Number(curr.value), 0)

  const dataLoss = (values: TransactionFormSchema) => {
    if (
      mode === TransactionMode.COMPLEX &&
      (values.source.length > 1 || values.destination.length > 1)
    ) {
      add('data-loss', {
        message: intl.formatMessage({
          id: 'transactions.create.mode.simple.warning',
          defaultMessage:
            'You are selecting the simple transaction mode and might lose already filled information.'
        })
      })
    } else {
      remove('data-loss')
    }
  }

  const totalSumSourceRule = (values: TransactionFormSchema) => {
    const value = Number(values.value)
    const totalSource = sum(values.source)

    if (value !== totalSource) {
      add('debit', {
        message: intl.formatMessage({
          id: 'transactions.errors.debit',
          defaultMessage:
            'The sum of the debits differs from the transaction amount'
        })
      })
      return true
    }

    remove('debit')
    return false
  }

  const totalSumDestinationRule = (values: TransactionFormSchema) => {
    const value = Number(values.value)
    const totalDestination = sum(values.destination)

    if (value !== totalDestination) {
      add('credit', {
        message: intl.formatMessage({
          id: 'transactions.errors.credit',
          defaultMessage:
            'The sum of the credits differs from the transaction amount'
        })
      })
      return true
    }

    remove('credit')
    return false
  }

  const insufficientFundsRule = (
    values: TransactionFormSchema,
    accounts: Record<string, Account>
  ) => {
    const asset = values.asset
    const sources = values.source

    // Check if source if has enough funds to
    // complete the transaction
    return sources
      .map((source) => {
        // External accounts has untrusted balances
        if (source.account.includes(externalAccountAliasPrefix)) {
          return false
        }

        const account = accounts[source.account]
        if (!account) {
          return false
        }

        const balance = account.balances?.find(
          (balance) => balance.assetCode === asset
        )

        if (!balance) {
          add(`source.${source.account}`, {
            message: intl.formatMessage(
              {
                id: 'transactions.errors.insufficientFunds',
                defaultMessage: 'Insufficient funds in {account} account'
              },
              { account: source.account }
            ),
            metadata: {
              account
            }
          })
          setOpen(true)
          return true
        }

        if (Number(balance.available) < Number(source.value)) {
          add(`source.${source.account}`, {
            message: intl.formatMessage(
              {
                id: 'transactions.errors.insufficientFunds',
                defaultMessage: 'Insufficient funds in {account} account'
              },
              { account: source.account }
            ),
            metadata: {
              account
            }
          })
          setOpen(true)
          return true
        }

        remove(`source.${source.account}`)
        setOpen(false)
        return false
      })
      .some((error) => error)
  }

  const validate = (onValid?: (values?: TransactionFormSchema) => void) => {
    return (values: TransactionFormSchema) => {
      const errors = [
        totalSumSourceRule(values),
        totalSumDestinationRule(values),
        insufficientFundsRule(values, accounts)
      ]

      if (errors.some((error) => error)) {
        return
      }

      onValid?.(values)
    }
  }

  React.useEffect(() => {
    dataLoss(values)
  }, [mode, source?.length, destination?.length])

  React.useEffect(() => {
    totalSumSourceRule(values)
    totalSumDestinationRule(values)
  }, [value, sum(source), sum(destination)])

  return { errors, open, setOpen, validate, add, remove }
}

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

  const total = (source: TransactionSourceFormSchema) =>
    source.reduce((account, current) => account + Number(current.value), 0)

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
    const totalSource = total(values.source)

    if (value !== totalSource) {
      add('debit', {
        message: intl.formatMessage({
          id: 'transactions.errors.debit',
          defaultMessage:
            'The total of the debits differs from the transaction amount'
        })
      })
      return true
    }

    remove('debit')
    return false
  }

  const totalSumDestinationRule = (values: TransactionFormSchema) => {
    const value = Number(values.value)
    const totalDestination = total(values.destination)

    if (value !== totalDestination) {
      add('credit', {
        message: intl.formatMessage({
          id: 'transactions.errors.credit',
          defaultMessage:
            'The total of the credits differs from the transaction amount'
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
        if (source.accountAlias.includes(externalAccountAliasPrefix)) {
          return false
        }

        const account = accounts[source.accountAlias]
        if (!account) {
          return false
        }

        const balance = account.balances?.find(
          (balance) => balance.assetCode === asset
        )

        if (!balance) {
          add(`source.${source.accountAlias}`, {
            message: intl.formatMessage(
              {
                id: 'transactions.errors.insufficientFunds',
                defaultMessage: 'Insufficient funds in {account} account'
              },
              { account: source.accountAlias }
            ),
            metadata: {
              account
            }
          })
          setOpen(true)
          return true
        }

        if (Number(balance.available) < Number(source.value)) {
          add(`source.${source.accountAlias}`, {
            message: intl.formatMessage(
              {
                id: 'transactions.errors.insufficientFunds',
                defaultMessage: 'Insufficient funds in {account} account'
              },
              { account: source.accountAlias }
            ),
            metadata: {
              account
            }
          })
          setOpen(true)
          return true
        }

        remove(`source.${source.accountAlias}`)
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
  }, [value, total(source), total(destination)])

  return { errors, open, setOpen, validate, add, remove }
}

import { useEffect, useState } from 'react'
import { useIntl } from 'react-intl'
import { TransactionFormSchema, TransactionSourceFormSchema } from './schemas'

export type TransactionFormErrors = Record<string, string>

export const useTransactionFormErrors = (values: TransactionFormSchema) => {
  const intl = useIntl()
  const [errors, setErrors] = useState<TransactionFormErrors>({})
  const { value, source, destination } = values

  const sum = (source: TransactionSourceFormSchema) =>
    source.reduce((acc, curr) => acc + Number(curr.value), 0)

  const addError = (key: string, value: string) => {
    setErrors((prev) => ({ ...prev, [key]: value }))
  }

  const removeError = (key: string) => {
    setErrors((prev) => {
      const { [key]: _, ...rest } = prev
      return rest
    })
  }

  useEffect(() => {
    const v = Number(value)

    if (v !== sum(source)) {
      addError(
        'debit',
        intl.formatMessage({
          id: 'transactions.errors.debit',
          defaultMessage: 'Total Debits do not match total Credits'
        })
      )
    } else {
      removeError('debit')
    }

    if (v !== sum(destination)) {
      addError(
        'credit',
        intl.formatMessage({
          id: 'transactions.errors.debit',
          defaultMessage: 'Total Debits do not match total Credits'
        })
      )
    } else {
      removeError('credit')
    }
  }, [value, sum(source), sum(destination)])

  return { errors }
}

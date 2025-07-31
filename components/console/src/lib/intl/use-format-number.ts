import React from 'react'
import { useLocale } from './use-locale'

export function useFormatNumber() {
  const { locale } = useLocale()

  const separator = React.useMemo(
    () =>
      Intl.NumberFormat(locale)
        .formatToParts(1.1)
        .find((part) => part.type === 'decimal')?.value ?? '.',
    [locale]
  )

  const thousandSeparator = React.useMemo(
    () =>
      Intl.NumberFormat(locale)
        .formatToParts(1000)
        .find((part) => part.type === 'group')?.value ?? ',',
    [locale]
  )

  const formatNumber = React.useCallback(
    (value: string) => {
      if (typeof value !== 'string') {
        return value
      }

      const number = parseFloat(value)
      if (isNaN(number)) {
        return value
      }

      const [integer, decimal] = value.split('.')

      return (
        integer.replace(/\B(?=(\d{3})+(?!\d))/g, thousandSeparator) +
        (decimal ? separator + decimal : '')
      )
    },
    [separator]
  )

  return { formatNumber }
}

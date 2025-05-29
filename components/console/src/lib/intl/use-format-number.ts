import React from 'react'
import Decimal from 'decimal.js-light'
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

  const formatNumber = React.useCallback(
    (value: Decimal) => value.toString().replaceAll('.', separator),
    [separator]
  )

  return { formatNumber }
}

import { useLocale } from './use-locale'
import React from 'react'

export function useFormatCurrency() {
  const { locale } = useLocale()

  const formatCurrency = React.useCallback(
    (value: number, currency?: string) => {
      if (typeof value !== 'number') {
        return value
      }

      return new Intl.NumberFormat(locale, {
        style: 'currency',
        currency: currency || 'USD'
      }).format(value)
    },
    [locale]
  )

  return { formatCurrency }
} 
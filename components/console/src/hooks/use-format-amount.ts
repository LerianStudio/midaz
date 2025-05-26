import React from 'react'
import { useFormatNumber } from '@/lib/intl/use-format-number'
import { transactions as lib } from '@lerian/lib-commons-js'
import { AmountDto } from '@/core/application/dto/transaction-dto'

export function useFormatAmount() {
  const { formatNumber } = useFormatNumber()

  const formatAmount = React.useCallback(
    (amount: AmountDto) =>
      formatNumber(lib.undoScaleDecimal(amount.value, -amount.scale)),
    [formatNumber]
  )

  return { formatAmount }
}

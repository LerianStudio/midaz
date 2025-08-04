import React from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { TransactionReceiptItem } from '@/components/transactions/primitives/transaction-receipt'
import { FeeDisplayItem } from '@/hooks/use-fee-display'
import { useIntl } from 'react-intl'

interface FeeBreakdownCardProps {
  title?: string
  items: FeeDisplayItem[]
  showSeparators?: boolean
  className?: string
}

export const FeeBreakdownCard: React.FC<FeeBreakdownCardProps> = ({
  title,
  items,
  showSeparators = true,
  className
}) => {
  const intl = useIntl()
  const defaultTitle = intl.formatMessage({
    id: 'transactions.fees.breakdown',
    defaultMessage: 'Fee Breakdown'
  })

  const originalAmountItem = items.find(
    (item) => !item.isFee && item.label.includes('Original')
  )
  const feeItems = items.filter((item) => item.isFee)
  const summaryItems = items.filter(
    (item) => !item.isFee && !item.label.includes('Original')
  )

  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle>{title || defaultTitle}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Original Amount */}
        {originalAmountItem && (
          <>
            <TransactionReceiptItem
              label={originalAmountItem.label}
              value={originalAmountItem.value}
              className={originalAmountItem.className}
            />
            {showSeparators && feeItems.length > 0 && <Separator />}
          </>
        )}

        {/* Fee Items */}
        {feeItems.length > 0 && (
          <div className="space-y-2">
            {feeItems.map((item, index) => (
              <TransactionReceiptItem
                key={`fee-${index}`}
                label={item.label}
                value={item.value}
                className={item.className}
              />
            ))}
          </div>
        )}

        {/* Summary Items */}
        {summaryItems.length > 0 && (
          <>
            {showSeparators && <Separator />}
            <div className="space-y-2">
              {summaryItems.map((item, index) => (
                <TransactionReceiptItem
                  key={`summary-${index}`}
                  label={item.label}
                  value={item.value}
                  className={item.className}
                />
              ))}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

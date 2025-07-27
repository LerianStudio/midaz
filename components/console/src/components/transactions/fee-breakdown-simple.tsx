import React from 'react'
import { TransactionDto } from '@/core/application/dto/transaction-dto'
import {
  useFeeCalculations,
  getIsDeductibleFrom
} from '@/hooks/use-fee-calculations'
import { useFeeDisplay } from '@/hooks/use-fee-display'
import { FeeBreakdownCard } from './fee-breakdown-card'
import { useOrganization } from '@/providers/organization-provider'
import { useIntl } from 'react-intl'

interface FeeBreakdownProps {
  transaction: TransactionDto | any
  originalAmount?: number
  showCard?: boolean
  className?: string
}

export const FeeBreakdownSimple: React.FC<FeeBreakdownProps> = ({
  transaction,
  originalAmount: _originalAmount,
  showCard = true,
  className
}) => {
  const intl = useIntl()
  const organization = useOrganization()
  const currency =
    (organization as any)?.selectedOrganization?.defaultAssetCode || 'BRL'

  const isDeductibleFromValue = getIsDeductibleFrom(transaction)

  const feeCalculation = useFeeCalculations(transaction, isDeductibleFromValue)

  const displayItems = useFeeDisplay(
    feeCalculation.originalAmount,
    feeCalculation.totalFees,
    feeCalculation.appliedFees,
    feeCalculation.deductibleFees,
    feeCalculation.nonDeductibleFees,
    feeCalculation.senderPaysAmount,
    feeCalculation.recipientReceivesAmount,
    currency
  )

  if (feeCalculation.warnings.length > 0) {
    console.warn('Fee calculation warnings:', feeCalculation.warnings)
  }

  if (!feeCalculation.hasValidFees) {
    const noFeesMessage = intl.formatMessage({
      id: 'transactions.fees.none',
      defaultMessage: 'No fees'
    })

    if (!showCard) {
      return <p className="text-muted-foreground">{noFeesMessage}</p>
    }

    return (
      <FeeBreakdownCard
        items={[
          {
            label: noFeesMessage,
            value: '',
            className: 'text-muted-foreground'
          }
        ]}
        className={className}
      />
    )
  }

  if (showCard) {
    return <FeeBreakdownCard items={displayItems} className={className} />
  }

  return (
    <div className={`space-y-2 ${className}`}>
      {displayItems.map((item, index) => (
        <div key={index} className="flex justify-between">
          <span className="text-sm">{item.label}</span>
          <span className={`text-sm ${item.className || ''}`}>
            {item.value}
          </span>
        </div>
      ))}
    </div>
  )
}

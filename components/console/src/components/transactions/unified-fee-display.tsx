import React from 'react'
import { useIntl, defineMessages } from 'react-intl'
import { TransactionReceiptItem } from '@/components/transactions/primitives/transaction-receipt'
import { Separator } from '@/components/ui/separator'
import { FeeCalculationState } from '@/types/fee-calculation.types'

interface UnifiedFeeDisplayProps {
  feeState: FeeCalculationState
  showExplanations?: boolean
}

const feeMessages = defineMessages({
  feeDeductedFromRecipient: {
    id: 'transactions.fees.deductedFromDestination',
    defaultMessage: 'Fee deducted from recipient'
  },
  feeChargedToSender: {
    id: 'transactions.fees.chargedToSource',
    defaultMessage: 'Fee charged to sender'
  },
  senderPays: {
    id: 'fees.sourcePays',
    defaultMessage: 'Sender pays'
  },
  recipientReceives: {
    id: 'fees.destinationReceives',
    defaultMessage: 'Recipient receives'
  },
  mixedFeesExplanation: {
    id: 'transactions.breakdown.mixedFeesExplanation',
    defaultMessage: '{deductible} deducted, {nonDeductible} charged'
  },
  deductibleOnlyExplanation: {
    id: 'transactions.breakdown.deductibleOnlyExplanation',
    defaultMessage: 'All fees deducted from recipient'
  },
  chargedOnlyExplanation: {
    id: 'transactions.breakdown.chargedOnlyExplanation',
    defaultMessage: 'All fees charged to sender'
  }
})

const formatAmount = (value: number, currency: string) => {
  return `${currency} ${value.toFixed(2)}`
}

export const UnifiedFeeDisplay: React.FC<UnifiedFeeDisplayProps> = ({
  feeState,
  showExplanations = true
}) => {
  const intl = useIntl()

  if (feeState.totalFees <= 0) {
    return null
  }

  const {
    originalCurrency,
    deductibleFees,
    nonDeductibleFees,
    appliedFees,
    senderPaysAmount,
    recipientReceivesAmount
  } = feeState

  return (
    <React.Fragment>
      <Separator orientation="horizontal" />

      {/* Individual fee breakdown - Show first */}
      {appliedFees.map((fee) => (
        <TransactionReceiptItem
          key={fee.feeId}
          label={fee.feeLabel}
          value={
            <span>
              + {formatAmount(fee.calculatedAmount, originalCurrency)}
            </span>
          }
        />
      ))}

      {/* Show deductible fees total if any */}
      {deductibleFees > 0 && (
        <TransactionReceiptItem
          label={intl.formatMessage(feeMessages.feeDeductedFromRecipient)}
          value={
            <span className="font-medium text-red-600">
              {formatAmount(deductibleFees, originalCurrency)}
            </span>
          }
        />
      )}

      {/* Show non-deductible fees total if any */}
      {nonDeductibleFees > 0 && (
        <TransactionReceiptItem
          label={intl.formatMessage(feeMessages.feeChargedToSender)}
          value={
            <span className="font-medium text-blue-600">
              {formatAmount(nonDeductibleFees, originalCurrency)}
            </span>
          }
        />
      )}

      <Separator orientation="horizontal" />

      {/* Final amounts */}
      <TransactionReceiptItem
        label={intl.formatMessage(feeMessages.senderPays)}
        value={
          <span className="font-medium text-neutral-700">
            {formatAmount(senderPaysAmount, originalCurrency)}
          </span>
        }
      />

      <TransactionReceiptItem
        label={intl.formatMessage(feeMessages.recipientReceives)}
        value={
          <span className="font-medium text-green-600">
            {formatAmount(recipientReceivesAmount, originalCurrency)}
          </span>
        }
      />

      {/* Explanations */}
      {showExplanations && (
        <>
          {/* Mixed scenario explanation */}
          {deductibleFees > 0 && nonDeductibleFees > 0 && (
            <TransactionReceiptItem
              label=""
              value={
                <span className="max-w-md text-xs text-gray-500 italic">
                  {intl.formatMessage(feeMessages.mixedFeesExplanation, {
                    deductible: formatAmount(deductibleFees, originalCurrency),
                    nonDeductible: formatAmount(
                      nonDeductibleFees,
                      originalCurrency
                    )
                  })}
                </span>
              }
            />
          )}

          {/* Single fee type explanations */}
          {deductibleFees > 0 && nonDeductibleFees === 0 && (
            <TransactionReceiptItem
              label=""
              value={
                <span className="max-w-md text-xs text-gray-500 italic">
                  {intl.formatMessage(feeMessages.deductibleOnlyExplanation)}
                </span>
              }
            />
          )}

          {deductibleFees === 0 && nonDeductibleFees > 0 && (
            <TransactionReceiptItem
              label=""
              value={
                <span className="text-xs text-gray-500 italic">
                  {intl.formatMessage(feeMessages.chargedOnlyExplanation)}
                </span>
              }
            />
          )}
        </>
      )}
    </React.Fragment>
  )
}

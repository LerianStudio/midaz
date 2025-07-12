import React, { useMemo } from 'react'
import { defineMessages, useIntl } from 'react-intl'
import {
  TransactionDto,
  TransactionOperationDto
} from '@/core/application/dto/transaction-dto'
import { TransactionReceiptItem } from '@/components/transactions/primitives/transaction-receipt'
import { Separator } from '@/components/ui/separator'

type FeeBreakdownProps = {
  transaction: TransactionDto | any // Allow fee service response format
}

type AppliedFee = {
  feeLabel: string
  calculatedAmount: string
}

const feeMessages = defineMessages({
  totalFees: {
    id: 'transactions.fees.total',
    defaultMessage: 'Total Fees'
  },
  noFees: {
    id: 'transactions.fees.none',
    defaultMessage: 'No fees'
  },
  finalAmount: {
    id: 'transactions.fees.finalAmount',
    defaultMessage: 'Final Amount'
  }
})

const isFeeOperation = (operation: TransactionOperationDto): boolean => {
  const descriptionLowerCase = operation.description?.toLowerCase() ?? ''
  const chartOfAccountsLowerCase = (
    operation.chartOfAccounts ?? ''
  ).toLowerCase()
  const accountAliasLowerCase = (operation.accountAlias ?? '').toLowerCase()

  return (
    descriptionLowerCase.includes('fee') ||
    chartOfAccountsLowerCase.includes('fee') ||
    accountAliasLowerCase.includes('fee')
  )
}

const formatAmount = (value?: string | number) => {
  if (!value) return '0.00'
  const num = Number(value)
  return Number.isNaN(num) ? '0.00' : num.toFixed(2)
}

const isFeesResponse = (data: any): boolean => {
  return data && 'transaction' in data && data.transaction?.send
}

export const FeeBreakdown: React.FC<FeeBreakdownProps> = ({ transaction }) => {
  const intl = useIntl()

  const { originalAmount, totalFees, appliedFees } = useMemo(() => {
    if (!transaction) {
      return {
        originalAmount: 0,
        totalFees: 0,
        appliedFees: []
      }
    }

    const isFeeResponse = isFeesResponse(transaction)

    if (isFeeResponse) {
      const feeData = (transaction as any).transaction
      const sourceOperations = feeData.send?.source?.from || []
      const destinationOperations = feeData.send?.distribute?.to || []

      if (sourceOperations.length === 0 || destinationOperations.length === 0) {
        return {
          originalAmount: 0,
          totalFees: 0,
          appliedFees: []
        }
      }

      const enhancedTotalValue = Number(feeData.send?.value || 0)

      const originalDestinationOperation = destinationOperations.find(
        (operation: any) =>
          operation.amount?.asset === feeData.send?.asset &&
          operation.amount?.operation === 'CREDIT' &&
          !operation.metadata?.source &&
          !operation.accountAlias?.toLowerCase().includes('fee') &&
          !operation.description?.toLowerCase().includes('fee')
      )

      let originalAmount = 0
      if (originalDestinationOperation) {
        originalAmount = Number(originalDestinationOperation.amount?.value || 0)
      } else {
        const largestDestinationOperation = destinationOperations.reduce(
          (largest: any, current: any) => {
            const currentValue = Number(current.amount?.value || 0)
            const largestValue = Number(largest?.amount?.value || 0)
            return currentValue > largestValue ? current : largest
          },
          null
        )
        originalAmount = largestDestinationOperation
          ? Number(largestDestinationOperation.amount?.value || 0)
          : sourceOperations.reduce(
              (sum: number, operation: any) =>
                sum + Number(operation.amount?.value || 0),
              0
            )
      }

      const totalFeesAmount = enhancedTotalValue - originalAmount

      const feeOperations = destinationOperations.filter((operation: any) => {
        if (operation === originalDestinationOperation) return false
        return (
          operation.metadata?.source ||
          operation.amount?.asset !== feeData.send?.asset ||
          operation.accountAlias?.toLowerCase().includes('fee') ||
          operation.description?.toLowerCase().includes('fee')
        )
      })

      const appliedFeesList: AppliedFee[] = feeOperations.map(
        (operation: any) => ({
          feeLabel:
            operation.description ||
            (operation.metadata?.source
              ? `${operation.metadata.source} Fee`
              : 'Fee') ||
            `${operation.amount?.asset} Fee`,
          calculatedAmount: operation.amount?.value || '0'
        })
      )

      if (
        feeOperations.length === 0 ||
        (destinationOperations.length === 1 &&
          sourceOperations.length === 1 &&
          totalFeesAmount === 0)
      ) {
        return {
          originalAmount: enhancedTotalValue,
          totalFees: 0,
          appliedFees: []
        }
      }

      return {
        originalAmount,
        totalFees: totalFeesAmount,
        appliedFees: appliedFeesList
      }
    } else {
      // Standard TransactionDto format
      const { source, destination } = transaction
      if (!source || !destination) {
        return {
          originalAmount: 0,
          totalFees: 0,
          appliedFees: []
        }
      }

      const sourceTotal = source.reduce(
        (sum: number, operation: TransactionOperationDto) =>
          sum + Number(operation.amount),
        0
      )
      const destinationTotal = destination.reduce(
        (sum: number, operation: TransactionOperationDto) =>
          sum + Number(operation.amount),
        0
      )

      const feeOperations = destination.filter(isFeeOperation)

      const appliedFeesList: AppliedFee[] = feeOperations.map(
        (operation: TransactionOperationDto) => ({
          feeLabel: operation.description ?? operation.accountAlias ?? 'Fee',
          calculatedAmount: operation.amount
        })
      )

      const totalFeesAmount = destinationTotal - sourceTotal

      return {
        originalAmount: sourceTotal,
        totalFees: totalFeesAmount,
        appliedFees: appliedFeesList
      }
    }
  }, [transaction])

  const asset = isFeesResponse(transaction)
    ? (transaction as any).transaction?.send?.asset || 'USD'
    : transaction.asset || 'USD'

  const formatFeeAmount = (amount: number) => {
    return amount > 0
      ? `+ ${asset} ${formatAmount(amount)}`
      : `(${intl.formatMessage(feeMessages.noFees)})`
  }

  const formatFinalAmount = (original: number, fees: number) => {
    return `${asset} ${formatAmount(original + fees)}`
  }

  if (!transaction) {
    return null
  }

  const shouldHideBreakdown =
    !isFeesResponse(transaction) && (appliedFees.length === 0 || totalFees <= 0)

  if (shouldHideBreakdown) {
    return null
  }

  return (
    <React.Fragment>
      <TransactionReceiptItem
        label={intl.formatMessage(feeMessages.totalFees)}
        value={
          <span className={totalFees > 0 ? 'text-blue-500' : 'text-green-500'}>
            {formatFeeAmount(totalFees)}
          </span>
        }
      />
      <TransactionReceiptItem
        label={intl.formatMessage(feeMessages.finalAmount)}
        value={
          <span className="font-semibold text-zinc-700">
            {formatFinalAmount(originalAmount, totalFees)}
          </span>
        }
      />
      <Separator orientation="horizontal" />
    </React.Fragment>
  )
}

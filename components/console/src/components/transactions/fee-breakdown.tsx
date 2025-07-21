import React, { useMemo } from 'react'
import { defineMessages, useIntl } from 'react-intl'
import {
  TransactionDto,
  TransactionOperationDto
} from '@/core/application/dto/transaction-dto'
import { TransactionReceiptItem } from '@/components/transactions/primitives/transaction-receipt'
import { Separator } from '@/components/ui/separator'

type FeeBreakdownProps = {
  transaction: TransactionDto | any
  originalAmount?: number
}

type AppliedFee = {
  feeLabel: string
  calculatedAmount: string
}

const feeMessages = defineMessages({
  originalAmount: {
    id: 'transactions.fees.originalAmount',
    defaultMessage: 'Original amount'
  },
  totalFees: {
    id: 'transactions.fees.total',
    defaultMessage: 'Total Fees'
  },
  finalAmount: {
    id: 'transactions.fees.finalAmount',
    defaultMessage: 'Transaction final amount'
  },
  senderPays: {
    id: 'transactions.fees.senderPays',
    defaultMessage: 'Amount sender pays'
  },
  recipientReceives: {
    id: 'transactions.fees.recipientReceives',
    defaultMessage: 'Amount recipient receives'
  },
  feeDeductedFromRecipient: {
    id: 'transactions.fees.deductedFromRecipient',
    defaultMessage: 'Fee deducted from recipient'
  },
  feeChargedToSender: {
    id: 'transactions.fees.chargedToSender',
    defaultMessage: 'Fee charged to sender'
  },
  noFees: {
    id: 'transactions.fees.none',
    defaultMessage: 'No fees'
  }
})

const isFeeOperation = (
  operation: TransactionOperationDto,
  transaction?: TransactionDto | { transaction: any }
): boolean => {
  const descriptionLowerCase = operation.description?.toLowerCase() ?? ''
  const chartOfAccountsLowerCase = (
    operation.chartOfAccounts ?? ''
  ).toLowerCase()
  const accountAliasLowerCase = (operation.accountAlias ?? '').toLowerCase()

  return (
    descriptionLowerCase.includes('fee') ||
    chartOfAccountsLowerCase.includes('fee') ||
    accountAliasLowerCase.includes('fee') ||
    Boolean(
      transaction &&
        'source' in transaction &&
        transaction.source?.[0]?.accountAlias === operation.accountAlias
    )
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

const getIsDeductibleFrom = (transaction: any): boolean => {
  if (!isFeesResponse(transaction)) return false

  const feeData = transaction.transaction
  return feeData.isDeductibleFrom === true
}

export const FeeBreakdown: React.FC<FeeBreakdownProps> = ({
  transaction,
  originalAmount: providedOriginalAmount
}) => {
  const intl = useIntl()

  const {
    originalAmount,
    totalFees,
    appliedFees,
    isDeductibleFrom,
    feeCollector,
    sourceAccount,
    destinationAccount
  } = useMemo(() => {
    if (!transaction) {
      return {
        originalAmount: 0,
        totalFees: 0,
        appliedFees: [],
        isDeductibleFrom: false,
        feeCollector: null,
        sourceAccount: null,
        destinationAccount: null
      }
    }

    const isFeeResponse = isFeesResponse(transaction)

    if (isFeeResponse) {
      const feeData = (transaction as any).transaction
      const sourceOperations = feeData.send?.source?.from || []
      const destinationOperations = feeData.send?.distribute?.to || []
      const isDeductibleFromFlag = getIsDeductibleFrom(transaction)

      if (sourceOperations.length === 0 || destinationOperations.length === 0) {
        return {
          originalAmount: 0,
          totalFees: 0,
          appliedFees: [],
          isDeductibleFrom: isDeductibleFromFlag,
          feeCollector: null,
          sourceAccount: null,
          destinationAccount: null
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

      const originalAmount = providedOriginalAmount ?? enhancedTotalValue
      const operations = destinationOperations
      const mainRecipient = operations.find(
        (operation: any) =>
          !operation.metadata?.source &&
          operation.accountAlias !== feeData.send.source.from[0]?.accountAlias
      )

      const feeOperations = operations.filter(
        (operation: any) =>
          operation.metadata?.source ||
          operation.accountAlias === feeData.send.source.from[0]?.accountAlias
      )

      const recipientReceives = mainRecipient
        ? Number(mainRecipient.amount.value)
        : originalAmount
      const totalFeesAmount = feeOperations.reduce(
        (accumulator: number, operation: any) =>
          accumulator + Number(operation.amount.value),
        0
      )

      const isActuallyDeductible = recipientReceives < originalAmount

      const sourceAccount = feeData.send.source.from[0]?.accountAlias
      const destinationAccount = mainRecipient?.accountAlias
      const feeCollector = feeOperations[0]?.accountAlias

      const appliedFeesList: AppliedFee[] = feeOperations.map(
        (operation: any) => ({
          feeLabel:
            operation.description ||
            `Fee collected by ${operation.accountAlias}`,
          calculatedAmount: operation.amount?.value || '0'
        })
      )

      if (totalFeesAmount === 0) {
        return {
          originalAmount,
          totalFees: 0,
          appliedFees: [],
          isDeductibleFrom: false,
          feeCollector: null,
          sourceAccount: feeData.send.source.from[0]?.accountAlias,
          destinationAccount: mainRecipient?.accountAlias
        }
      }

      return {
        originalAmount,
        totalFees: totalFeesAmount,
        appliedFees: appliedFeesList,
        isDeductibleFrom: isActuallyDeductible,
        feeCollector,
        sourceAccount,
        destinationAccount
      }
    } else {
      const { source, destination } = transaction as TransactionDto
      if (!source || !destination) {
        return {
          originalAmount: 0,
          totalFees: 0,
          appliedFees: [],
          isDeductibleFrom: false,
          feeCollector: null,
          sourceAccount: null,
          destinationAccount: null
        }
      }

      const originalAmount =
        providedOriginalAmount ?? Number((transaction as TransactionDto).amount)

      const sourceAccountAlias = source[0]?.accountAlias
      const feeOperations = destination.filter(
        (operation: TransactionOperationDto) =>
          isFeeOperation(operation, transaction) ||
          operation.accountAlias === sourceAccountAlias
      )

      const mainDestinationOps = destination.filter(
        (operation: TransactionOperationDto) =>
          !isFeeOperation(operation, transaction) &&
          operation.accountAlias !== sourceAccountAlias
      )

      const recipientReceives = mainDestinationOps.reduce(
        (accumulator: number, operation: TransactionOperationDto) =>
          accumulator + Number(operation.amount),
        0
      )

      const totalFeesAmount = feeOperations.reduce(
        (accumulator: number, operation: TransactionOperationDto) =>
          accumulator + Number(operation.amount),
        0
      )

      const isDeductibleFromDetected =
        recipientReceives < originalAmount && totalFeesAmount > 0

      const appliedFeesList: AppliedFee[] = feeOperations.map(
        (operation: TransactionOperationDto) => ({
          feeLabel:
            operation.description ||
            `Fee collected by ${operation.accountAlias}`,
          calculatedAmount: operation.amount
        })
      )

      const sourceAccount = source[0]?.accountAlias
      const destinationAccount = mainDestinationOps[0]?.accountAlias
      const feeCollector = feeOperations[0]?.accountAlias

      return {
        originalAmount,
        totalFees: totalFeesAmount,
        appliedFees: appliedFeesList,
        isDeductibleFrom: isDeductibleFromDetected,
        feeCollector,
        sourceAccount,
        destinationAccount
      }
    }
  }, [transaction])

  const asset = isFeesResponse(transaction)
    ? (transaction as any).transaction?.send?.asset || 'USD'
    : (transaction as TransactionDto).asset || 'USD'

  const formatFeeAmount = (amount: number) => {
    return amount > 0
      ? `+ ${asset} ${formatAmount(amount)}`
      : `(${intl.formatMessage(feeMessages.noFees)})`
  }

  if (!transaction) {
    return null
  }

  const shouldHideBreakdown =
    !isFeesResponse(transaction) && (appliedFees.length === 0 || totalFees <= 0)

  if (shouldHideBreakdown) {
    return null
  }

  const originalAmountNumber = Number(originalAmount)
  const totalFeesNumber = Number(totalFees)

  const senderPaysAmount =
    feeCollector === sourceAccount
      ? originalAmountNumber + totalFeesNumber
      : originalAmountNumber

  const recipientReceivesAmount =
    feeCollector === destinationAccount
      ? isDeductibleFrom
        ? originalAmountNumber - totalFeesNumber // Destination receives less (fee deducted)
        : originalAmountNumber - totalFeesNumber // Destination pays fee from what they receive
      : originalAmountNumber // Destination gets full amount

  return (
    <React.Fragment>
      <Separator orientation="horizontal" />

      <TransactionReceiptItem
        label={intl.formatMessage({
          id: 'transactions.fees.description',
          defaultMessage:
            feeCollector === sourceAccount
              ? 'Fee paid by sender'
              : feeCollector === destinationAccount
                ? isDeductibleFrom
                  ? 'Fee deducted from transaction'
                  : 'Fee paid by destination'
                : isDeductibleFrom
                  ? 'Fee deducted from transaction'
                  : 'Fee added to total cost'
        })}
        value={
          <span className="font-medium text-blue-600">
            {asset} {formatAmount(totalFees)}
          </span>
        }
      />

      {appliedFees.map((fee, index) => (
        <TransactionReceiptItem
          key={index}
          label={fee.feeLabel}
          value={
            <span className="text-blue-600">
              + {asset} {formatAmount(Number(fee.calculatedAmount))}
            </span>
          }
        />
      ))}

      <Separator orientation="horizontal" />

      <TransactionReceiptItem
        label={intl.formatMessage({
          id: 'transactions.breakdown.senderPays',
          defaultMessage:
            feeCollector === sourceAccount
              ? 'Sender pays (including fees)'
              : 'Sender sends (original amount)'
        })}
        value={
          <span className="font-medium text-neutral-700">
            {asset} {formatAmount(senderPaysAmount)}
          </span>
        }
      />

      <TransactionReceiptItem
        label={intl.formatMessage({
          id: 'transactions.breakdown.destinationReceives',
          defaultMessage:
            feeCollector === destinationAccount
              ? isDeductibleFrom
                ? 'Destination receives (after fee deduction)'
                : 'Destination pays fee and receives'
              : isDeductibleFrom
                ? 'Destination receives (reduced amount)'
                : 'Destination receives (full amount)'
        })}
        value={
          <span
            className={
              isDeductibleFrom
                ? 'font-medium text-orange-600'
                : 'font-medium text-green-600'
            }
          >
            {asset} {formatAmount(recipientReceivesAmount)}
          </span>
        }
      />

      {isDeductibleFrom && (
        <TransactionReceiptItem
          label=""
          value={
            <span className="text-xs text-gray-500 italic">
              {intl.formatMessage({
                id: 'transactions.breakdown.deductibleExplanation',
                defaultMessage: 'Fee was deducted from the transaction amount'
              })}
            </span>
          }
        />
      )}
    </React.Fragment>
  )
}

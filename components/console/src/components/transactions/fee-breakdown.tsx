import React, { useMemo } from 'react'
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

const formatAmount = (value?: string) => {
  if (!value) return '0.00'
  const num = Number(value)
  if (Number.isNaN(num)) return value
  return num.toFixed(2)
}

export const FeeBreakdown: React.FC<FeeBreakdownProps> = ({ transaction }) => {
  const { originalAmount, totalFees, appliedFees } = useMemo(() => {
    // Handle both standard TransactionDto and fee service response formats
    const isFeesResponse =
      transaction &&
      'transaction' in transaction &&
      (transaction as any).transaction?.send

    if (isFeesResponse) {
      // Fee service response format
      const feeTransaction = (transaction as any).transaction
      const sourceOps = feeTransaction.send?.source?.from || []
      const destinationOps = feeTransaction.send?.distribute?.to || []

      console.log('Fee service response analysis:')
      console.log('Source ops:', sourceOps)
      console.log('Destination ops:', destinationOps)

      // The fee service enhances the transaction by adding fees
      // We need to detect what was added compared to the original amount
      const enhancedTotalValue = Number(feeTransaction.send?.value || 0)

      // Find the original transaction amount from the main destination operation
      // The original amount should be the largest USD operation to a different account
      const originalDestinationOp = destinationOps.find(
        (op: any) =>
          op.amount?.asset === feeTransaction.send?.asset &&
          op.amount?.operation === 'CREDIT' &&
          !op.metadata?.source // This indicates it's not a fee or internal operation
      )

      const originalAmount = originalDestinationOp
        ? Number(originalDestinationOp.amount?.value || 0)
        : 0

      console.log('Enhanced total value:', enhancedTotalValue)
      console.log('Original amount:', originalAmount)

      // Calculate total fees as the difference
      const feesTotal = enhancedTotalValue - originalAmount

      console.log('Calculated fees total:', feesTotal)

      // Find fee operations - these are additional operations beyond the original transaction
      const feeOps = destinationOps.filter((op: any) => {
        // Skip the original destination operation
        if (op === originalDestinationOp) return false

        // Fee operations are typically:
        // 1. Operations with metadata indicating source
        // 2. Operations in different currencies
        // 3. Operations that are not the main transfer
        return (
          op.metadata?.source ||
          op.amount?.asset !== feeTransaction.send?.asset ||
          op.accountAlias?.toLowerCase().includes('fee') ||
          op.description?.toLowerCase().includes('fee')
        )
      })

      const fees: AppliedFee[] = feeOps.map((op: any) => ({
        feeLabel:
          op.description ||
          (op.metadata?.source ? `${op.metadata.source} Fee` : 'Fee') ||
          `${op.amount?.asset} Fee`,
        calculatedAmount: op.amount?.value || '0'
      }))

      return {
        originalAmount,
        totalFees: feesTotal,
        appliedFees: fees
      }
    } else {
      // Standard TransactionDto format
      if (!transaction.source || !transaction.destination) {
        return {
          originalAmount: 0,
          totalFees: 0,
          appliedFees: []
        }
      }

      const sourceTotal = transaction.source.reduce(
        (sum: number, op: TransactionOperationDto) => sum + Number(op.amount),
        0
      )
      const destinationTotal = transaction.destination.reduce(
        (sum: number, op: TransactionOperationDto) => sum + Number(op.amount),
        0
      )

      const feeOps = transaction.destination.filter(isFeeOperation)

      const fees: AppliedFee[] = feeOps.map((op: TransactionOperationDto) => ({
        feeLabel: op.description ?? op.accountAlias ?? 'Fee',
        calculatedAmount: op.amount
      }))

      const feesTotal = destinationTotal - sourceTotal

      return {
        originalAmount: sourceTotal,
        totalFees: feesTotal,
        appliedFees: fees
      }
    }
  }, [transaction])

  console.log('FeeBreakdown computed values:', {
    originalAmount,
    totalFees,
    appliedFees
  })

  const getAsset = () => {
    const isFeesResponse =
      transaction &&
      'transaction' in transaction &&
      (transaction as any).transaction?.send
    if (isFeesResponse) {
      return (transaction as any).transaction?.send?.asset || 'USD'
    }
    return transaction.asset || 'USD'
  }

  if (appliedFees.length === 0 || totalFees === 0) {
    return null
  }

  return (
    <React.Fragment>
      <TransactionReceiptItem
        label="Total Fees"
        value={
          <span className="text-blue-500">
            + {getAsset()} {formatAmount(totalFees.toString())}
          </span>
        }
      />
      <TransactionReceiptItem
        label="Final Amount"
        value={
          <span className="font-semibold text-zinc-700">
            {getAsset()} {formatAmount((originalAmount + totalFees).toString())}
          </span>
        }
      />
      <Separator orientation="horizontal" />
    </React.Fragment>
  )
}

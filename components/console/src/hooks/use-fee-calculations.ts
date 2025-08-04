import { useMemo } from 'react'
import { useIntl } from 'react-intl'
import {
  TransactionDto,
  TransactionOperationDto
} from '@/core/application/dto/transaction-dto'
import {
  createFeeValidationService,
  getTransactionAccounts
} from '@/utils/fee-validation'

export interface FeeCalculationResult {
  originalAmount: string
  totalFees: string
  appliedFees: AppliedFee[]
  deductibleFees: string
  nonDeductibleFees: string
  sourcePaysAmount: string
  destinationReceivesAmount: string
  afterFeesAmount: string
  hasValidFees: boolean
  warnings: string[]
}

export interface AppliedFee {
  feeLabel: string
  calculatedAmount: string
  isDeductibleFrom: boolean
  creditAccount: string
  priority?: number
}

interface FeeRule {
  feeId: string
  feeLabel: string
  isDeductibleFrom: boolean
  creditAccount: string
  priority: number
}

interface FeeOperation {
  accountAlias: string
  asset: string
  amount?: {
    value?: string
    [key: string]: any
  }
  description?: string
  chartOfAccounts?: string
  metadata?: {
    source?: string
    feeId?: string
    feeLabel?: string
    isDeductibleFrom?: boolean
    priority?: number
    [key: string]: any
  }
}

export const isFeesResponse = (data: any): boolean => {
  return data && 'transaction' in data && data.transaction?.send
}

export const getIsDeductibleFrom = (transaction: any): boolean => {
  if (!isFeesResponse(transaction)) return false
  const feeData = transaction.transaction
  return feeData.isDeductibleFrom === true
}

export const formatAmount = (value?: string | number): string => {
  if (!value) return '0.00'
  const num = Number(value)
  return Number.isNaN(num) ? '0.00' : num.toFixed(2)
}

export const isFeeOperation = (
  operation: TransactionOperationDto,
  _transaction?: TransactionDto | { transaction: any }
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
    Boolean(operation.metadata?.source) // Fee operations have metadata.source
  )
}

export const useFeeCalculations = (
  transaction: TransactionDto | any,
  isDeductibleFromValue?: boolean
): FeeCalculationResult => {
  const intl = useIntl()
  const feeValidationService = createFeeValidationService()

  return useMemo(() => {
    const warnings: string[] = []

    if (isFeesResponse(transaction)) {
      const feeData = transaction.transaction
      const sourceOperations = feeData.send?.source?.from || []
      const destinationOperations = feeData.send?.distribute?.to || []
      const feeRules: FeeRule[] = feeData.feeRules || []

      const originalAmount = formatAmount(feeData.send?.value || '0')

      const mainRecipients = destinationOperations.filter(
        (op: FeeOperation) => !op.metadata?.source
      )
      const transactionAccounts = getTransactionAccounts(
        sourceOperations,
        mainRecipients
      )

      const allFeeOperations = destinationOperations.filter(
        (op: FeeOperation) => op.metadata?.source
      )
      const validFeeOperations = feeValidationService.filterValidFeeOperations(
        allFeeOperations,
        transactionAccounts
      )

      let deductibleTotal = 0
      let nonDeductibleTotal = 0
      const appliedFees: AppliedFee[] = []

      validFeeOperations.forEach((feeOp: any) => {
        const feeAmount = Number(feeOp.amount?.value || '0')
        const matchedRule = feeRules.find(
          (rule: FeeRule) =>
            rule.creditAccount === feeOp.accountAlias ||
            rule.creditAccount.replace('@', '') ===
              feeOp.accountAlias.replace('@', '')
        )

        const isDeductible =
          matchedRule?.isDeductibleFrom ||
          feeOp.metadata?.isDeductibleFrom ||
          false

        if (isDeductible) {
          deductibleTotal += feeAmount
        } else {
          nonDeductibleTotal += feeAmount
        }

        appliedFees.push({
          feeLabel:
            matchedRule?.feeLabel ||
            feeOp.metadata?.feeLabel ||
            intl.formatMessage(
              {
                id: 'transactions.fees.collectedBy',
                defaultMessage: 'Fee collected by {accountAlias}'
              },
              { accountAlias: feeOp.accountAlias }
            ),
          calculatedAmount: formatAmount(feeAmount),
          isDeductibleFrom: isDeductible,
          creditAccount: feeOp.accountAlias || '',
          priority: matchedRule?.priority || feeOp.metadata?.priority || 0
        })
      })

      appliedFees.sort((a, b) => (a.priority || 0) - (b.priority || 0))

      const totalFees = deductibleTotal + nonDeductibleTotal
      const originalAmountNum = Number(originalAmount)

      return {
        originalAmount,
        totalFees: formatAmount(totalFees),
        appliedFees,
        deductibleFees: formatAmount(deductibleTotal),
        nonDeductibleFees: formatAmount(nonDeductibleTotal),
        sourcePaysAmount: formatAmount(originalAmountNum + nonDeductibleTotal),
        destinationReceivesAmount: formatAmount(
          originalAmountNum - deductibleTotal
        ),
        afterFeesAmount: formatAmount(originalAmountNum - deductibleTotal),
        hasValidFees: appliedFees.length > 0,
        warnings
      }
    }

    const sources = transaction.source || []
    const destinations = transaction.destination || []
    const operations = [...sources, ...destinations]

    const sourceTotal = sources.reduce(
      (sum: number, op: TransactionOperationDto) =>
        sum + Number(op.amount || 0),
      0
    )
    const destinationTotal = destinations.reduce(
      (sum: number, op: TransactionOperationDto) =>
        sum + Number(op.amount || 0),
      0
    )

    const feeOperations = operations.filter((op) =>
      isFeeOperation(op, transaction)
    )
    const totalFees = feeOperations.reduce(
      (sum: number, op: TransactionOperationDto) =>
        sum + Number(op.amount || 0),
      0
    )

    const appliedFees: AppliedFee[] = feeOperations.map((op) => ({
      feeLabel:
        op.description ||
        intl.formatMessage(
          {
            id: 'transactions.fees.collectedBy',
            defaultMessage: 'Fee collected by {accountAlias}'
          },
          { accountAlias: op.accountAlias || 'Unknown' }
        ),
      calculatedAmount: formatAmount(op.amount),
      isDeductibleFrom: isDeductibleFromValue || false,
      creditAccount: op.accountAlias || '',
      priority: 0
    }))

    const deductibleFees = isDeductibleFromValue ? totalFees : 0
    const nonDeductibleFees = isDeductibleFromValue ? 0 : totalFees

    return {
      originalAmount: formatAmount(sourceTotal),
      totalFees: formatAmount(totalFees),
      appliedFees,
      deductibleFees: formatAmount(deductibleFees),
      nonDeductibleFees: formatAmount(nonDeductibleFees),
      sourcePaysAmount: formatAmount(sourceTotal),
      destinationReceivesAmount: formatAmount(destinationTotal),
      afterFeesAmount: formatAmount(destinationTotal),
      hasValidFees: appliedFees.length > 0,
      warnings
    }
  }, [transaction, isDeductibleFromValue, intl, feeValidationService])
}

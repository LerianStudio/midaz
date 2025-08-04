import {
  createFeeValidationService,
  getTransactionAccounts
} from '@/utils/fee-validation'
import {
  FeeRuntimeValidator,
  FeePackageRule
} from '@/utils/fee-runtime-validation'
import { FeeCalculationState, AppliedFee } from '@/types/fee-calculation.types'

interface ValidationResult {
  isValid: boolean
  errors: string[]
  warnings: string[]
  validatedResponse: any
  preservedStructure: {
    hasMultipleSources: boolean
    hasMultipleDestinations: boolean
    hasDuplicateAccounts: boolean
    preservedOperations: {
      source: any[]
      destination: any[]
      fees: any[]
    }
  }
}

export function validateAndFilterFeeResponseRefactored(
  feeResponse: any,
  originalRequest: any
): ValidationResult {
  if (!feeResponse?.transaction) {
    return {
      isValid: true,
      errors: [],
      warnings: [],
      validatedResponse: feeResponse,
      preservedStructure: {
        hasMultipleSources: false,
        hasMultipleDestinations: false,
        hasDuplicateAccounts: false,
        preservedOperations: {
          source: [],
          destination: [],
          fees: []
        }
      }
    }
  }

  const errors: string[] = []
  const warnings: string[] = []

  const validatedResponse = JSON.parse(JSON.stringify(feeResponse))
  const feeData = validatedResponse.transaction

  let sourceOperations = feeData.send?.source?.from || []
  let destinationOperations = feeData.send?.distribute?.to || []

  const sourceAccounts = sourceOperations.map((op: any) => op.accountAlias)
  const uniqueSourceAccounts = new Set(sourceAccounts)
  const destAccounts = destinationOperations.map((op: any) => op.accountAlias)
  const uniqueDestAccounts = new Set(destAccounts)

  const hasMultipleSources = sourceOperations.length > 1
  const hasMultipleDestinations = destinationOperations.length > 1
  const hasDuplicateSources =
    sourceAccounts.length !== uniqueSourceAccounts.size
  const hasDuplicateDestinations =
    destAccounts.length !== uniqueDestAccounts.size

  const overlappingAccounts = Array.from(uniqueSourceAccounts).filter((acc) =>
    uniqueDestAccounts.has(acc)
  )

  if (overlappingAccounts.length > 0) {
    warnings.push(
      `Accounts appear in both source and destination: ${overlappingAccounts.join(', ')}`
    )
  }

  // Each operation represents a specific flow and should be maintained
  const preservedSourceOperations = sourceOperations.map(
    (op: any, index: number) => ({
      ...op,
      operationIndex: index,
      isDuplicate:
        sourceAccounts.filter((acc: string) => acc === op.accountAlias).length >
        1
    })
  )

  const feeOperations = destinationOperations.filter(
    (op: any) => op.metadata?.source
  )
  const nonFeeOperations = destinationOperations.filter(
    (op: any) => !op.metadata?.source
  )

  const feeValidationService = createFeeValidationService()
  const transactionAccounts = getTransactionAccounts(
    originalRequest.transaction?.source || [],
    originalRequest.transaction?.destination || []
  )

  const validFeeOperations = feeValidationService.filterValidFeeOperations(
    feeOperations,
    transactionAccounts
  )

  const invalidFeeCount = feeOperations.length - validFeeOperations.length
  if (invalidFeeCount > 0) {
    warnings.push(`${invalidFeeCount} invalid fee operations were filtered out`)
  }

  const preservedDestinationOperations = [
    ...nonFeeOperations.map((op: any, index: number) => ({
      ...op,
      operationIndex: index,
      operationType: 'destination',
      isDuplicate:
        destAccounts.filter((acc: string) => acc === op.accountAlias).length > 1
    })),
    ...validFeeOperations.map((op: any, index: number) => ({
      ...op,
      operationIndex: index + nonFeeOperations.length,
      operationType: 'fee',
      isDuplicate: false
    }))
  ]

  validatedResponse.transaction.send.source.from = preservedSourceOperations
  validatedResponse.transaction.send.distribute.to =
    preservedDestinationOperations

  const sourceTotal = preservedSourceOperations.reduce(
    (sum: number, op: any) => sum + parseFloat(op.amount?.value || '0'),
    0
  )

  const destinationTotal = preservedDestinationOperations.reduce(
    (sum: number, op: any) => sum + parseFloat(op.amount?.value || '0'),
    0
  )

  const balanceDifference = Math.abs(sourceTotal - destinationTotal)
  if (balanceDifference > 0.01) {
    errors.push(
      `Transaction is unbalanced: source total (${sourceTotal.toFixed(2)}) ` +
        `!= destination total (${destinationTotal.toFixed(2)})`
    )
  }

  if (originalRequest.transaction?.source) {
    originalRequest.transaction.source.forEach((originalSource: any) => {
      const matchingOps = preservedSourceOperations.filter(
        (op: any) => op.accountAlias === originalSource.accountAlias
      )

      if (matchingOps.length === 0) {
        errors.push(
          `Source account ${originalSource.accountAlias} is missing from response`
        )
      }
    })
  }

  if (validatedResponse.transaction.feeRules && validFeeOperations.length > 0) {
    const feeCalculationState = extractFeeCalculationStateRefactored(
      validatedResponse,
      originalRequest,
      preservedSourceOperations,
      preservedDestinationOperations
    )

    const packageRules: FeePackageRule[] =
      validatedResponse.transaction.feeRules.map((rule: any) => ({
        feeId: rule.feeId,
        feeLabel: rule.feeLabel,
        priority: rule.priority,
        referenceAmount: rule.referenceAmount || 'originalAmount',
        isDeductibleFrom: rule.isDeductibleFrom,
        creditAccount: rule.creditAccount
      }))

    const feeValidationResult = FeeRuntimeValidator.validateFeeCalculation(
      feeCalculationState,
      packageRules
    )

    if (!feeValidationResult.isValid) {
      errors.push(...feeValidationResult.errors)
    }

    if (
      feeValidationResult.warnings &&
      feeValidationResult.warnings.length > 0
    ) {
      warnings.push(...feeValidationResult.warnings)
    }
  }

  // The structure is preserved in the operations themselves

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
    validatedResponse,
    preservedStructure: {
      hasMultipleSources,
      hasMultipleDestinations,
      hasDuplicateAccounts: hasDuplicateSources || hasDuplicateDestinations,
      preservedOperations: {
        source: preservedSourceOperations,
        destination: nonFeeOperations,
        fees: validFeeOperations
      }
    }
  }
}

/**
 * Extract fee calculation state without losing operation relationships
 */
function extractFeeCalculationStateRefactored(
  feeResponse: any,
  originalRequest: any,
  sourceOperations: any[],
  destinationOperations: any[]
): FeeCalculationState {
  const feeData = feeResponse.transaction

  const originalAmount = parseFloat(originalRequest.transaction?.value || '0')
  const originalCurrency = feeData.send?.asset || 'USD'

  // but track all sources in metadata
  const primarySource = sourceOperations[0]?.accountAlias || ''
  const _allSources = sourceOperations.map((op) => op.accountAlias)

  const mainRecipients = destinationOperations.filter(
    (op: any) => op.operationType === 'destination'
  )
  const primaryRecipient = mainRecipients[0]?.accountAlias || ''

  const feeOperations = destinationOperations.filter(
    (op: any) => op.operationType === 'fee'
  )

  let deductibleFees = 0
  let nonDeductibleFees = 0
  const appliedFees: AppliedFee[] = []

  feeOperations.forEach((feeOp: any) => {
    const feeAmount = parseFloat(feeOp.amount?.value || '0')

    const matchingRule = feeData.feeRules?.find(
      (rule: any) =>
        rule.creditAccount === feeOp.accountAlias ||
        rule.creditAccount.replace('@', '') ===
          feeOp.accountAlias.replace('@', '')
    )

    const isDeductible = matchingRule?.isDeductibleFrom || false

    if (isDeductible) {
      deductibleFees += feeAmount
    } else {
      nonDeductibleFees += feeAmount
    }

    appliedFees.push({
      feeId: matchingRule?.feeId || `fee-${appliedFees.length + 1}`,
      feeLabel: feeOp.description || matchingRule?.feeLabel || 'Fee',
      calculatedAmount: feeAmount,
      isDeductibleFrom: isDeductible,
      creditAccount: feeOp.accountAlias,
      priority: matchingRule?.priority || appliedFees.length + 1
    })
  })

  const totalFees = deductibleFees + nonDeductibleFees

  const totalSourceAmount = sourceOperations.reduce(
    (sum: number, op: any) => sum + parseFloat(op.amount?.value || '0'),
    0
  )

  const totalDestinationAmount = mainRecipients.reduce(
    (sum: number, op: any) => sum + parseFloat(op.amount?.value || '0'),
    0
  )

  return {
    originalAmount,
    originalCurrency,
    sourceAccount: primarySource,
    destinationAccount: primaryRecipient,
    deductibleFees,
    nonDeductibleFees,
    totalFees,
    appliedFees,
    sourcePaysAmount: totalSourceAmount,
    destinationReceivesAmount: totalDestinationAmount,
    packageId: feeData.packageAppliedID,
    packageLabel: feeData.packageLabel,
    calculatedAt: new Date()
  }
}

/**
 * Helper to check if response should use refactored validation
 */
export function shouldUseRefactoredValidation(feeResponse: any): boolean {
  if (!feeResponse?.transaction) return false

  const sourceOps = feeResponse.transaction.send?.source?.from || []
  const destOps = feeResponse.transaction.send?.distribute?.to || []

  // 1. Multi-party transactions (N:N)
  // 3. Transactions with fee operations

  const hasMultipleSources = sourceOps.length > 1
  const hasMultipleDestinations =
    destOps.filter((op: any) => !op.metadata?.source).length > 1
  const hasFeeOperations = destOps.some((op: any) => op.metadata?.source)

  const sourceAccounts = sourceOps.map((op: any) => op.accountAlias)
  const hasDuplicates = sourceAccounts.length !== new Set(sourceAccounts).size

  return (
    hasMultipleSources ||
    hasMultipleDestinations ||
    hasFeeOperations ||
    hasDuplicates
  )
}

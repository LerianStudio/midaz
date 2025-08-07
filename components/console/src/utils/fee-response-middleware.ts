import {
  createFeeValidationService,
  getTransactionAccounts
} from '@/utils/fee-validation'
import { validateTransaction } from '@/utils/transaction-validation'
import {
  FeeRuntimeValidator,
  FeePackageRule
} from '@/utils/fee-runtime-validation'
import { FeeCalculationState, AppliedFee } from '@/types/fee-calculation.types'
import {
  validateAndFilterFeeResponseRefactored,
  shouldUseRefactoredValidation
} from '@/utils/fee-response-middleware-refactored'
import {
  FeeApiCalculateRequest,
  FeeApiCalculateResponse,
  isFeeApiSuccessResponse
} from '@/types/fee-api.types'

import { LOG_PREFIXES } from '@/constants/fee-constants'

export function validateAndFilterFeeResponse(
  feeResponse: FeeApiCalculateResponse,
  originalRequest: FeeApiCalculateRequest
): FeeApiCalculateResponse {
  if (shouldUseRefactoredValidation(feeResponse)) {
    const result = validateAndFilterFeeResponseRefactored(
      feeResponse,
      originalRequest
    )

    if (!result.isValid) {
      throw new Error(
        `Transaction validation failed: ${result.errors.join(', ')}`
      )
    }

    if (result.warnings.length > 0) {
      console.warn(
        `${LOG_PREFIXES.FEE_MIDDLEWARE} Validation warnings:`,
        result.warnings
      )
    }

    return result.validatedResponse
  }

  return validateAndFilterFeeResponseOriginal(feeResponse, originalRequest)
}

function extractAccountLists(operations: any[]): string[] {
  return operations?.map((op) => op.accountAlias) || []
}

function checkDuplicateAccounts(accounts: string[]): {
  hasDuplicates: boolean
  duplicates: string[]
} {
  const uniqueAccounts = new Set(accounts)
  const duplicates = accounts.filter(
    (acc, index) => accounts.indexOf(acc) !== index
  )
  return {
    hasDuplicates: accounts.length !== uniqueAccounts.size,
    duplicates
  }
}

function deduplicateSourceOperations(operations: any[]): any[] {
  const accountMap = new Map<string, any>()

  operations.forEach((op) => {
    const existing = accountMap.get(op.accountAlias)
    if (existing) {
      const existingAmount = parseFloat(existing.amount?.value || '0')
      const currentAmount = parseFloat(op.amount?.value || '0')
      existing.amount.value = (existingAmount + currentAmount).toFixed(2)
    } else {
      accountMap.set(op.accountAlias, {
        ...op,
        amount: { ...op.amount }
      })
    }
  })

  return Array.from(accountMap.values())
}

function validateAndFilterFeeResponseOriginal(
  feeResponse: FeeApiCalculateResponse,
  originalRequest: FeeApiCalculateRequest
): FeeApiCalculateResponse {
  if (!isFeeApiSuccessResponse(feeResponse)) {
    return feeResponse
  }

  const originalSourceAccounts = extractAccountLists(
    feeResponse.transaction?.send?.source?.from || []
  )
  const { hasDuplicates, duplicates } = checkDuplicateAccounts(
    originalSourceAccounts
  )

  if (hasDuplicates) {
    console.error(
      `${LOG_PREFIXES.FEE_MIDDLEWARE} WARNING: Original response has duplicate source accounts:`,
      {
        sources: originalSourceAccounts,
        duplicates
      }
    )
  }

  const feeValidationService = createFeeValidationService()

  const validatedResponse = JSON.parse(JSON.stringify(feeResponse))

  const feeData = validatedResponse.transaction
  let sourceOperations = feeData.send?.source?.from || []
  let destinationOperations = feeData.send?.distribute?.to || []

  const sourceAccounts = new Set(
    sourceOperations.map((op: any) => op.accountAlias)
  )
  const destAccounts = new Set(
    destinationOperations.map((op: any) => op.accountAlias)
  )
  const overlappingAccounts = Array.from(sourceAccounts).filter((acc) =>
    destAccounts.has(acc)
  )

  if (overlappingAccounts.length > 0) {
    console.warn(
      `${LOG_PREFIXES.FEE_MIDDLEWARE} Accounts appear in both source and destination:`,
      overlappingAccounts
    )
  }

  if (hasDuplicates) {
    validatedResponse.transaction.send.source.from =
      deduplicateSourceOperations(sourceOperations)
    sourceOperations = validatedResponse.transaction.send.source.from
  }

  if (sourceOperations.length === 0 || destinationOperations.length === 0) {
    return validatedResponse
  }

  const transactionAccounts = getTransactionAccounts(
    originalRequest.transaction?.send?.source?.from || [],
    originalRequest.transaction?.send?.distribute?.to || []
  )

  // Fee operations can have metadata.source = "fee" or metadata.source = account name
  const allFeeOperations = destinationOperations.filter(
    (op: any) => op.metadata?.source
  )

  const nonFeeOperations = destinationOperations.filter(
    (op: any) => !op.metadata?.source
  )

  const validFeeOperations = feeValidationService.filterValidFeeOperations(
    allFeeOperations,
    transactionAccounts
  )

  // Need to deduplicate in case an account appears as both regular destination and fee
  const accountMap = new Map<string, any>()

  nonFeeOperations.forEach((op: any) => {
    accountMap.set(op.accountAlias, op)
  })

  validFeeOperations.forEach((feeOp: any) => {
    const existingOp = accountMap.get(feeOp.accountAlias)
    if (existingOp) {
      // Merge the amounts
      const existingAmount = parseFloat(existingOp.amount?.value || '0')
      const feeAmount = parseFloat(feeOp.amount?.value || '0')
      existingOp.amount.value = (existingAmount + feeAmount).toFixed(2)
      existingOp.metadata = {
        ...existingOp.metadata,
        isMerged: true,
        mergedFeeAmount: feeAmount // Track the fee amount that was merged
      }
    } else {
      accountMap.set(feeOp.accountAlias, feeOp)
    }
  })

  const validDestinationOperations = Array.from(accountMap.values())

  validatedResponse.transaction.send.distribute.to = validDestinationOperations
  destinationOperations = validDestinationOperations

  const hasMergedOperations = validDestinationOperations.some(
    (op: any) => op.metadata?.isMerged
  )

  if (
    validFeeOperations.length < allFeeOperations.length ||
    hasMergedOperations
  ) {
    const newDestinationTotal = validDestinationOperations.reduce(
      (total: number, op: any) => {
        return total + parseFloat(op.amount?.value || '0')
      },
      0
    )

    // If fees were removed, source should be reduced by that amount
    const newSourceTotal = newDestinationTotal

    validatedResponse.transaction.send.value = newSourceTotal.toFixed(2)

    if (sourceOperations.length > 0) {
      if (sourceOperations.length === 1) {
        sourceOperations[0].amount.value = newSourceTotal.toFixed(2)
      } else {
        const currentSourceTotal = sourceOperations.reduce(
          (total: number, source: any) => {
            return total + parseFloat(source.amount?.value || '0')
          },
          0
        )

        const adjustmentRatio = newSourceTotal / currentSourceTotal

        sourceOperations.forEach((source: any) => {
          const currentAmount = parseFloat(source.amount?.value || '0')
          const newAmount = currentAmount * adjustmentRatio

          if (newAmount < 0) {
            console.error('[FeeResponseMiddleware] Source would go negative:', {
              accountAlias: source.accountAlias,
              currentAmount,
              newAmount
            })
            throw new Error(
              `Source account ${source.accountAlias} would have negative amount`
            )
          }

          source.amount.value = newAmount.toFixed(2)
        })
      }
    }
  }

  const originalAmount = parseFloat(
    originalRequest.transaction?.send?.value || '0'
  )

  const validationResult = validateTransaction(
    validatedResponse.transaction.send.source.from,
    validatedResponse.transaction.send.distribute.to,
    originalAmount
  )

  if (!validationResult.isValid) {
    console.error(
      '[FeeResponseMiddleware] Transaction validation failed:',
      validationResult.errors
    )
    throw new Error(
      `Transaction validation failed: ${validationResult.errors.join(', ')}`
    )
  }

  if (validationResult.warnings.length > 0) {
    console.warn(
      '[FeeResponseMiddleware] Transaction validation warnings:',
      validationResult.warnings
    )
  }

  if (validatedResponse.transaction.feeRules) {
    const feeCalculationState = extractFeeCalculationState(
      validatedResponse,
      originalRequest
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
      console.error(
        '[FeeResponseMiddleware] Fee runtime validation failed:',
        feeValidationResult.errors
      )
      throw new Error(
        `Fee validation failed: ${feeValidationResult.errors.join(', ')}`
      )
    }

    if (
      feeValidationResult.warnings &&
      feeValidationResult.warnings.length > 0
    ) {
      console.warn(
        '[FeeResponseMiddleware] Fee runtime validation warnings:',
        feeValidationResult.warnings
      )
    }
  }

  return validatedResponse
}

/**
 * Extract fee calculation state from fee response
 */
function extractFeeCalculationState(
  feeResponse: any,
  originalRequest: any
): FeeCalculationState {
  const feeData = feeResponse.transaction
  const sourceOperations = feeData.send?.source?.from || []
  const destinationOperations = feeData.send?.distribute?.to || []

  const originalAmount = parseFloat(
    originalRequest.transaction?.send?.value || '0'
  )
  const originalCurrency = feeData.send?.asset || 'USD'

  const sourceAccount = sourceOperations[0]?.accountAlias || ''
  const mainRecipient = destinationOperations.find(
    (op: any) => !op.metadata?.source
  )
  const destinationAccount = mainRecipient?.accountAlias || ''

  const feeOperations = destinationOperations.filter(
    (op: any) => op.metadata?.source
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

  const sourcePaysAmount = originalAmount + nonDeductibleFees
  const destinationReceivesAmount = originalAmount - deductibleFees

  return {
    originalAmount,
    originalCurrency,
    sourceAccount,
    destinationAccount,
    deductibleFees,
    nonDeductibleFees,
    totalFees,
    appliedFees,
    sourcePaysAmount,
    destinationReceivesAmount,
    packageId: feeData.packageAppliedID,
    packageLabel: feeData.packageLabel,
    calculatedAt: new Date()
  }
}

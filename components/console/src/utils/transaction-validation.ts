import { FeeRuntimeValidator } from '@/utils/fee-runtime-validation'
import { extractFeeStateFromCalculation } from '@/utils/fee-calculation-state'

interface ValidationResult {
  isValid: boolean
  errors: string[]
  warnings: string[]
}

interface TransactionOperation {
  accountAlias: string
  amount: {
    value: string
    scale: number
  }
  metadata?: {
    source?: string
  }
}

export class TransactionValidator {
  private readonly MAX_FEE_PERCENTAGE = 100 // 100% max fee
  private readonly BALANCE_TOLERANCE = 0.01 // Allow 1 cent tolerance for rounding

  /**
   * Validates a complete transaction including fees
   */
  validateTransaction(
    sourceOperations: TransactionOperation[],
    destinationOperations: TransactionOperation[],
    originalAmount: number
  ): ValidationResult {
    const errors: string[] = []
    const warnings: string[] = []

    const negativeAmountCheck = this.checkForNegativeAmounts(
      sourceOperations,
      destinationOperations
    )
    errors.push(...negativeAmountCheck.errors)

    const balanceCheck = this.validateTransactionBalance(
      sourceOperations,
      destinationOperations
    )
    if (!balanceCheck.isBalanced) {
      errors.push(balanceCheck.error)
    }

    const feeCheck = this.validateFeeSizes(
      destinationOperations,
      originalAmount
    )
    errors.push(...feeCheck.errors)
    warnings.push(...feeCheck.warnings)

    const duplicateCheck = this.checkForDuplicateAccounts(
      sourceOperations,
      destinationOperations
    )
    errors.push(...duplicateCheck.errors)

    const recipientCheck = this.validateRecipientAmounts(destinationOperations)
    errors.push(...recipientCheck.errors)

    return {
      isValid: errors.length === 0,
      errors,
      warnings
    }
  }

  /**
   * Check for negative amounts in any operation
   */
  private checkForNegativeAmounts(
    sourceOperations: TransactionOperation[],
    destinationOperations: TransactionOperation[]
  ): { errors: string[] } {
    const errors: string[] = []
    const allOperations = [...sourceOperations, ...destinationOperations]

    allOperations.forEach((op) => {
      const amount = parseFloat(op.amount.value)
      if (amount < 0) {
        errors.push(
          `Negative amount detected for account ${op.accountAlias}: ${amount}`
        )
      }
    })

    return { errors }
  }

  /**
   * Validate that total debits equal total credits
   */
  private validateTransactionBalance(
    sourceOperations: TransactionOperation[],
    destinationOperations: TransactionOperation[]
  ): { isBalanced: boolean; error: string } {
    const totalDebits = sourceOperations.reduce((sum, op) => {
      const amount = typeof op.amount === 'object' ? op.amount.value : op.amount
      const parsed = parseFloat(amount)
      return sum + (isNaN(parsed) ? 0 : parsed)
    }, 0)

    const totalCredits = destinationOperations.reduce((sum, op) => {
      const amount = typeof op.amount === 'object' ? op.amount.value : op.amount
      const parsed = parseFloat(amount)
      return sum + (isNaN(parsed) ? 0 : parsed)
    }, 0)

    const difference = Math.abs(totalDebits - totalCredits)
    const isBalanced = difference <= this.BALANCE_TOLERANCE

    return {
      isBalanced,
      error: isBalanced
        ? ''
        : `Transaction is unbalanced. Debits: ${isNaN(totalDebits) ? 'NaN' : totalDebits.toFixed(2)}, Credits: ${isNaN(totalCredits) ? 'NaN' : totalCredits.toFixed(2)}, Difference: ${isNaN(difference) ? 'NaN' : difference.toFixed(2)}`
    }
  }

  /**
   * Validate fee sizes are reasonable
   */
  private validateFeeSizes(
    destinationOperations: TransactionOperation[],
    originalAmount: number
  ): { errors: string[]; warnings: string[] } {
    const errors: string[] = []
    const warnings: string[] = []

    const feeOperations = destinationOperations.filter(
      (op) => op.metadata?.source
    )
    const _nonFeeOperations = destinationOperations.filter(
      (op) => !op.metadata?.source
    )

    const totalFees = feeOperations.reduce(
      (sum, op) => sum + parseFloat(op.amount.value),
      0
    )

    if (originalAmount > 0) {
      const feePercentage = (totalFees / originalAmount) * 100

      if (feePercentage > this.MAX_FEE_PERCENTAGE) {
        errors.push(
          `Total fees (${totalFees.toFixed(2)}) exceed ${this.MAX_FEE_PERCENTAGE}% of transaction amount (${originalAmount.toFixed(2)})`
        )
      } else if (feePercentage > 50) {
        warnings.push(
          `High fee warning: Total fees are ${feePercentage.toFixed(2)}% of transaction amount`
        )
      }
    }

    feeOperations.forEach((feeOp) => {
      const feeAmount = parseFloat(feeOp.amount.value)
      if (feeAmount > originalAmount) {
        errors.push(
          `Individual fee for ${feeOp.accountAlias} (${feeAmount.toFixed(2)}) exceeds transaction amount`
        )
      }
    })

    return { errors, warnings }
  }

  /**
   * Check for duplicate accounts
   */
  private checkForDuplicateAccounts(
    sourceOperations: TransactionOperation[],
    destinationOperations: TransactionOperation[]
  ): { errors: string[] } {
    const errors: string[] = []

    const sourceAccounts = sourceOperations.map((op) => op.accountAlias)
    const uniqueSourceAccounts = new Set(sourceAccounts)
    if (sourceAccounts.length !== uniqueSourceAccounts.size) {
      console.error('[TransactionValidator] Duplicate source accounts:', {
        sourceAccounts,
        uniqueSourceAccounts: Array.from(uniqueSourceAccounts),
        duplicates: sourceAccounts.filter(
          (acc, index) => sourceAccounts.indexOf(acc) !== index
        )
      })
      errors.push('Duplicate source accounts detected')
    }

    const nonFeeDestinations = destinationOperations
      .filter((op) => !op.metadata?.source)
      .map((op) => op.accountAlias)
    const uniqueDestinations = new Set(nonFeeDestinations)
    if (nonFeeDestinations.length !== uniqueDestinations.size) {
      errors.push('Duplicate destination accounts detected')
    }

    return { errors }
  }

  /**
   * Validate recipient amounts after fee deduction
   */
  private validateRecipientAmounts(
    destinationOperations: TransactionOperation[]
  ): { errors: string[] } {
    const errors: string[] = []

    const recipients = destinationOperations.filter(
      (op) => !op.metadata?.source
    )

    recipients.forEach((recipient) => {
      const amount = parseFloat(recipient.amount.value)
      if (amount <= 0) {
        errors.push(
          `Recipient ${recipient.accountAlias} would receive non-positive amount: ${amount.toFixed(2)}`
        )
      }
    })

    return { errors }
  }

  /**
   * Pre-flight validation for transaction submission
   */
  validateTransactionPreflight(
    formValues: any,
    feeCalculation: any,
    consolidatedPayload?: any
  ): ValidationResult {
    const errors: string[] = []
    const warnings: string[] = []

    if (!formValues.value || parseFloat(formValues.value) <= 0) {
      errors.push('Transaction amount must be positive')
    }

    if (!formValues.source || formValues.source.length === 0) {
      errors.push('At least one source account is required')
    }

    if (!formValues.destination || formValues.destination.length === 0) {
      errors.push('At least one destination account is required')
    }

    if (feeCalculation && consolidatedPayload?._consolidatedData) {
      const sourceOps =
        consolidatedPayload._consolidatedData.sourceOperations || []
      const destOps =
        consolidatedPayload._consolidatedData.destinationOperations || []
      const amount = parseFloat(formValues.value || '0')

      const transactionValidation = this.validateTransaction(
        sourceOps,
        destOps,
        amount
      )
      errors.push(...transactionValidation.errors)
      warnings.push(...transactionValidation.warnings)
    } else if (feeCalculation && consolidatedPayload) {
      const sourceOps =
        consolidatedPayload.source?.map((op: any) => ({
          ...op,
          amount: { value: op.amount }
        })) || []
      const destOps =
        consolidatedPayload.destination?.map((op: any) => ({
          ...op,
          amount: { value: op.amount }
        })) || []
      const amount = parseFloat(formValues.value || '0')

      const transactionValidation = this.validateTransaction(
        sourceOps,
        destOps,
        amount
      )
      errors.push(...transactionValidation.errors)
      warnings.push(...transactionValidation.warnings)
    } else if (feeCalculation) {
      const sourceOps = feeCalculation.transaction?.send?.source?.from || []
      const destOps = feeCalculation.transaction?.send?.distribute?.to || []
      const amount = parseFloat(formValues.value || '0')

      const transactionValidation = this.validateTransaction(
        sourceOps,
        destOps,
        amount
      )
      errors.push(...transactionValidation.errors)
      warnings.push(...transactionValidation.warnings)

      try {
        const feeState = extractFeeStateFromCalculation(
          feeCalculation,
          formValues
        )

        if (feeState) {
          const packageRules = feeCalculation.transaction?.feeRules?.map(
            (rule: any) => ({
              feeId: rule.feeId,
              feeLabel: rule.feeLabel,
              priority: rule.priority,
              referenceAmount: rule.referenceAmount || 'originalAmount',
              isDeductibleFrom: rule.isDeductibleFrom,
              creditAccount: rule.creditAccount
            })
          )

          const feeValidationResult =
            FeeRuntimeValidator.validateFeeCalculation(feeState, packageRules)

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
      } catch (error) {
        console.error('Fee runtime validation error:', error)
        warnings.push('Could not perform complete fee validation')
      }
    }

    return {
      isValid: errors.length === 0,
      errors,
      warnings
    }
  }
}

export const transactionValidator = new TransactionValidator()

export function validateTransaction(
  sourceOperations: TransactionOperation[],
  destinationOperations: TransactionOperation[],
  originalAmount: number
): ValidationResult {
  return transactionValidator.validateTransaction(
    sourceOperations,
    destinationOperations,
    originalAmount
  )
}

export function validateTransactionPreflight(
  formValues: any,
  feeCalculation: any,
  consolidatedPayload?: any
): ValidationResult {
  return transactionValidator.validateTransactionPreflight(
    formValues,
    feeCalculation,
    consolidatedPayload
  )
}

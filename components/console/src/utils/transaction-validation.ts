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
  private readonly BALANCE_TOLERANCE = 0.01 // Allow 1 cent tolerance for rounding

  /**
   * Validates a complete transaction
   */
  validateTransaction(
    sourceOperations: TransactionOperation[],
    destinationOperations: TransactionOperation[]
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

    const duplicateCheck = this.checkForDuplicateAccounts(
      sourceOperations,
      destinationOperations
    )
    errors.push(...duplicateCheck.errors)

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

    const destinationAccounts = destinationOperations.map(
      (op) => op.accountAlias
    )
    const uniqueDestinations = new Set(destinationAccounts)
    if (destinationAccounts.length !== uniqueDestinations.size) {
      errors.push('Duplicate destination accounts detected')
    }

    return { errors }
  }

  /**
   * Pre-flight validation for transaction submission
   */
  validateTransactionPreflight(
    formValues: any,
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

    if (consolidatedPayload) {
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

      const transactionValidation = this.validateTransaction(sourceOps, destOps)
      errors.push(...transactionValidation.errors)
      warnings.push(...transactionValidation.warnings)
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
  destinationOperations: TransactionOperation[]
): ValidationResult {
  return transactionValidator.validateTransaction(
    sourceOperations,
    destinationOperations
  )
}

export function validateTransactionPreflight(
  formValues: any,
  consolidatedPayload?: any
): ValidationResult {
  return transactionValidator.validateTransactionPreflight(
    formValues,
    consolidatedPayload
  )
}

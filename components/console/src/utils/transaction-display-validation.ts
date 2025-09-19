import {
  TransactionDisplayData,
  TransactionDisplayValidation
} from '@/types/transaction-display.types'

export class TransactionDisplayValidator {
  /**
   * Validates transaction display data for accuracy and completeness
   */
  static validate(
    displayData: TransactionDisplayData
  ): TransactionDisplayValidation {
    const errors: TransactionDisplayValidation['errors'] = []
    const warnings: TransactionDisplayValidation['warnings'] = []

    const balanceValidation = this.validateBalance(displayData)
    errors.push(...balanceValidation.errors)
    warnings.push(...balanceValidation.warnings)

    const relationshipValidation = this.validateRelationships(displayData)
    errors.push(...relationshipValidation.errors)
    warnings.push(...relationshipValidation.warnings)

    const dataValidation = this.validateDataCompleteness(displayData)
    errors.push(...dataValidation.errors)
    warnings.push(...dataValidation.warnings)

    return {
      isValid: errors.length === 0,
      errors,
      warnings,
      displayData
    }
  }

  /**
   * Validate transaction balance
   */
  private static validateBalance(displayData: TransactionDisplayData): {
    errors: TransactionDisplayValidation['errors']
    warnings: TransactionDisplayValidation['warnings']
  } {
    const errors: TransactionDisplayValidation['errors'] = []
    const warnings: TransactionDisplayValidation['warnings'] = []

    const { summary, flows } = displayData

    const sourceTotal = parseFloat(summary.totalSourceAmount)
    const destinationTotal = parseFloat(summary.totalDestinationAmount)

    const difference = Math.abs(sourceTotal - destinationTotal)

    if (difference > 0.01) {
      errors.push({
        type: 'balance',
        message: `Transaction is unbalanced: source (${sourceTotal}) != destination (${destinationTotal})`,
        affectedAccounts: [...summary.uniqueSourceAccounts]
      })
    }

    flows.forEach((flow, index) => {
      const flowSourceAmount = parseFloat(flow.sourceAmount)
      const flowDestTotal = parseFloat(flow.destinationTotalAmount)

      const flowDifference = Math.abs(flowSourceAmount - flowDestTotal)

      if (flowDifference > 0.01) {
        warnings.push({
          type: 'display',
          message: `Flow ${index + 1} may have balance issues: source (${flowSourceAmount}) vs destination (${flowDestTotal})`,
          suggestion: 'Check if transaction amounts are correctly distributed'
        })
      }
    })

    return { errors, warnings }
  }

  /**
   * Validate transaction relationships
   */
  private static validateRelationships(displayData: TransactionDisplayData): {
    errors: TransactionDisplayValidation['errors']
    warnings: TransactionDisplayValidation['warnings']
  } {
    const errors: TransactionDisplayValidation['errors'] = []
    const warnings: TransactionDisplayValidation['warnings'] = []

    const { flows, summary } = displayData

    const sourceAccounts = new Set(summary.uniqueSourceAccounts)
    const destAccounts = new Set(summary.uniqueDestinationAccounts)

    const sourceDestOverlap = Array.from(sourceAccounts).filter((acc) =>
      destAccounts.has(acc)
    )
    if (sourceDestOverlap.length > 0) {
      warnings.push({
        type: 'display',
        message: `Accounts appear as both source and destination: ${sourceDestOverlap.join(', ')}`,
        suggestion:
          'This may cause display confusion. Consider showing net amounts.'
      })
    }

    flows.forEach((flow, index) => {
      if (flow.destinationOperations.length === 0) {
        errors.push({
          type: 'relationship',
          message: `Flow ${index + 1} has no destination operations`,
          affectedAccounts: [flow.sourceOperation.accountAlias]
        })
      }
    })

    return { errors, warnings }
  }

  /**
   * Validate data completeness
   */
  private static validateDataCompleteness(
    displayData: TransactionDisplayData
  ): {
    errors: TransactionDisplayValidation['errors']
    warnings: TransactionDisplayValidation['warnings']
  } {
    const errors: TransactionDisplayValidation['errors'] = []
    const warnings: TransactionDisplayValidation['warnings'] = []

    if (!displayData.asset) {
      errors.push({
        type: 'data',
        message: 'Transaction asset is missing'
      })
    }

    if (
      !displayData.originalAmount ||
      parseFloat(displayData.originalAmount) <= 0
    ) {
      errors.push({
        type: 'data',
        message: 'Transaction original amount is missing or invalid'
      })
    }

    displayData.flows.forEach((flow, flowIndex) => {
      if (!flow.sourceOperation.accountAlias) {
        errors.push({
          type: 'data',
          message: `Flow ${flowIndex + 1} source account alias is missing`
        })
      }

      flow.destinationOperations.forEach((destOp, destIndex) => {
        if (!destOp.accountAlias) {
          errors.push({
            type: 'data',
            message: `Flow ${flowIndex + 1} destination ${destIndex + 1} account alias is missing`
          })
        }
      })
    })

    if (
      displayData.displayMode === 'complex' &&
      !displayData.metadata?.transactionStructure
    ) {
      warnings.push({
        type: 'display',
        message: 'Complex transaction missing structural metadata',
        suggestion: 'Add transactionStructure metadata for better display hints'
      })
    }

    return { errors, warnings }
  }

  /**
   * Quick validation check for critical errors only
   */
  static hasErrors(displayData: TransactionDisplayData): boolean {
    const validation = this.validate(displayData)
    return !validation.isValid
  }

  /**
   * Get a human-readable summary of validation issues
   */
  static getSummary(validation: TransactionDisplayValidation): string {
    const errorCount = validation.errors.length
    const warningCount = validation.warnings.length

    if (errorCount === 0 && warningCount === 0) {
      return 'Transaction display data is valid'
    }

    const parts = []
    if (errorCount > 0) {
      parts.push(`${errorCount} error${errorCount > 1 ? 's' : ''}`)
    }
    if (warningCount > 0) {
      parts.push(`${warningCount} warning${warningCount > 1 ? 's' : ''}`)
    }

    return `Transaction display validation: ${parts.join(', ')}`
  }
}

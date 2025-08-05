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

    const feeValidation = this.validateFees(displayData)
    errors.push(...feeValidation.errors)
    warnings.push(...feeValidation.warnings)

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
    const feeTotal = parseFloat(summary.totalFeeAmount)

    const expectedTotal = destinationTotal + feeTotal
    const difference = Math.abs(sourceTotal - expectedTotal)

    if (difference > 0.01) {
      errors.push({
        type: 'balance',
        message: `Transaction is unbalanced: source (${sourceTotal}) != destination (${destinationTotal}) + fees (${feeTotal})`,
        affectedAccounts: [...summary.uniqueSourceAccounts]
      })
    }

    flows.forEach((flow, index) => {
      const flowSourceAmount = parseFloat(flow.sourceAmount)
      const flowDestTotal = parseFloat(flow.destinationTotalAmount)
      const flowFeeTotal = parseFloat(flow.feeTotalAmount)

      const flowExpectedTotal = flowDestTotal + flowFeeTotal
      const flowDifference = Math.abs(flowSourceAmount - flowExpectedTotal)

      if (flowDifference > 0.01) {
        warnings.push({
          type: 'display',
          message: `Flow ${index + 1} may have balance issues: source (${flowSourceAmount}) vs destination+fees (${flowExpectedTotal})`,
          suggestion:
            'Check if fee calculations are correctly applied to this flow'
        })
      }
    })

    return { errors, warnings }
  }

  /**
   * Validate fee consistency
   */
  private static validateFees(displayData: TransactionDisplayData): {
    errors: TransactionDisplayValidation['errors']
    warnings: TransactionDisplayValidation['warnings']
  } {
    const errors: TransactionDisplayValidation['errors'] = []
    const warnings: TransactionDisplayValidation['warnings'] = []

    const { summary, feeCalculation, flows } = displayData

    if (feeCalculation?.appliedFees) {
      const calculatedFeeTotal = feeCalculation.appliedFees.reduce(
        (sum, fee) => sum + parseFloat(fee.amount),
        0
      )

      const reportedFeeTotal = parseFloat(summary.totalFeeAmount)

      if (Math.abs(calculatedFeeTotal - reportedFeeTotal) > 0.01) {
        errors.push({
          type: 'fee',
          message: `Fee total mismatch: calculated (${calculatedFeeTotal}) vs reported (${reportedFeeTotal})`,
          affectedAccounts: summary.uniqueFeeAccounts
        })
      }

      const deductibleTotal = feeCalculation.appliedFees
        .filter((f) => f.isDeductibleFrom)
        .reduce((sum, f) => sum + parseFloat(f.amount), 0)

      const nonDeductibleTotal = feeCalculation.appliedFees
        .filter((f) => !f.isDeductibleFrom)
        .reduce((sum, f) => sum + parseFloat(f.amount), 0)

      const reportedDeductible = parseFloat(summary.totalDeductibleFees)
      const reportedNonDeductible = parseFloat(summary.totalNonDeductibleFees)

      if (Math.abs(deductibleTotal - reportedDeductible) > 0.01) {
        errors.push({
          type: 'fee',
          message: `Deductible fee mismatch: calculated (${deductibleTotal}) vs reported (${reportedDeductible})`
        })
      }

      if (Math.abs(nonDeductibleTotal - reportedNonDeductible) > 0.01) {
        errors.push({
          type: 'fee',
          message: `Non-deductible fee mismatch: calculated (${nonDeductibleTotal}) vs reported (${reportedNonDeductible})`
        })
      }
    }

    flows.forEach((flow) => {
      flow.feeOperations.forEach((feeOp) => {
        if (!feeOp.feeType) {
          warnings.push({
            type: 'display',
            message: `Fee operation for ${feeOp.accountAlias} missing fee type classification`,
            suggestion:
              'Ensure fee type (deductible/non-deductible) is properly set'
          })
        }
      })
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
    const feeAccounts = new Set(summary.uniqueFeeAccounts)

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

    const sourceFeeOverlap = Array.from(sourceAccounts).filter((acc) =>
      feeAccounts.has(acc)
    )
    if (sourceFeeOverlap.length > 0) {
      warnings.push({
        type: 'display',
        message: `Source accounts also collecting fees: ${sourceFeeOverlap.join(', ')}`,
        suggestion:
          'Ensure fee amounts are clearly separated from principal flows'
      })
    }

    flows.forEach((flow, index) => {
      if (
        flow.destinationOperations.length === 0 &&
        flow.feeOperations.length === 0
      ) {
        errors.push({
          type: 'relationship',
          message: `Flow ${index + 1} has no destination or fee operations`,
          affectedAccounts: [flow.sourceOperation.accountAlias]
        })
      }
    })

    const allFeeOps = flows.flatMap((f) => f.feeOperations)
    const orphanedFees = allFeeOps.filter((feeOp) => !feeOp.sourceAccountAlias)

    if (orphanedFees.length > 0) {
      warnings.push({
        type: 'display',
        message: `${orphanedFees.length} fee operations not associated with a source account`,
        suggestion: 'Fee attribution may be unclear in display'
      })
    }

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

      flow.feeOperations.forEach((feeOp, feeIndex) => {
        if (!feeOp.accountAlias) {
          errors.push({
            type: 'data',
            message: `Flow ${flowIndex + 1} fee ${feeIndex + 1} account alias is missing`
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

    if (
      displayData.feeCalculation &&
      displayData.feeCalculation.appliedFees.length > 0
    ) {
      if (!displayData.feeCalculation.packageLabel) {
        warnings.push({
          type: 'display',
          message: 'Fee package label is missing',
          suggestion: 'Users may not understand which fee package was applied'
        })
      }

      displayData.feeCalculation.appliedFees.forEach((fee, index) => {
        if (!fee.feeLabel) {
          warnings.push({
            type: 'display',
            message: `Applied fee ${index + 1} is missing a label`,
            suggestion:
              'Add descriptive labels to help users understand each fee'
          })
        }
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

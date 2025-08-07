import { AppliedFee, FeeCalculationState } from '@/types/fee-calculation.types'

export interface FeeValidationResult {
  isValid: boolean
  errors: string[]
  warnings: string[]
}

export interface FeePackageRule {
  feeId: string
  feeLabel: string
  priority: number
  referenceAmount: 'originalAmount' | 'afterFeesAmount'
  isDeductibleFrom: boolean
  creditAccount: string
  calculationModel?: {
    applicationRule: 'flatFee' | 'percentual' | 'maxBetweenTypes'
    calculations: Array<{
      type: 'flat' | 'percentage'
      value: number
    }>
  }
}

export class FeeRuntimeValidator {
  /**
   * Validate fee calculation state against RFC business rules
   */
  static validateFeeCalculation(
    state: FeeCalculationState,
    packageRules?: FeePackageRule[]
  ): FeeValidationResult {
    const errors: string[] = []
    const warnings: string[] = []

    if (!state.originalAmount || state.originalAmount < 0) {
      errors.push('Original amount must be a positive number')
    }

    if (!state.originalCurrency) {
      errors.push('Currency must be specified')
    }

    if (!state.sourceAccount || !state.destinationAccount) {
      errors.push('Both source and destination accounts must be specified')
    }

    const calculatedTotal = state.deductibleFees + state.nonDeductibleFees
    if (Math.abs(calculatedTotal - state.totalFees) > 0.01) {
      errors.push(
        `Fee totals mismatch: deductible (${state.deductibleFees}) + non-deductible (${state.nonDeductibleFees}) != total (${state.totalFees})`
      )
    }

    const sumOfIndividualFees = state.appliedFees.reduce(
      (sum, fee) => sum + fee.calculatedAmount,
      0
    )
    if (Math.abs(sumOfIndividualFees - state.totalFees) > 0.01) {
      errors.push(
        `Individual fees sum (${sumOfIndividualFees}) does not match total fees (${state.totalFees})`
      )
    }

    const sumDeductible = state.appliedFees
      .filter((fee) => fee.isDeductibleFrom)
      .reduce((sum, fee) => sum + fee.calculatedAmount, 0)
    const sumNonDeductible = state.appliedFees
      .filter((fee) => !fee.isDeductibleFrom)
      .reduce((sum, fee) => sum + fee.calculatedAmount, 0)

    if (Math.abs(sumDeductible - state.deductibleFees) > 0.01) {
      errors.push(
        `Deductible fees mismatch: sum of individual (${sumDeductible}) != total deductible (${state.deductibleFees})`
      )
    }

    if (Math.abs(sumNonDeductible - state.nonDeductibleFees) > 0.01) {
      errors.push(
        `Non-deductible fees mismatch: sum of individual (${sumNonDeductible}) != total non-deductible (${state.nonDeductibleFees})`
      )
    }

    const expectedSenderAmount = state.originalAmount + state.nonDeductibleFees
    const expectedRecipientAmount = state.originalAmount - state.deductibleFees

    if (Math.abs(state.sourcePaysAmount - expectedSenderAmount) > 0.01) {
      errors.push(
        `Source amount calculation error: expected ${expectedSenderAmount}, got ${state.sourcePaysAmount}`
      )
    }

    if (
      Math.abs(state.destinationReceivesAmount - expectedRecipientAmount) > 0.01
    ) {
      errors.push(
        `Destination amount calculation error: expected ${expectedRecipientAmount}, got ${state.destinationReceivesAmount}`
      )
    }

    if (state.destinationReceivesAmount < 0) {
      errors.push('Destination cannot receive negative amount')
    }

    const feePercentage = (state.totalFees / state.originalAmount) * 100
    if (feePercentage > 100) {
      errors.push(
        `Total fees exceed transaction amount (${feePercentage.toFixed(2)}%)`
      )
    } else if (feePercentage > 50) {
      warnings.push(
        `High fee percentage: ${feePercentage.toFixed(2)}% of transaction`
      )
    }

    if (packageRules && packageRules.length > 0) {
      const validationResult = this.validateAgainstPackageRules(
        state,
        packageRules
      )
      errors.push(...validationResult.errors)
      warnings.push(...validationResult.warnings)
    }

    return {
      isValid: errors.length === 0,
      errors,
      warnings
    }
  }

  /**
   * Validate fees against package rules
   */
  private static validateAgainstPackageRules(
    state: FeeCalculationState,
    packageRules: FeePackageRule[]
  ): { errors: string[]; warnings: string[] } {
    const errors: string[] = []
    const warnings: string[] = []

    const priority1Rules = packageRules.filter((rule) => rule.priority === 1)
    for (const rule of priority1Rules) {
      if (rule.referenceAmount !== 'originalAmount') {
        errors.push(
          `Fee "${rule.feeLabel}" has priority 1 but uses ${rule.referenceAmount} instead of originalAmount`
        )
      }
    }

    const priorities = packageRules.map((rule) => rule.priority)
    const uniquePriorities = new Set(priorities)
    if (priorities.length !== uniquePriorities.size) {
      errors.push('All fees within a package must have unique priorities')
    }

    const sortedByPriority = [...packageRules].sort(
      (a, b) => a.priority - b.priority
    )
    const appliedFeeIds = state.appliedFees.map((fee) => fee.feeId)
    const expectedOrder = sortedByPriority.map((rule) => rule.feeId)

    let orderMismatch = false
    for (
      let i = 0;
      i < Math.min(appliedFeeIds.length, expectedOrder.length);
      i++
    ) {
      if (appliedFeeIds[i] !== expectedOrder[i]) {
        orderMismatch = true
        break
      }
    }

    if (orderMismatch) {
      warnings.push(
        'Fees may not have been processed in priority order. Expected: ' +
          expectedOrder.join(', ') +
          ', Got: ' +
          appliedFeeIds.join(', ')
      )
    }

    let _runningAmount = state.originalAmount
    for (const appliedFee of state.appliedFees) {
      const matchingRule = packageRules.find(
        (rule) => rule.feeId === appliedFee.feeId
      )
      if (!matchingRule) continue

      if (matchingRule.referenceAmount === 'afterFeesAmount') {
        const previousDeductibleFees = state.appliedFees
          .slice(0, state.appliedFees.indexOf(appliedFee))
          .filter((fee) => fee.isDeductibleFrom)
          .reduce((sum, fee) => sum + fee.calculatedAmount, 0)

        const expectedReferenceAmount =
          state.originalAmount - previousDeductibleFees

        // but we can warn if the pattern seems wrong
        if (appliedFee.isDeductibleFrom && appliedFee.priority > 1) {
          warnings.push(
            `Fee "${appliedFee.feeLabel}" uses afterFeesAmount and may have been calculated on ${expectedReferenceAmount.toFixed(
              2
            )} instead of ${state.originalAmount.toFixed(2)}`
          )
        }
      }
    }

    for (const fee of state.appliedFees) {
      if (!fee.creditAccount) {
        errors.push(`Fee "${fee.feeLabel}" is missing credit account`)
      }
    }

    const feeIds = state.appliedFees.map((fee) => fee.feeId)
    const uniqueFeeIds = new Set(feeIds)
    if (feeIds.length !== uniqueFeeIds.size) {
      errors.push('Duplicate fee IDs detected in applied fees')
    }

    return { errors, warnings }
  }

  /**
   * Validate fee priority order
   */
  static validateFeePriorityOrder(fees: AppliedFee[]): boolean {
    for (let i = 1; i < fees.length; i++) {
      if (fees[i].priority < fees[i - 1].priority) {
        return false
      }
    }
    return true
  }

  /**
   * Calculate expected reference amount for a fee
   */
  static calculateExpectedReferenceAmount(
    fee: FeePackageRule,
    originalAmount: number,
    previousDeductibleFees: number
  ): number {
    if (fee.referenceAmount === 'originalAmount') {
      return originalAmount
    } else if (fee.referenceAmount === 'afterFeesAmount') {
      return originalAmount - previousDeductibleFees
    }
    throw new Error(`Unknown reference amount type: ${fee.referenceAmount}`)
  }

  /**
   * Validate individual fee calculation
   */
  static validateIndividualFeeCalculation(
    fee: AppliedFee,
    rule: FeePackageRule,
    referenceAmount: number
  ): { isValid: boolean; expectedAmount?: number; error?: string } {
    if (!rule.calculationModel) {
      return { isValid: true } // Can't validate without calculation model
    }

    const { applicationRule, calculations } = rule.calculationModel

    let expectedAmount = 0

    try {
      switch (applicationRule) {
        case 'flatFee':
          const flatCalc = calculations.find((c) => c.type === 'flat')
          expectedAmount = flatCalc ? flatCalc.value : 0
          break

        case 'percentual':
          const percentageCalc = calculations.find(
            (c) => c.type === 'percentage'
          )
          expectedAmount = percentageCalc
            ? (percentageCalc.value / 100) * referenceAmount
            : 0
          break

        case 'maxBetweenTypes':
          for (const calculation of calculations) {
            let calculatedAmount: number
            if (calculation.type === 'flat') {
              calculatedAmount = calculation.value
            } else if (calculation.type === 'percentage') {
              calculatedAmount = (calculation.value / 100) * referenceAmount
            } else {
              continue
            }
            expectedAmount = Math.max(expectedAmount, calculatedAmount)
          }
          break

        default:
          return {
            isValid: false,
            error: `Unknown application rule: ${applicationRule}`
          }
      }

      const isValid = Math.abs(fee.calculatedAmount - expectedAmount) < 0.01

      return {
        isValid,
        expectedAmount,
        error: isValid
          ? undefined
          : `Expected ${expectedAmount.toFixed(2)}, got ${fee.calculatedAmount.toFixed(
              2
            )}`
      }
    } catch (error) {
      return {
        isValid: false,
        error: error instanceof Error ? error.message : 'Unknown error'
      }
    }
  }
}

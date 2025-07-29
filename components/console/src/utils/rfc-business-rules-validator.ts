import {
  FeeApiCalculateRequest,
  FeeApiTransaction,
  FeeApiSourceAccount,
  FeeApiDestinationAccount
} from '@/types/fee-api.types'

import { FEE_ERROR_CODES, FEE_FIELD_NAMES } from '@/constants/fee-constants'

export interface ValidationResult {
  isValid: boolean
  errors: ValidationError[]
}

export interface ValidationError {
  code: string
  field: string
  message: string
  severity: 'error' | 'warning'
}

export interface FeeRule {
  feeId: string
  priority: number
  referenceAmount:
    | typeof FEE_FIELD_NAMES.ORIGINAL_AMOUNT
    | typeof FEE_FIELD_NAMES.AFTER_FEES_AMOUNT
  applicationRule: 'flatFee' | 'percentual' | 'maxBetweenTypes'
  calculations?: {
    flatAmount?: number
    percentage?: number
  }
}

export class RFCBusinessRulesValidator {
  private static readonly ERRORS = {
    PRIORITY_1_REFERENCE: FEE_ERROR_CODES.PRIORITY_1_REFERENCE,
    PRIORITY_GREATER_1_REFERENCE: FEE_ERROR_CODES.PRIORITY_GREATER_1_REFERENCE,
    MAX_BETWEEN_TYPES_RULE: FEE_ERROR_CODES.PRIORITY_OPERATION_ERROR,
    INVALID_PERCENTAGE_SUM: FEE_ERROR_CODES.INVALID_FEE_STRUCTURE,
    INVALID_ASSET_CONSISTENCY: FEE_ERROR_CODES.CALCULATION_ERROR,
    INVALID_AMOUNT_VALUE: FEE_ERROR_CODES.VALIDATION_ERROR,
    MISSING_CHART_OF_ACCOUNTS: '0154',
    INVALID_DISTRIBUTION_SUM: '0155',
    DEDUCTIBLE_FEE_VALIDATION: '0156',
    ROUTE_CONSISTENCY: '0157'
  }

  static validateCalculationRequest(
    request: FeeApiCalculateRequest
  ): ValidationResult {
    const errors: ValidationError[] = []

    if (!request.segmentId) {
      errors.push({
        code: this.ERRORS.MISSING_CHART_OF_ACCOUNTS,
        field: 'segmentId',
        message: 'Segment ID is required for fee calculation',
        severity: 'error'
      })
    }

    if (!request.ledgerId) {
      errors.push({
        code: this.ERRORS.MISSING_CHART_OF_ACCOUNTS,
        field: 'ledgerId',
        message: 'Ledger ID is required for fee calculation',
        severity: 'error'
      })
    }

    if (!request.transaction) {
      errors.push({
        code: this.ERRORS.INVALID_AMOUNT_VALUE,
        field: 'transaction',
        message: 'Transaction object is required',
        severity: 'error'
      })
      return { isValid: false, errors }
    }

    const transactionErrors = this.validateTransaction(request.transaction)
    errors.push(...transactionErrors)

    return {
      isValid: errors.filter((e) => e.severity === 'error').length === 0,
      errors
    }
  }

  private static validateTransaction(
    transaction: FeeApiTransaction
  ): ValidationError[] {
    const errors: ValidationError[] = []

    if (!transaction.chartOfAccountsGroupName) {
      errors.push({
        code: this.ERRORS.MISSING_CHART_OF_ACCOUNTS,
        field: 'transaction.chartOfAccountsGroupName',
        message: 'Chart of accounts group name is required',
        severity: 'error'
      })
    }

    if (!transaction.send) {
      errors.push({
        code: this.ERRORS.INVALID_AMOUNT_VALUE,
        field: 'transaction.send',
        message: 'Transaction send object is required',
        severity: 'error'
      })
      return errors
    }

    const sendErrors = this.validateTransactionSend(transaction.send)
    errors.push(...sendErrors)

    const assetConsistencyErrors = this.validateAssetConsistency(transaction)
    errors.push(...assetConsistencyErrors)

    const distributionErrors = this.validateAccountDistribution(transaction)
    errors.push(...distributionErrors)

    return errors
  }

  private static validateTransactionSend(
    send: FeeApiTransaction['send']
  ): ValidationError[] {
    const errors: ValidationError[] = []

    if (!send.asset) {
      errors.push({
        code: this.ERRORS.INVALID_ASSET_CONSISTENCY,
        field: 'transaction.send.asset',
        message: 'Asset is required in send object',
        severity: 'error'
      })
    }

    if (!send.value || isNaN(Number(send.value)) || Number(send.value) <= 0) {
      errors.push({
        code: this.ERRORS.INVALID_AMOUNT_VALUE,
        field: 'transaction.send.value',
        message: 'Transaction value must be a positive number',
        severity: 'error'
      })
    }

    if (send.source?.from) {
      send.source.from.forEach((source, index) => {
        const sourceErrors = this.validateSourceAccount(source, index)
        errors.push(...sourceErrors)
      })
    }

    if (send.distribute?.to) {
      send.distribute.to.forEach((dest, index) => {
        const destErrors = this.validateDestinationAccount(dest, index)
        errors.push(...destErrors)
      })
    }

    return errors
  }

  private static validateSourceAccount(
    source: FeeApiSourceAccount,
    index: number
  ): ValidationError[] {
    const errors: ValidationError[] = []
    const prefix = `transaction.send.source.from[${index}]`

    if (!source.account && !source.accountAlias) {
      errors.push({
        code: this.ERRORS.MISSING_CHART_OF_ACCOUNTS,
        field: `${prefix}.account`,
        message: 'Either account or accountAlias is required',
        severity: 'error'
      })
    }

    if (!source.amount?.value || isNaN(Number(source.amount.value))) {
      errors.push({
        code: this.ERRORS.INVALID_AMOUNT_VALUE,
        field: `${prefix}.amount.value`,
        message: 'Valid amount value is required',
        severity: 'error'
      })
    }

    if (!source.chartOfAccounts) {
      errors.push({
        code: this.ERRORS.MISSING_CHART_OF_ACCOUNTS,
        field: `${prefix}.chartOfAccounts`,
        message: 'Chart of accounts is required for source account',
        severity: 'error'
      })
    }

    if (source.share) {
      const shareErrors = this.validateShare(source.share, `${prefix}.share`)
      errors.push(...shareErrors)
    }

    return errors
  }

  private static validateDestinationAccount(
    dest: FeeApiDestinationAccount,
    index: number
  ): ValidationError[] {
    const errors: ValidationError[] = []
    const prefix = `transaction.send.distribute.to[${index}]`

    if (!dest.account && !dest.accountAlias) {
      errors.push({
        code: this.ERRORS.MISSING_CHART_OF_ACCOUNTS,
        field: `${prefix}.account`,
        message: 'Either account or accountAlias is required',
        severity: 'error'
      })
    }

    if (!dest.amount?.value || isNaN(Number(dest.amount.value))) {
      errors.push({
        code: this.ERRORS.INVALID_AMOUNT_VALUE,
        field: `${prefix}.amount.value`,
        message: 'Valid amount value is required',
        severity: 'error'
      })
    }

    if (!dest.chartOfAccounts) {
      errors.push({
        code: this.ERRORS.MISSING_CHART_OF_ACCOUNTS,
        field: `${prefix}.chartOfAccounts`,
        message: 'Chart of accounts is required for destination account',
        severity: 'error'
      })
    }

    return errors
  }

  private static validateShare(
    share: { percentage?: number; percentageOfPercentage?: number },
    fieldPath: string
  ): ValidationError[] {
    const errors: ValidationError[] = []

    if (share.percentage !== undefined) {
      if (share.percentage < 0 || share.percentage > 100) {
        errors.push({
          code: this.ERRORS.INVALID_PERCENTAGE_SUM,
          field: `${fieldPath}.percentage`,
          message: 'Percentage must be between 0 and 100',
          severity: 'error'
        })
      }
    }

    if (share.percentageOfPercentage !== undefined) {
      if (
        share.percentageOfPercentage < 0 ||
        share.percentageOfPercentage > 100
      ) {
        errors.push({
          code: this.ERRORS.INVALID_PERCENTAGE_SUM,
          field: `${fieldPath}.percentageOfPercentage`,
          message: 'Percentage of percentage must be between 0 and 100',
          severity: 'error'
        })
      }
    }

    return errors
  }

  private static validateAssetConsistency(
    transaction: FeeApiTransaction
  ): ValidationError[] {
    const errors: ValidationError[] = []
    const mainAsset = transaction.send.asset

    if (transaction.send.source?.from) {
      transaction.send.source.from.forEach((source, index) => {
        if (source.amount?.asset && source.amount.asset !== mainAsset) {
          errors.push({
            code: this.ERRORS.INVALID_ASSET_CONSISTENCY,
            field: `transaction.send.source.from[${index}].amount.asset`,
            message: `Asset must match transaction asset (${mainAsset})`,
            severity: 'error'
          })
        }
      })
    }

    if (transaction.send.distribute?.to) {
      transaction.send.distribute.to.forEach((dest, index) => {
        if (dest.amount?.asset && dest.amount.asset !== mainAsset) {
          errors.push({
            code: this.ERRORS.INVALID_ASSET_CONSISTENCY,
            field: `transaction.send.distribute.to[${index}].amount.asset`,
            message: `Asset must match transaction asset (${mainAsset})`,
            severity: 'error'
          })
        }
      })
    }

    return errors
  }

  private static validateAccountDistribution(
    transaction: FeeApiTransaction
  ): ValidationError[] {
    const errors: ValidationError[] = []
    const totalValue = Number(transaction.send.value)

    if (
      transaction.send.source?.from &&
      transaction.send.source.from.length > 0
    ) {
      const sourcesWithShares = transaction.send.source.from.filter(
        (s) => s.share?.percentage !== undefined
      )

      if (sourcesWithShares.length > 0) {
        const totalPercentage = sourcesWithShares.reduce(
          (sum, s) => sum + (s.share?.percentage || 0),
          0
        )

        if (Math.abs(totalPercentage - 100) > 0.01) {
          errors.push({
            code: this.ERRORS.INVALID_PERCENTAGE_SUM,
            field: 'transaction.send.source.from',
            message: `Source account percentages must sum to 100% (current: ${totalPercentage}%)`,
            severity: 'error'
          })
        }
      }

      const totalSourceAmount = transaction.send.source.from.reduce(
        (sum, source) => sum + Number(source.amount?.value || 0),
        0
      )

      if (Math.abs(totalSourceAmount - totalValue) > 0.01) {
        errors.push({
          code: this.ERRORS.INVALID_DISTRIBUTION_SUM,
          field: 'transaction.send.source.from',
          message: `Source amounts must sum to transaction value (expected: ${totalValue}, got: ${totalSourceAmount})`,
          severity: 'error'
        })
      }
    }

    if (
      transaction.send.distribute?.to &&
      transaction.send.distribute.to.length > 0
    ) {
      const totalDestAmount = transaction.send.distribute.to.reduce(
        (sum, dest) => sum + Number(dest.amount?.value || 0),
        0
      )

      if (Math.abs(totalDestAmount - totalValue) > 0.01) {
        errors.push({
          code: this.ERRORS.INVALID_DISTRIBUTION_SUM,
          field: 'transaction.send.distribute.to',
          message: `Destination amounts must sum to transaction value (expected: ${totalValue}, got: ${totalDestAmount})`,
          severity: 'error'
        })
      }
    }

    return errors
  }

  static validateFeeRules(feeRules: FeeRule[]): ValidationResult {
    const errors: ValidationError[] = []

    feeRules.forEach((rule, index) => {
      if (rule.priority === 1 && rule.referenceAmount !== 'originalAmount') {
        errors.push({
          code: this.ERRORS.PRIORITY_1_REFERENCE,
          field: `feeRules[${index}].referenceAmount`,
          message:
            'Priority 1 fees must reference originalAmount per RFC requirements',
          severity: 'error'
        })
      }

      if (rule.priority > 1 && rule.referenceAmount !== 'afterFeesAmount') {
        errors.push({
          code: this.ERRORS.PRIORITY_GREATER_1_REFERENCE,
          field: `feeRules[${index}].referenceAmount`,
          message:
            'Priority greater than 1 fees must reference afterFeesAmount per RFC requirements',
          severity: 'error'
        })
      }

      if (rule.applicationRule === 'maxBetweenTypes') {
        const hasFlat = rule.calculations?.flatAmount !== undefined
        const hasPercentage = rule.calculations?.percentage !== undefined

        if (!hasFlat || !hasPercentage) {
          errors.push({
            code: this.ERRORS.MAX_BETWEEN_TYPES_RULE,
            field: `feeRules[${index}].calculations`,
            message:
              'maxBetweenTypes rule requires both flatAmount and percentage calculations',
            severity: 'error'
          })
        }
      }
    })

    return {
      isValid: errors.filter((e) => e.severity === 'error').length === 0,
      errors
    }
  }

  static calculateMaxBetweenTypes(
    flatAmount: number,
    percentage: number,
    referenceAmount: number
  ): number {
    const percentageAmount = (percentage / 100) * referenceAmount
    return Math.max(flatAmount, percentageAmount)
  }

  static validateDeductibleFees(
    feeRules: Array<{ isDeductibleFrom: boolean; priority: number }>
  ): ValidationResult {
    const errors: ValidationError[] = []
    const deductibleFees = feeRules.filter((rule) => rule.isDeductibleFrom)

    if (deductibleFees.length > 0) {
      const priorities = deductibleFees.map((f) => f.priority)
      const uniquePriorities = new Set(priorities)

      if (priorities.length !== uniquePriorities.size) {
        errors.push({
          code: this.ERRORS.DEDUCTIBLE_FEE_VALIDATION,
          field: 'feeRules',
          message: 'Deductible fees must have unique priorities',
          severity: 'warning'
        })
      }
    }

    return {
      isValid: errors.filter((e) => e.severity === 'error').length === 0,
      errors
    }
  }
}

export class RFCValidationError extends Error {
  constructor(public errors: ValidationError[]) {
    super(`RFC validation failed with ${errors.length} errors`)
    this.name = 'RFCValidationError'
  }
}

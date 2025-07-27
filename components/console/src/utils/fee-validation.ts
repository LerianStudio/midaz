import { AppliedFee } from '@/types/fee-calculation.types'

export interface FeeValidationService {
  /**
   * Validates if a fee should be applied to a specific transaction
   * @param transactionAccounts The accounts involved in the transaction
   * @param feeAccount The creditAccount from the fee rule
   * @returns true if the fee should be applied
   */
  shouldApplyFee(transactionAccounts: string[], feeAccount: string): boolean

  /**
   * Filters fee operations to only include those that should be applied
   * @param feeOperations All fee operations from the backend
   * @param transactionAccounts The accounts involved in the transaction
   * @returns Filtered fee operations that should actually be applied
   */
  filterValidFeeOperations(
    feeOperations: any[],
    transactionAccounts: string[]
  ): any[]

  /**
   * Filters applied fees to only include those that should be applied
   * @param appliedFees All applied fees
   * @param transactionAccounts The accounts involved in the transaction
   * @returns Filtered applied fees that should actually be applied
   */
  filterValidAppliedFees(
    appliedFees: AppliedFee[],
    transactionAccounts: string[]
  ): AppliedFee[]
}

export class FeeValidationServiceImpl implements FeeValidationService {
  shouldApplyFee(transactionAccounts: string[], feeAccount: string): boolean {
    const normalizedFeeAccount = this.normalizeAccountName(feeAccount)
    const normalizedTransactionAccounts = transactionAccounts.map((account) =>
      this.normalizeAccountName(account)
    )

    return normalizedTransactionAccounts.includes(normalizedFeeAccount)
  }

  filterValidFeeOperations(
    feeOperations: any[],
    transactionAccounts: string[]
  ): any[] {
    return feeOperations.filter((operation) => {
      const creditAccount = operation.accountAlias || ''

      const extractedAccountName =
        this.extractAccountFromFeeAccount(creditAccount)

      return this.shouldApplyFee(transactionAccounts, extractedAccountName)
    })
  }

  filterValidAppliedFees(
    appliedFees: AppliedFee[],
    transactionAccounts: string[]
  ): AppliedFee[] {
    return appliedFees.filter((fee) =>
      this.shouldApplyFee(transactionAccounts, fee.creditAccount)
    )
  }

  private normalizeAccountName(accountName: string): string {
    return accountName.replace(/^@/, '').toLowerCase()
  }

  private extractAccountFromFeeAccount(feeAccountName: string): string {
    // Supported patterns:
    // - @processing-fee-accountname -> accountname
    // - @tarifa-accountname -> accountname

    const withoutPrefix = feeAccountName.replace(/^@/, '')

    const patterns = [
      /^fee-(.+)$/i, // fee-accountname
      /^(.+)-fee$/i, // accountname-fee
      /^tarifa-(.+)$/i, // tarifa-accountname
      /^(.+)-tarifa$/i, // accountname-tarifa
      /^processing-fee-(.+)$/i, // processing-fee-accountname
      /^(.+)-processing-fee$/i, // accountname-processing-fee
      /^admin-fee-(.+)$/i, // admin-fee-accountname
      /^(.+)-admin-fee$/i // accountname-admin-fee
    ]

    for (const pattern of patterns) {
      const match = withoutPrefix.match(pattern)
      if (match && match[1]) {
        return match[1]
      }
    }

    // This ensures we don't accidentally exclude valid accounts
    return feeAccountName
  }
}

/**
 * Get all accounts involved in a transaction (source and destination)
 * @param sourceOperations Source operations from transaction
 * @param destinationOperations Destination operations from transaction
 * @returns Array of account names involved in the transaction
 */
export function getTransactionAccounts(
  sourceOperations: any[],
  destinationOperations: any[]
): string[] {
  const accounts: string[] = []

  sourceOperations.forEach((operation) => {
    // Fee operations don't typically appear in source operations
    if (operation.accountAlias) {
      accounts.push(operation.accountAlias)
    }
  })

  destinationOperations.forEach((operation) => {
    // Don't exclude based on account name alone
    if (operation.accountAlias && !operation.metadata?.source) {
      accounts.push(operation.accountAlias)
    }
  })

  return accounts
}

/**
 * Factory function to create fee validation service instance
 */
export function createFeeValidationService(): FeeValidationService {
  return new FeeValidationServiceImpl()
}

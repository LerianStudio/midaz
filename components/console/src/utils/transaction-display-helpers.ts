export interface OperationBreakdown {
  principal: number
  fees: Array<{
    amount: number
    description: string
    isDeductible: boolean
    source?: string
  }>
  total: number
}

/**
 * Extracts operation breakdown from consolidated operations
 */
export function extractOperationBreakdown(
  operation: any,
  feeRules?: any[]
): OperationBreakdown {
  const breakdown: OperationBreakdown = {
    principal: 0,
    fees: [],
    total: parseFloat(operation.amount || '0')
  }

  if (
    operation.metadata?.hasMultipleOperations &&
    operation.metadata?.operations
  ) {
    operation.metadata.operations.forEach((op: any) => {
      const amount = parseFloat(op.amount || '0')

      if (op.isFee) {
        const matchingRule = feeRules?.find(
          (rule: any) =>
            rule.creditAccount === operation.accountAlias ||
            rule.creditAccount.replace('@', '') ===
              operation.accountAlias.replace('@', '')
        )

        breakdown.fees.push({
          amount,
          description: op.description || 'Fee',
          isDeductible: matchingRule?.isDeductibleFrom || false,
          source: op.feeSource
        })
      } else {
        breakdown.principal += amount
      }
    })
  } else {
    if (operation.metadata?.isFee) {
      const matchingRule = feeRules?.find(
        (rule: any) =>
          rule.creditAccount === operation.accountAlias ||
          rule.creditAccount.replace('@', '') ===
            operation.accountAlias.replace('@', '')
      )

      breakdown.fees.push({
        amount: breakdown.total,
        description: operation.description || 'Fee',
        isDeductible: matchingRule?.isDeductibleFrom || false,
        source: operation.metadata?.feeSource
      })
    } else {
      breakdown.principal = breakdown.total
    }
  }

  return breakdown
}

/**
 * Gets display amount for an operation (principal only, excluding fees)
 */
export function getDisplayAmount(operation: any): number {
  const breakdown = extractOperationBreakdown(operation)
  return breakdown.principal
}

/**
 * Gets fee amount for an operation
 */
export function getFeeAmount(operation: any): number {
  const breakdown = extractOperationBreakdown(operation)
  return breakdown.fees.reduce((sum, fee) => sum + fee.amount, 0)
}

/**
 * Formats operation for display with proper labeling
 */
export function formatOperationDisplay(
  operation: any,
  feeRules?: any[]
): {
  displayAmount: string
  hasConsolidation: boolean
  breakdown?: {
    principal: string
    fees: Array<{
      amount: string
      label: string
      type: 'deductible' | 'non-deductible'
    }>
  }
} {
  const breakdown = extractOperationBreakdown(operation, feeRules)

  if (breakdown.fees.length === 0) {
    return {
      displayAmount: operation.amount,
      hasConsolidation: false
    }
  }

  return {
    displayAmount: breakdown.total.toString(),
    hasConsolidation: true,
    breakdown: {
      principal: breakdown.principal.toString(),
      fees: breakdown.fees.map((fee) => ({
        amount: fee.amount.toString(),
        label: fee.description,
        type: fee.isDeductible ? 'deductible' : 'non-deductible'
      }))
    }
  }
}

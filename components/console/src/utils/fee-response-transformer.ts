interface FeeApiOperation {
  accountAlias: string
  amount: {
    asset: string
    value: string | number
  }
  route: string
  metadata: Record<string, any> | null
}

interface FeeApiResponse {
  segmentId: string
  ledgerId: string
  transaction: {
    chartOfAccountsGroupName: string
    route: string
    pending?: string
    description: string
    metadata?: Record<string, any>
    send: {
      asset: string
      value: number | string
      source: {
        from: FeeApiOperation[]
      }
      distribute: {
        to: FeeApiOperation[]
      }
    }
    feeRules?: Array<{
      feeId: string
      feeLabel: string
      isDeductibleFrom: boolean
      creditAccount: string
      priority: number
    }>
    isDeductibleFrom?: boolean
  }
}

interface TransformedFeeResponse {
  originalRequest: {
    segmentId: string
    ledgerId: string
    totalAmount: string
    asset: string
  }
  fees: {
    totalFeeAmount: string
    totalWithFees: string
    operations: {
      source: Array<{
        accountAlias: string
        originalAmount: string
        feeAmount: string
        totalAmount: string
        feePercentage: string
        route: string
        metadata: Record<string, any> | null
      }>
      destination: Array<{
        accountAlias: string
        amount: string
        route: string
        metadata: Record<string, any> | null
      }>
    }
  }
  metadata: Record<string, any>
  feeRules?: Array<{
    feeId: string
    feeLabel: string
    isDeductibleFrom: boolean
    creditAccount: string
    priority: number
  }>
}

/**
 * Group operations by account and calculate fee amounts
 */
function groupOperationsByAccount(operations: FeeApiOperation[]) {
  const grouped = operations.reduce(
    (acc, op) => {
      const existing = acc[op.accountAlias]
      if (!existing) {
        acc[op.accountAlias] = {
          accountAlias: op.accountAlias,
          operations: [],
          totalAmount: 0,
          metadata: op.metadata
        }
      }

      const amount =
        typeof op.amount.value === 'string'
          ? parseFloat(op.amount.value)
          : op.amount.value

      acc[op.accountAlias].operations.push({
        amount,
        route: op.route,
        metadata: op.metadata
      })
      acc[op.accountAlias].totalAmount += amount

      return acc
    },
    {} as Record<string, any>
  )

  return Object.values(grouped)
}

/**
 * Calculate fee amounts from source operations
 */
function calculateSourceFees(
  sourceOperations: FeeApiOperation[],
  _originalTotalAmount: number
) {
  const groupedAccounts = groupOperationsByAccount(sourceOperations)

  return groupedAccounts.map((account) => {
    const principalOps = account.operations.filter(
      (op: any) =>
        !op.route.toLowerCase().includes('taxa') &&
        !op.route.toLowerCase().includes('fee')
    )
    const feeOps = account.operations.filter(
      (op: any) =>
        op.route.toLowerCase().includes('taxa') ||
        op.route.toLowerCase().includes('fee')
    )

    const principalAmount = principalOps.reduce(
      (sum: number, op: any) => sum + op.amount,
      0
    )
    const feeAmount = feeOps.reduce(
      (sum: number, op: any) => sum + op.amount,
      0
    )
    const feePercentage =
      principalAmount > 0
        ? ((feeAmount / principalAmount) * 100).toFixed(2)
        : '0'

    return {
      accountAlias: account.accountAlias,
      originalAmount: principalAmount.toFixed(2),
      feeAmount: feeAmount.toFixed(2),
      totalAmount: account.totalAmount.toFixed(2),
      feePercentage,
      route: principalOps[0]?.route || account.operations[0].route,
      metadata: account.metadata
    }
  })
}

/**
 * Transform the fee API response to a more consumable format
 */
export function transformFeeResponse(
  response: FeeApiResponse
): TransformedFeeResponse {
  const { transaction } = response
  const originalTotalAmount =
    typeof transaction.send.value === 'string'
      ? parseFloat(transaction.send.value)
      : transaction.send.value

  const sourceWithFees = calculateSourceFees(
    transaction.send.source.from,
    originalTotalAmount
  )

  const totalFeeAmount = sourceWithFees.reduce(
    (sum, account) => sum + parseFloat(account.feeAmount),
    0
  )

  const destinationOperations = groupOperationsByAccount(
    transaction.send.distribute.to
  ).map((account) => ({
    accountAlias: account.accountAlias,
    amount: account.totalAmount.toFixed(2),
    route: account.operations[0].route,
    metadata: account.metadata
  }))

  return {
    originalRequest: {
      segmentId: response.segmentId,
      ledgerId: response.ledgerId,
      totalAmount: originalTotalAmount.toFixed(2),
      asset: transaction.send.asset
    },
    fees: {
      totalFeeAmount: totalFeeAmount.toFixed(2),
      totalWithFees: (originalTotalAmount + totalFeeAmount).toFixed(2),
      operations: {
        source: sourceWithFees,
        destination: destinationOperations
      }
    },
    metadata: transaction.metadata || {},
    feeRules: transaction.feeRules
  }
}

/**
 * Check if a response indicates no fees were applied
 */
export function isNoFeesResponse(response: any): boolean {
  return (
    response.feesApplied !== undefined &&
    Array.isArray(response.feesApplied) &&
    response.feesApplied.length === 0 &&
    response.message !== undefined
  )
}

/**
 * Check if a response indicates gratuity was applied
 */
export function isGratuityResponse(response: any): boolean {
  return (
    response.feesApplied !== undefined &&
    Array.isArray(response.feesApplied) &&
    response.feesApplied.length === 0 &&
    response.message !== undefined &&
    response.message.toLowerCase().includes('gratuity')
  )
}

/**
 * Transform the API response to a consistent format
 */
export function normalizeFeesResponse(response: any): any {
  if (isNoFeesResponse(response)) {
    return {
      ...response,
      hasNoFees: !isGratuityResponse(response),
      hasGratuity: isGratuityResponse(response)
    }
  }

  // Handle both 'distribute' and 'distribuite' field names
  if (response.transaction?.send) {
    if (
      response.transaction.send.distribuite &&
      !response.transaction.send.distribute
    ) {
      response.transaction.send.distribute =
        response.transaction.send.distribuite
      delete response.transaction.send.distribuite
    }
    return transformFeeResponse(response)
  }

  return response
}

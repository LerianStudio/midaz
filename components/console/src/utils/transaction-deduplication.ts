export interface DeduplicationResult {
  source: any[]
  destination: any[]
  warnings: string[]
  adjustments: {
    account: string
    reason: string
    adjustment: string
  }[]
}

/**
 * Deduplicates accounts that appear in both source and destination
 * This is necessary because the API doesn't allow the same account in both arrays
 */
export function deduplicateTransactionAccounts(
  sourceOperations: any[],
  destinationOperations: any[]
): DeduplicationResult {
  const warnings: string[] = []
  const adjustments: DeduplicationResult['adjustments'] = []

  const sourceMap = new Map(sourceOperations.map((op) => [op.accountAlias, op]))
  const destMap = new Map(
    destinationOperations.map((op) => [op.accountAlias, op])
  )

  const overlappingAccounts = Array.from(sourceMap.keys()).filter((acc) =>
    destMap.has(acc)
  )

  if (overlappingAccounts.length === 0) {
    return {
      source: sourceOperations,
      destination: destinationOperations,
      warnings: [],
      adjustments: []
    }
  }

  const adjustedSource = [...sourceOperations]
  const adjustedDestination = destinationOperations.filter(
    (op) => !overlappingAccounts.includes(op.accountAlias)
  )

  overlappingAccounts.forEach((account) => {
    const sourceOp = sourceMap.get(account)!
    const destOp = destMap.get(account)!

    const sourceAmount = parseFloat(sourceOp.amount.value || sourceOp.amount)
    const destAmount = parseFloat(destOp.amount.value || destOp.amount)

    const netAmount = sourceAmount - destAmount

    if (netAmount > 0) {
      const sourceIndex = adjustedSource.findIndex(
        (op) => op.accountAlias === account
      )
      if (sourceIndex !== -1) {
        adjustedSource[sourceIndex] = {
          ...sourceOp,
          amount: sourceOp.amount.value
            ? {
                ...sourceOp.amount,
                value: netAmount.toString()
              }
            : netAmount.toString()
        }

        adjustments.push({
          account,
          reason: 'Account appears in both source and destination',
          adjustment: `Net debit: ${sourceAmount} - ${destAmount} = ${netAmount}`
        })
      }
    } else if (netAmount < 0) {
      const netCreditAmount = Math.abs(netAmount)

      const sourceIndex = adjustedSource.findIndex(
        (op) => op.accountAlias === account
      )
      if (sourceIndex !== -1) {
        adjustedSource.splice(sourceIndex, 1)
      }

      adjustedDestination.push({
        ...destOp,
        amount: destOp.amount.value
          ? {
              ...destOp.amount,
              value: netCreditAmount.toString()
            }
          : netCreditAmount.toString()
      })

      adjustments.push({
        account,
        reason: 'Account appears in both source and destination',
        adjustment: `Net credit: ${destAmount} - ${sourceAmount} = ${netCreditAmount}`
      })
    } else {
      const sourceIndex = adjustedSource.findIndex(
        (op) => op.accountAlias === account
      )
      if (sourceIndex !== -1) {
        adjustedSource.splice(sourceIndex, 1)
      }

      adjustments.push({
        account,
        reason: 'Account appears in both source and destination',
        adjustment: 'Net zero - removed from transaction'
      })
    }

    warnings.push(
      `Account ${account} appears in both source (${sourceAmount}) and destination (${destAmount}). ` +
        `Applied net calculation.`
    )
  })

  if (adjustedSource.length === 0) {
    throw new Error('All source accounts were removed during deduplication')
  }

  if (adjustedDestination.length === 0) {
    throw new Error(
      'All destination accounts were removed during deduplication'
    )
  }

  return {
    source: adjustedSource,
    destination: adjustedDestination,
    warnings,
    adjustments
  }
}

/**
 * Checks if deduplication is needed
 */
export function needsDeduplication(
  sourceOperations: any[],
  destinationOperations: any[]
): boolean {
  const sourceAccounts = new Set(sourceOperations.map((op) => op.accountAlias))
  const destAccounts = new Set(
    destinationOperations.map((op) => op.accountAlias)
  )

  return Array.from(sourceAccounts).some((acc) => destAccounts.has(acc))
}

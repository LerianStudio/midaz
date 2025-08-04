import {
  TransactionDisplayData,
  TransactionFlow,
  EnhancedTransactionOperation,
  TransactionDisplayMapper
} from '@/types/transaction-display.types'
import { v4 as uuidv4 } from 'uuid'

export class TransactionDisplayMapperImpl implements TransactionDisplayMapper {
  /**
   * Maps form data to display structure
   */
  mapFromFormData(formData: any): TransactionDisplayData {
    const flows: TransactionFlow[] = []

    if (formData.source?.length === 1 && formData.destination?.length === 1) {
      const sourceOp = this.createEnhancedOperation(
        formData.source[0],
        'source',
        formData.asset
      )
      const destOp = this.createEnhancedOperation(
        formData.destination[0],
        'destination',
        formData.asset
      )

      flows.push({
        flowId: uuidv4(),
        sourceOperation: sourceOp,
        destinationOperations: [destOp],
        feeOperations: [],
        sourceAmount: sourceOp.amount,
        destinationTotalAmount: destOp.amount,
        feeTotalAmount: '0',
        isSimpleFlow: true,
        hasDeductibleFees: false,
        hasNonDeductibleFees: false
      })
    } else {
      // This maintains the relationship between sources and destinations
      const totalSourceAmount = this.calculateTotal(formData.source || [])

      formData.source?.forEach((source: any) => {
        const sourceOp = this.createEnhancedOperation(
          source,
          'source',
          formData.asset
        )
        const sourceRatio = parseFloat(source.value) / totalSourceAmount

        const destinationOps: EnhancedTransactionOperation[] = []

        formData.destination?.forEach((dest: any) => {
          const destAmount = (parseFloat(dest.value) * sourceRatio).toFixed(2)
          const destOp = this.createEnhancedOperation(
            { ...dest, value: destAmount },
            'destination',
            formData.asset
          )
          destinationOps.push(destOp)
        })

        flows.push({
          flowId: uuidv4(),
          sourceOperation: sourceOp,
          destinationOperations: destinationOps,
          feeOperations: [],
          sourceAmount: sourceOp.amount,
          destinationTotalAmount: this.sumAmounts(destinationOps),
          feeTotalAmount: '0',
          isSimpleFlow: false,
          hasDeductibleFees: false,
          hasNonDeductibleFees: false
        })
      })
    }

    return {
      description: formData.description,
      chartOfAccountsGroupName: formData.chartOfAccountsGroupName,
      asset: formData.asset,
      originalAmount: formData.value?.toString() || '0',
      metadata: formData.metadata || {},
      flows,
      summary: this.calculateSummary(flows, formData.value?.toString() || '0'),
      displayMode:
        flows.length === 1 && flows[0].isSimpleFlow ? 'simple' : 'complex',
      hasWarnings: false,
      warnings: []
    }
  }

  /**
   * Maps fee calculation response to display structure
   */
  mapFromFeeCalculation(
    feeCalculation: any,
    originalFormData: any
  ): TransactionDisplayData {
    const flows: TransactionFlow[] = []
    const feeData = feeCalculation.transaction

    const sourceOperations = feeData.send?.source?.from || []
    const destinationOperations = feeData.send?.distribute?.to || []

    const feeOps = destinationOperations.filter(
      (op: any) => op.metadata?.source
    )
    const nonFeeOps = destinationOperations.filter(
      (op: any) => !op.metadata?.source
    )

    if (sourceOperations.length === 1 && nonFeeOps.length === 1) {
      const sourceOp = this.createEnhancedOperationFromFeeCalc(
        sourceOperations[0],
        'source'
      )
      const destOp = this.createEnhancedOperationFromFeeCalc(
        nonFeeOps[0],
        'destination'
      )
      const enhancedFeeOps = feeOps.map((op: any) =>
        this.createEnhancedOperationFromFeeCalc(
          op,
          'fee',
          sourceOp.accountAlias
        )
      )

      flows.push({
        flowId: uuidv4(),
        sourceOperation: sourceOp,
        destinationOperations: [destOp],
        feeOperations: enhancedFeeOps,
        sourceAmount: sourceOp.amount,
        destinationTotalAmount: destOp.amount,
        feeTotalAmount: this.sumAmounts(enhancedFeeOps),
        isSimpleFlow: true,
        hasDeductibleFees: enhancedFeeOps.some(
          (f: any) => f.feeType === 'deductible'
        ),
        hasNonDeductibleFees: enhancedFeeOps.some(
          (f: any) => f.feeType === 'non-deductible'
        )
      })
    } else {
      // Attempt to maintain relationships based on original form data
      const originalSources = originalFormData?.source || []
      const originalDestinations = originalFormData?.destination || []

      sourceOperations.forEach((sourceOp: any) => {
        const enhancedSource = this.createEnhancedOperationFromFeeCalc(
          sourceOp,
          'source'
        )

        const originalSource = originalSources.find(
          (s: any) => s.accountAlias === sourceOp.accountAlias
        )

        if (originalSource) {
          const sourceRatio =
            parseFloat(originalSource.value) /
            parseFloat(originalFormData.value)

          const flowDestOps: EnhancedTransactionOperation[] = []

          nonFeeOps.forEach((destOp: any) => {
            const originalDest = originalDestinations.find(
              (d: any) => d.accountAlias === destOp.accountAlias
            )

            if (originalDest) {
              const expectedAmount = (
                parseFloat(originalDest.value) * sourceRatio
              ).toFixed(2)
              const enhancedDest = this.createEnhancedOperationFromFeeCalc(
                destOp,
                'destination'
              )

              enhancedDest.amount = expectedAmount
              flowDestOps.push(enhancedDest)
            }
          })

          const flowFeeOps = feeOps
            .filter((feeOp: any) => {
              // This is a heuristic - may need refinement based on fee rules
              return (
                !feeOp.metadata?.sourceAccount ||
                feeOp.metadata?.sourceAccount === sourceOp.accountAlias
              )
            })
            .map((op: any) =>
              this.createEnhancedOperationFromFeeCalc(
                op,
                'fee',
                enhancedSource.accountAlias
              )
            )

          flows.push({
            flowId: uuidv4(),
            sourceOperation: enhancedSource,
            destinationOperations: flowDestOps,
            feeOperations: flowFeeOps,
            sourceAmount: enhancedSource.amount,
            destinationTotalAmount: this.sumAmounts(flowDestOps),
            feeTotalAmount: this.sumAmounts(flowFeeOps),
            isSimpleFlow: false,
            hasDeductibleFees: flowFeeOps.some(
              (f: any) => f.feeType === 'deductible'
            ),
            hasNonDeductibleFees: flowFeeOps.some(
              (f: any) => f.feeType === 'non-deductible'
            )
          })
        }
      })
    }

    const appliedFees = feeOps.map((feeOp: any) => {
      const matchingRule = feeData.feeRules?.find(
        (rule: any) =>
          rule.creditAccount === feeOp.accountAlias ||
          rule.creditAccount.replace('@', '') ===
            feeOp.accountAlias.replace('@', '')
      )

      return {
        feeId: matchingRule?.feeId || uuidv4(),
        feeLabel: feeOp.description || matchingRule?.feeLabel || 'Fee',
        amount: feeOp.amount.value,
        creditAccount: feeOp.accountAlias,
        isDeductibleFrom: matchingRule?.isDeductibleFrom || false,
        sourceAccount: feeOp.metadata?.sourceAccount
      }
    })

    return {
      description: feeData.description || originalFormData?.description,
      chartOfAccountsGroupName:
        feeData.chartOfAccountsGroupName ||
        originalFormData?.chartOfAccountsGroupName,
      asset: feeData.send?.asset || originalFormData?.asset,
      originalAmount: originalFormData?.value?.toString() || '0',
      metadata: { ...originalFormData?.metadata, ...feeData.metadata },
      flows,
      summary: this.calculateSummary(
        flows,
        originalFormData?.value?.toString() || '0'
      ),
      feeCalculation: {
        packageId: feeData.packageAppliedID,
        packageLabel: feeData.packageLabel,
        isDeductibleFrom: feeData.isDeductibleFrom || false,
        appliedFees
      },
      displayMode:
        flows.length === 1 && flows[0].isSimpleFlow ? 'simple' : 'complex',
      hasWarnings: this.checkForWarnings(flows, feeData),
      warnings: this.generateWarnings(flows, feeData)
    }
  }

  /**
   * Maps transaction entity to display structure
   */
  mapFromTransaction(transaction: any): TransactionDisplayData {
    const flows: TransactionFlow[] = []

    const sourceOps = transaction.source || []
    const destOps = transaction.destination || []

    const feeOps = destOps.filter(
      (op: any) =>
        op.description?.toLowerCase().includes('fee') ||
        op.chartOfAccounts?.fee ||
        op.metadata?.isFee
    )
    const nonFeeOps = destOps.filter(
      (op: any) =>
        !op.description?.toLowerCase().includes('fee') &&
        !op.chartOfAccounts?.fee &&
        !op.metadata?.isFee
    )

    if (sourceOps.length === 1 && nonFeeOps.length === 1) {
      const sourceOp = this.createEnhancedOperationFromTransaction(
        sourceOps[0],
        'source'
      )
      const destOp = this.createEnhancedOperationFromTransaction(
        nonFeeOps[0],
        'destination'
      )
      const enhancedFeeOps = feeOps.map((op: any) =>
        this.createEnhancedOperationFromTransaction(
          op,
          'fee',
          sourceOp.accountAlias
        )
      )

      flows.push({
        flowId: uuidv4(),
        sourceOperation: sourceOp,
        destinationOperations: [destOp],
        feeOperations: enhancedFeeOps,
        sourceAmount: sourceOp.amount,
        destinationTotalAmount: destOp.amount,
        feeTotalAmount: this.sumAmounts(enhancedFeeOps),
        isSimpleFlow: true,
        hasDeductibleFees: false, // Cannot determine from transaction data
        hasNonDeductibleFees: false
      })
    } else {
      const totalSourceAmount = this.sumAmounts(
        sourceOps.map((op: any) => ({ amount: op.amount }))
      )

      sourceOps.forEach((source: any) => {
        const sourceOp = this.createEnhancedOperationFromTransaction(
          source,
          'source'
        )
        const sourceRatio =
          parseFloat(source.amount) / parseFloat(totalSourceAmount)

        const flowDestOps = nonFeeOps.map((dest: any) => {
          const destAmount = (parseFloat(dest.amount) * sourceRatio).toFixed(2)
          return this.createEnhancedOperationFromTransaction(
            { ...dest, amount: destAmount },
            'destination'
          )
        })

        const flowFeeOps = feeOps.map((fee: any) => {
          const feeAmount = (parseFloat(fee.amount) * sourceRatio).toFixed(2)
          return this.createEnhancedOperationFromTransaction(
            { ...fee, amount: feeAmount },
            'fee',
            sourceOp.accountAlias
          )
        })

        flows.push({
          flowId: uuidv4(),
          sourceOperation: sourceOp,
          destinationOperations: flowDestOps,
          feeOperations: flowFeeOps,
          sourceAmount: sourceOp.amount,
          destinationTotalAmount: this.sumAmounts(flowDestOps),
          feeTotalAmount: this.sumAmounts(flowFeeOps),
          isSimpleFlow: false,
          hasDeductibleFees: false,
          hasNonDeductibleFees: false
        })
      })
    }

    return {
      transactionId: transaction.id,
      description: transaction.description,
      chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
      asset: transaction.asset,
      originalAmount: transaction.amount?.toString() || '0',
      metadata: transaction.metadata || {},
      flows,
      summary: this.calculateSummary(
        flows,
        transaction.amount?.toString() || '0'
      ),
      displayMode:
        flows.length === 1 && flows[0].isSimpleFlow ? 'simple' : 'complex',
      hasWarnings: false,
      warnings: []
    }
  }

  private createEnhancedOperation(
    data: any,
    type: 'source' | 'destination' | 'fee',
    defaultAsset: string
  ): EnhancedTransactionOperation {
    return {
      operationId: uuidv4(),
      operationType: type,
      accountAlias: data.accountAlias,
      asset: data.asset || defaultAsset,
      amount: data.value?.toString() || data.amount?.toString() || '0',
      description: data.description,
      chartOfAccounts: data.chartOfAccounts,
      metadata: data.metadata,
      isFee: type === 'fee',
      originalAmount: data.value?.toString() || data.amount?.toString() || '0'
    }
  }

  private createEnhancedOperationFromFeeCalc(
    operation: any,
    type: 'source' | 'destination' | 'fee',
    sourceAccountAlias?: string
  ): EnhancedTransactionOperation {
    const isFee = type === 'fee' || !!operation.metadata?.source
    const feeType = operation.metadata?.isDeductibleFrom
      ? 'deductible'
      : 'non-deductible'

    return {
      operationId: uuidv4(),
      operationType: type,
      accountAlias: operation.accountAlias,
      asset: operation.amount?.asset || 'USD',
      amount: operation.amount?.value || '0',
      description: operation.description,
      chartOfAccounts: operation.chartOfAccounts,
      metadata: operation.metadata,
      isFee,
      feeType: isFee ? feeType : undefined,
      sourceAccountAlias,
      originalAmount: operation.amount?.value || '0'
    }
  }

  private createEnhancedOperationFromTransaction(
    operation: any,
    type: 'source' | 'destination' | 'fee',
    sourceAccountAlias?: string
  ): EnhancedTransactionOperation {
    return {
      operationId: uuidv4(),
      operationType: type,
      accountAlias: operation.accountAlias,
      asset: operation.asset || 'USD',
      amount: operation.amount?.toString() || '0',
      description: operation.description,
      chartOfAccounts: operation.chartOfAccounts,
      metadata: operation.metadata,
      isFee: type === 'fee',
      sourceAccountAlias,
      originalAmount: operation.amount?.toString() || '0'
    }
  }

  private calculateTotal(items: any[]): number {
    return items.reduce((sum, item) => sum + parseFloat(item.value || '0'), 0)
  }

  private sumAmounts(operations: Array<{ amount: string }>): string {
    const total = operations.reduce(
      (sum, op) => sum + parseFloat(op.amount || '0'),
      0
    )
    return total.toFixed(2)
  }

  private calculateSummary(flows: TransactionFlow[], _originalAmount: string) {
    let totalSourceAmount = 0
    let totalDestinationAmount = 0
    let totalFeeAmount = 0
    let totalDeductibleFees = 0
    let totalNonDeductibleFees = 0

    const sourceAccounts = new Set<string>()
    const destinationAccounts = new Set<string>()
    const feeAccounts = new Set<string>()

    flows.forEach((flow) => {
      totalSourceAmount += parseFloat(flow.sourceAmount)
      totalDestinationAmount += parseFloat(flow.destinationTotalAmount)
      totalFeeAmount += parseFloat(flow.feeTotalAmount)

      sourceAccounts.add(flow.sourceOperation.accountAlias)

      flow.destinationOperations.forEach((op) => {
        destinationAccounts.add(op.accountAlias)
      })

      flow.feeOperations.forEach((op) => {
        feeAccounts.add(op.accountAlias)
        if (op.feeType === 'deductible') {
          totalDeductibleFees += parseFloat(op.amount)
        } else if (op.feeType === 'non-deductible') {
          totalNonDeductibleFees += parseFloat(op.amount)
        }
      })
    })

    return {
      totalSourceAmount: totalSourceAmount.toFixed(2),
      totalDestinationAmount: totalDestinationAmount.toFixed(2),
      totalFeeAmount: totalFeeAmount.toFixed(2),
      totalDeductibleFees: totalDeductibleFees.toFixed(2),
      totalNonDeductibleFees: totalNonDeductibleFees.toFixed(2),
      uniqueSourceAccounts: Array.from(sourceAccounts),
      uniqueDestinationAccounts: Array.from(destinationAccounts),
      uniqueFeeAccounts: Array.from(feeAccounts)
    }
  }

  private checkForWarnings(flows: TransactionFlow[], _feeData: any): boolean {
    const sourceAccounts = new Set(
      flows.map((f) => f.sourceOperation.accountAlias)
    )
    const destAccounts = new Set(
      flows.flatMap((f) => f.destinationOperations.map((op) => op.accountAlias))
    )

    const overlap = Array.from(sourceAccounts).filter((acc) =>
      destAccounts.has(acc)
    )

    return overlap.length > 0
  }

  private generateWarnings(flows: TransactionFlow[], _feeData: any): string[] {
    const warnings: string[] = []

    const sourceAccounts = new Set(
      flows.map((f) => f.sourceOperation.accountAlias)
    )
    const destAccounts = new Set(
      flows.flatMap((f) => f.destinationOperations.map((op) => op.accountAlias))
    )

    const overlap = Array.from(sourceAccounts).filter((acc) =>
      destAccounts.has(acc)
    )
    if (overlap.length > 0) {
      warnings.push(
        `The following accounts appear as both source and destination: ${overlap.join(', ')}`
      )
    }

    const hasMergedOps = flows.some(
      (f) =>
        f.destinationOperations.some((op) => op.metadata?.isMerged) ||
        f.feeOperations.some((op) => op.metadata?.isMerged)
    )

    if (hasMergedOps) {
      warnings.push(
        'Some operations have been merged due to duplicate accounts. This may affect the display accuracy.'
      )
    }

    return warnings
  }
}

export const transactionDisplayMapper = new TransactionDisplayMapperImpl()

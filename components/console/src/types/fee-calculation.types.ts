export interface FeeCalculationState {
  originalAmount: number
  originalCurrency: string
  sourceAccount: string
  destinationAccount: string

  deductibleFees: number
  nonDeductibleFees: number
  totalFees: number

  appliedFees: AppliedFee[]

  sourcePaysAmount: number
  destinationReceivesAmount: number

  packageId?: string
  packageLabel?: string

  calculatedAt: Date
}

export interface AppliedFee {
  feeId: string
  feeLabel: string
  calculatedAmount: number
  isDeductibleFrom: boolean
  creditAccount: string
  priority: number
}

export interface EnrichedTransactionMetadata {
  [key: string]: any

  feeCalculationData?: string

  feeCalculation?: FeeCalculationState

  feeDataSource: 'calculation' | 'reconstruction' | 'none'
}

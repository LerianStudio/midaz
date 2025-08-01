import { FeeCalculationState, AppliedFee } from '@/types/fee-calculation.types'
import {
  createFeeValidationService,
  getTransactionAccounts
} from '@/utils/fee-validation'
import { IntlShape } from 'react-intl'
import {
  FeeApiCalculateResponse,
  isFeeApiSuccessResponse
} from '@/types/fee-api.types'

interface TransactionFormValues {
  value: string
  asset: string
  source?: Array<{ accountAlias: string }>
  destination?: Array<{ accountAlias: string }>
}

interface FeeRule {
  feeId: string
  feeLabel: string
  isDeductibleFrom: boolean
  creditAccount: string
  priority: number
}

function extractTransactionAmounts(formValues: TransactionFormValues): {
  originalAmount: number
  originalCurrency: string
} {
  return {
    originalAmount: Number(formValues.value),
    originalCurrency: formValues.asset
  }
}

function extractAccountInformation(formValues: TransactionFormValues): {
  allSourceAccounts: string[]
  allDestinationAccounts: string[]
  sourceAccount: string
  destinationAccount: string
} {
  const allSourceAccounts =
    formValues.source?.map((s) => s.accountAlias).filter(Boolean) || []
  const allDestinationAccounts =
    formValues.destination?.map((d) => d.accountAlias).filter(Boolean) || []

  return {
    allSourceAccounts,
    allDestinationAccounts,
    sourceAccount: allSourceAccounts[0] || '',
    destinationAccount: allDestinationAccounts[0] || ''
  }
}

function categorizeOperations(
  destinationOperations: any[],
  allSourceAccounts: string[]
): {
  mainRecipients: any[]
  allFeeOperations: any[]
} {
  const mainRecipients = destinationOperations.filter(
    (op) => !op.metadata?.source && !allSourceAccounts.includes(op.accountAlias)
  )

  const allFeeOperations = destinationOperations.filter(
    (op) => op.metadata?.source || allSourceAccounts.includes(op.accountAlias)
  )

  return { mainRecipients, allFeeOperations }
}

function processFeeOperations(
  validFeeOperations: any[],
  feeRules: FeeRule[],
  intl?: IntlShape
): {
  appliedFees: AppliedFee[]
  deductibleFees: number
  nonDeductibleFees: number
} {
  let deductibleFees = 0
  let nonDeductibleFees = 0
  const appliedFees: AppliedFee[] = []

  validFeeOperations.forEach((operation) => {
    const matchedRule = feeRules.find(
      (rule) =>
        rule.creditAccount === operation.accountAlias ||
        rule.creditAccount.replace('@', '') ===
          operation.accountAlias.replace('@', '')
    )

    const feeAmount = Number(operation.amount?.value || '0')
    const isDeductible = matchedRule?.isDeductibleFrom || false

    if (isDeductible) {
      deductibleFees += feeAmount
    } else {
      nonDeductibleFees += feeAmount
    }

    appliedFees.push({
      feeId: matchedRule?.feeId || `fee-${appliedFees.length + 1}`,
      feeLabel:
        matchedRule?.feeLabel ||
        (intl
          ? intl.formatMessage(
              {
                id: 'fees.collectedByTemplate',
                defaultMessage: 'Fee collected by {accountAlias}'
              },
              { accountAlias: operation.accountAlias }
            )
          : `Fee collected by ${operation.accountAlias}`),
      calculatedAmount: feeAmount,
      isDeductibleFrom: isDeductible,
      creditAccount: operation.accountAlias,
      priority: matchedRule?.priority || 0
    })
  })

  return { appliedFees, deductibleFees, nonDeductibleFees }
}

export const extractFeeStateFromCalculation = (
  calculationResponse: FeeApiCalculateResponse,
  originalFormValues: TransactionFormValues,
  intl?: IntlShape
): FeeCalculationState => {
  if (!isFeeApiSuccessResponse(calculationResponse)) {
    throw new Error(
      intl
        ? intl.formatMessage({
            id: 'fees.invalidCalculationResponse',
            defaultMessage: 'Invalid calculation response'
          })
        : 'Invalid calculation response'
    )
  }

  const feeData = calculationResponse.transaction
  const feeRules = feeData.feeRules || []
  const sourceOperations = feeData.send?.source?.from || []
  const destinationOperations = feeData.send?.distribute?.to || []

  const { originalAmount, originalCurrency } =
    extractTransactionAmounts(originalFormValues)
  const {
    allSourceAccounts,
    allDestinationAccounts: _allDestinationAccounts,
    sourceAccount,
    destinationAccount
  } = extractAccountInformation(originalFormValues)

  const { mainRecipients, allFeeOperations } = categorizeOperations(
    destinationOperations,
    allSourceAccounts
  )

  const feeValidationService = createFeeValidationService()
  const transactionAccounts = getTransactionAccounts(
    sourceOperations,
    mainRecipients // Include all main recipients, exclude fee operations
  )

  const validFeeOperations = feeValidationService.filterValidFeeOperations(
    allFeeOperations,
    transactionAccounts
  )

  const { appliedFees, deductibleFees, nonDeductibleFees } =
    processFeeOperations(validFeeOperations, feeRules, intl)

  const totalFees = deductibleFees + nonDeductibleFees
  const sourcePaysAmount = originalAmount + nonDeductibleFees
  const destinationReceivesAmount = originalAmount - deductibleFees

  return {
    originalAmount,
    originalCurrency,
    sourceAccount,
    destinationAccount,
    deductibleFees,
    nonDeductibleFees,
    totalFees,
    appliedFees: appliedFees.sort((a, b) => a.priority - b.priority),
    sourcePaysAmount,
    destinationReceivesAmount,
    packageId: feeData.metadata?.packageAppliedID,
    packageLabel: intl
      ? intl.formatMessage({
          id: 'fees.packageDefault',
          defaultMessage: 'Fee Package'
        })
      : 'Fee Package',
    calculatedAt: new Date()
  }
}

export const createEnrichedTransactionMetadata = (
  originalMetadata: any,
  feeState: FeeCalculationState
) => ({
  ...originalMetadata,
  feeCalculationData: JSON.stringify(feeState),
  feeDataSource: 'calculation'
})

export const extractFeeStateFromTransaction = (
  transaction: any
): FeeCalculationState | null => {
  try {
    if (transaction.metadata?.feeCalculationData) {
      const feeState = JSON.parse(transaction.metadata.feeCalculationData)
      return {
        ...feeState,
        calculatedAt: new Date(feeState.calculatedAt)
      }
    }

    if (transaction.metadata?.feeCalculation) {
      return {
        ...transaction.metadata.feeCalculation,
        calculatedAt: new Date(transaction.metadata.feeCalculation.calculatedAt)
      }
    }

    // This is a simplified reconstruction - in a real implementation,

    return null // For now, return null if no enriched data available
  } catch (error) {
    console.warn(
      'Failed to parse fee calculation data from transaction metadata:',
      error
    )
    return null
  }
}

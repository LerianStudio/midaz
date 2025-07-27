import { useMemo } from 'react'
import { useIntl } from 'react-intl'
import { AppliedFee } from './use-fee-calculations'

export interface FeeDisplayItem {
  label: string
  value: string
  isFee?: boolean
  isDeductible?: boolean
  creditAccount?: string
  className?: string
}

export interface FeeDisplayConfig {
  showDeductibleSeparately: boolean
  showSourcePays: boolean
  showDestinationReceives: boolean
  highlightFees: boolean
}

const defaultConfig: FeeDisplayConfig = {
  showDeductibleSeparately: true,
  showSourcePays: true,
  showDestinationReceives: true,
  highlightFees: true
}

export const useFeeDisplay = (
  originalAmount: string,
  totalFees: string,
  appliedFees: AppliedFee[],
  deductibleFees: string,
  nonDeductibleFees: string,
  sourcePaysAmount: string,
  destinationReceivesAmount: string,
  currency: string,
  config: Partial<FeeDisplayConfig> = {}
) => {
  const intl = useIntl()
  const mergedConfig = { ...defaultConfig, ...config }

  return useMemo(() => {
    const items: FeeDisplayItem[] = []

    items.push({
      label: intl.formatMessage({
        id: 'transactions.originalAmount',
        defaultMessage: 'Original amount'
      }),
      value: `${currency} ${originalAmount}`,
      className: 'font-medium'
    })

    if (appliedFees.length > 0) {
      appliedFees.forEach((fee) => {
        items.push({
          label: fee.feeLabel,
          value: `${currency} ${fee.calculatedAmount}`,
          isFee: true,
          isDeductible: fee.isDeductibleFrom,
          creditAccount: fee.creditAccount,
          className: mergedConfig.highlightFees ? 'text-muted-foreground' : ''
        })
      })
    }

    if (mergedConfig.showDeductibleSeparately && appliedFees.length > 1) {
      if (Number(deductibleFees) > 0) {
        items.push({
          label: intl.formatMessage({
            id: 'transactions.fees.deductedFromDestination',
            defaultMessage: 'Fee deducted from recipient'
          }),
          value: `${currency} ${deductibleFees}`,
          className: 'font-medium text-orange-600'
        })
      }

      if (Number(nonDeductibleFees) > 0) {
        items.push({
          label: intl.formatMessage({
            id: 'transactions.fees.chargedToSource',
            defaultMessage: 'Fee charged to source'
          }),
          value: `${currency} ${nonDeductibleFees}`,
          className: 'font-medium text-orange-600'
        })
      }
    }

    if (Number(totalFees) > 0) {
      items.push({
        label: intl.formatMessage({
          id: 'transactions.fees.total',
          defaultMessage: 'Total Fees'
        }),
        value: `${currency} ${totalFees}`,
        className: 'font-semibold'
      })
    }

    if (mergedConfig.showSourcePays) {
      items.push({
        label: intl.formatMessage({
          id: 'fees.sourcePays',
          defaultMessage: 'Source pays'
        }),
        value: `${currency} ${sourcePaysAmount}`,
        className: 'font-semibold text-primary'
      })
    }

    if (mergedConfig.showDestinationReceives) {
      items.push({
        label: intl.formatMessage({
          id: 'fees.destinationReceives',
          defaultMessage: 'Destination receives'
        }),
        value: `${currency} ${destinationReceivesAmount}`,
        className: 'font-semibold text-primary'
      })
    }

    return items
  }, [
    originalAmount,
    totalFees,
    appliedFees,
    deductibleFees,
    nonDeductibleFees,
    sourcePaysAmount,
    destinationReceivesAmount,
    currency,
    intl,
    mergedConfig
  ])
}

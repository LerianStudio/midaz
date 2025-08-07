'use client'

import React, { useEffect, useState } from 'react'
import { UnifiedFeeDisplay } from '@/components/transactions/unified-fee-display'
import { FeeCalculationState } from '@/types/fee-calculation.types'
import { getCachedOrRecalculatedFees } from '@/utils/fee-recalculation'
import { Loader2 } from 'lucide-react'

interface SummaryFeeDisplayProps {
  transaction: any
  organizationId: string
  ledgerId: string
  showExplanations?: boolean
}

export const SummaryFeeDisplay: React.FC<SummaryFeeDisplayProps> = ({
  transaction,
  organizationId,
  ledgerId,
  showExplanations = false
}) => {
  const [feeState, setFeeState] = useState<FeeCalculationState | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const loadFeeData = async () => {
      try {
        setIsLoading(true)
        setError(null)

        if (!transaction.metadata?.packageAppliedID) {
          setFeeState(null)
          setIsLoading(false)
          return
        }

        const calculatedFeeState = await getCachedOrRecalculatedFees(
          transaction,
          organizationId,
          ledgerId
        )

        setFeeState(calculatedFeeState)
      } catch (err) {
        console.error('Failed to load fee data for summary:', err)
        setError('Failed to load fee information')
        setFeeState(null)
      } finally {
        setIsLoading(false)
      }
    }

    if (organizationId && ledgerId && transaction) {
      loadFeeData()
    }
  }, [transaction, organizationId, ledgerId])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-4">
        <Loader2 className="h-4 w-4 animate-spin" />
        <span className="ml-2 text-sm text-gray-500">
          Loading fee information...
        </span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="py-2">
        <span className="text-sm text-red-600">{error}</span>
      </div>
    )
  }

  if (!feeState) {
    return null
  }

  return (
    <UnifiedFeeDisplay
      feeState={feeState}
      showExplanations={showExplanations}
    />
  )
}

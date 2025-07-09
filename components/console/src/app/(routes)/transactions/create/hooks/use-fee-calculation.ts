'use client'

import { postFetcher } from '@/lib/fetcher'
import { useMutation, UseMutationResult } from '@tanstack/react-query'
import { TransactionFormSchema } from '../schemas'

export type UseFeeCalculationProps = {
  organizationId: string
  ledgerId: string
  onSuccess?: (data: any) => void
  onError?: (error: Error) => void
}

export type FeeCalculationRequest = {
  transaction: TransactionFormSchema
}

export type FeeCalculationResponse = {
  // This should match the response structure from plugin-fees service
  // For now, we'll leave it as any until we understand the response format better
  [key: string]: any
}

export const useFeeCalculation = ({
  organizationId,
  ledgerId,
  ...options
}: UseFeeCalculationProps): UseMutationResult<
  FeeCalculationResponse,
  Error,
  FeeCalculationRequest
> => {
  return useMutation<FeeCalculationResponse, Error, FeeCalculationRequest>({
    mutationKey: ['fees', 'calculate'],
    mutationFn: postFetcher(
      `/api/organizations/${organizationId}/ledgers/${ledgerId}/fees/calculate`
    ),
    ...options
  })
}

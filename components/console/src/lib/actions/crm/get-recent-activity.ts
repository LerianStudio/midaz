'use server'

import { authActionClient } from '../safe-action'
import { handleActionError } from '../utils'
import { z } from 'zod'

export interface CRMActivity {
  id: string
  type:
    | 'customer_created'
    | 'customer_updated'
    | 'account_linked'
    | 'account_unlinked'
  customerId: string
  customerName: string
  customerType: 'natural' | 'legal'
  description: string
  timestamp: Date
  metadata?: {
    accountId?: string
    accountName?: string
    previousValue?: string
    newValue?: string
  }
}

const getRecentActivitySchema = z.object({
  organizationId: z.string(),
  ledgerId: z.string(),
  limit: z.number().min(1).max(50).default(10)
})

export const getRecentActivity = authActionClient
  .schema(getRecentActivitySchema)
  .action(async ({ parsedInput, ctx }) => {
    try {
      // TODO: Replace with actual CRM API call once available
      // For now, returning empty array
      const activities: CRMActivity[] = []

      return activities
    } catch (error) {
      handleActionError(error, 'Failed to fetch CRM recent activity')
    }
  })

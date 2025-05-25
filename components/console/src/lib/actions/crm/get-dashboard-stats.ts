'use server'

import { authActionClient } from '../safe-action'
import { handleActionError } from '../utils'
import { z } from 'zod'

// Define the stats response type
export interface CRMDashboardStats {
  totalCustomers: number
  individualCustomers: number
  corporateCustomers: number
  accountLinks: number
  monthlyGrowth: {
    totalCustomers: number
    individualCustomers: number
    corporateCustomers: number
    accountLinks: number
  }
}

const getDashboardStatsSchema = z.object({
  organizationId: z.string(),
  ledgerId: z.string()
})

export const getDashboardStats = authActionClient
  .schema(getDashboardStatsSchema)
  .action(async ({ parsedInput, ctx }) => {
    try {
      // TODO: Replace with actual CRM API call once available
      // For now, returning mock data that matches the UI
      const mockStats: CRMDashboardStats = {
        totalCustomers: 1247,
        individualCustomers: 892,
        corporateCustomers: 355,
        accountLinks: 2103,
        monthlyGrowth: {
          totalCustomers: 12,
          individualCustomers: 8,
          corporateCustomers: 24,
          accountLinks: 15
        }
      }

      return mockStats
    } catch (error) {
      handleActionError(error, 'Failed to fetch CRM dashboard stats')
    }
  })

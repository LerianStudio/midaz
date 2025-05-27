'use server'

import { authActionClient } from '../safe-action'
import { handleActionError } from '../utils'
import { z } from 'zod'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { FetchAllHolders, FetchAllHoldersUseCase } from '@/core/application/use-cases/crm/holders/fetch-all-holders-use-case'
import { FetchAllAliases, FetchAllAliasesUseCase } from '@/core/application/use-cases/crm/aliases/fetch-all-aliases-use-case'

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
      const { organizationId } = parsedInput
      
      // Fetch all holders to calculate stats
      const fetchAllHoldersUseCase = container.get<FetchAllHolders>(
        FetchAllHoldersUseCase
      )
      
      // Fetch with maximum allowed limit
      const holdersResult = await fetchAllHoldersUseCase.execute(
        organizationId,
        100, // Maximum allowed limit by CRM API
        1
      )
      
      // Calculate customer stats
      const totalCustomers = holdersResult.total || holdersResult.items.length
      const individualCustomers = holdersResult.items.filter(
        holder => holder.type === 'NATURAL_PERSON'
      ).length
      const corporateCustomers = holdersResult.items.filter(
        holder => holder.type === 'LEGAL_PERSON'
      ).length
      
      // Fetch aliases count for account links
      let totalAliases = 0
      if (holdersResult.items.length > 0) {
        const fetchAllAliasesUseCase = container.get<FetchAllAliases>(
          FetchAllAliasesUseCase
        )
        
        // Fetch aliases for each holder and count them
        const aliasPromises = holdersResult.items.slice(0, 10).map(holder => // Limit to first 10 for performance
          fetchAllAliasesUseCase.execute(organizationId, holder.id, 1, 1)
            .then(result => result.total || 0)
            .catch(() => 0)
        )
        
        const aliasCounts = await Promise.all(aliasPromises)
        totalAliases = aliasCounts.reduce((sum, count) => sum + count, 0)
        
        // Estimate total if we only checked first 10 holders
        if (holdersResult.items.length > 10) {
          const avgAliasesPerHolder = totalAliases / 10
          totalAliases = Math.round(avgAliasesPerHolder * holdersResult.items.length)
        }
      }
      
      // For monthly growth, we'd need historical data which the API doesn't provide
      // So we'll return 0 for now (indicating no growth data available)
      const stats: CRMDashboardStats = {
        totalCustomers,
        individualCustomers,
        corporateCustomers,
        accountLinks: totalAliases,
        monthlyGrowth: {
          totalCustomers: 0,
          individualCustomers: 0,
          corporateCustomers: 0,
          accountLinks: 0
        }
      }

      return stats
    } catch (error) {
      handleActionError(error, 'Failed to fetch CRM dashboard stats')
    }
  })

import { getController } from '@/lib/http/server'
import { PortfolioController } from '@/core/application/controllers/portfolio-controller'

export const GET = getController(
  PortfolioController,
  (c) => c.fetchWithAccounts
)

import { getController } from '@/lib/http/server'
import { AccountController } from '@/core/application/controllers/account-controller'

export const GET = getController(
  AccountController,
  (c) => c.fetchWithPortfolios
)

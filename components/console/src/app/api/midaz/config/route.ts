import { getController } from '@/lib/http/server'
import { MidazConfigController } from '@/core/application/controllers/midaz-config-controller'

export const GET = getController(
  MidazConfigController,
  (c) => c.getConfigValidation
)

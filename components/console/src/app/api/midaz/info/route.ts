import { getController } from '@/lib/http/server'
import { MidazInfoController } from '@/core/application/controllers/midaz-info-controller'

export const GET = getController(MidazInfoController, (c) => c.getVersion)

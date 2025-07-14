import { getController } from '@/lib/http/server'
import { VersionController } from '@/core/application/controllers/version-controller'

export const GET = getController(VersionController, (c) => c.getVersion)

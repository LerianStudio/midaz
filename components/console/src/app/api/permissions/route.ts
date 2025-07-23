import { PermissionController } from '@/core/application/controllers/permission-controller'
import { getController } from '@/lib/http/server'

export const GET = getController(PermissionController, (c) => c.fetch)

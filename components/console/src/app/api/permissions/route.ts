import { getController } from '@/lib/http/server'
import { PermissionController } from '@/core/application/controllers/permission-controller'

export const dynamic = 'force-dynamic'

export const GET = getController(PermissionController, (c) => c.fetch)

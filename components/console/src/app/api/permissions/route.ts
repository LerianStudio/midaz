import { PermissionController } from '@/core/application/controllers/permission-controller'
import { getController } from '@/lib/http/server'

// export const dynamic = 'force-dynamic'
export const GET = getController(PermissionController, (c) => c.fetch)

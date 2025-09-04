import { getController } from '@/lib/http/server'
import { MidazMenuController } from '@/core/application/controllers/midaz-menu-controllers'

export const dynamic = 'force-dynamic'

export const GET = getController(MidazMenuController, (c) => c.getMidazMenus)

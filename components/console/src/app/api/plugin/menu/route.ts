import { getController } from '@/lib/http/server'
import { PluginMenuController } from '@/core/application/controllers/plugin-menu-controller'

export const dynamic = 'force-dynamic'

export const GET = getController(
  PluginMenuController,
  (c) => c.fetchAllPluginMenus
)

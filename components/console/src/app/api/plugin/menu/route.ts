import { getController } from '@/lib/http/server'
import { PluginMenuController } from '@/core/application/controllers/plugin-manifest-controller'

export const dynamic = 'force-dynamic'

export const POST = getController(PluginMenuController, (c) => c.addPluginMenu)

export const GET = getController(
  PluginMenuController,
  (c) => c.fetchAllPluginMenus
)

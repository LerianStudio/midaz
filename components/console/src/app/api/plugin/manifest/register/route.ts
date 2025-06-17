import { getController } from '@/lib/http/server'
import { PluginManifestController } from '@/core/application/controllers/plugin-manifest-controller'

export const dynamic = 'force-dynamic'

export const POST = getController(
  PluginManifestController,
  (c) => c.addPluginManifest
)

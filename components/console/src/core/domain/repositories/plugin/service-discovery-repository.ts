import { PluginManifestEntity } from '../../entities/plugin-manifest-entity'

export abstract class ServiceDiscoveryRepository {
  abstract fetchPluginManifest(
    pluginManifestUrl: string
  ): Promise<PluginManifestEntity>
}

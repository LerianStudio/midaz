import { PluginManifestEntity } from '../../entities/plugin-manifest-entity'

export abstract class PluginManifestRepository<T = unknown> {
  abstract create(
    pluginManifest: PluginManifestEntity
  ): Promise<PluginManifestEntity>
  abstract fetchAll(): Promise<PluginManifestEntity[]>
  abstract fetchById(
    pluginManifestId: string
  ): Promise<PluginManifestEntity | undefined>
  abstract update(
    pluginManifestId: string,
    pluginManifest: Partial<PluginManifestEntity>
  ): Promise<PluginManifestEntity>
  abstract delete(pluginManifestId: string): Promise<void>

  abstract readonly model: T
}

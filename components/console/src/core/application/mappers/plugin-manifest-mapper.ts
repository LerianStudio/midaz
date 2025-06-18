import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'
import type { PluginManifestDto } from '../dto/plugin-manifest-dto'

export class PluginManifestMapper {
  public static toResponseDto(
    pluginManifest: PluginManifestEntity
  ): PluginManifestDto {
    return {
      id: pluginManifest.id!,
      name: pluginManifest.name,
      title: pluginManifest.title,
      description: pluginManifest.description,
      version: pluginManifest.version,
      route: pluginManifest.route,
      icon: pluginManifest.icon,
      enabled: pluginManifest.enabled,
      entry: pluginManifest.entry,
      healthcheck: pluginManifest.healthcheck,
      host: pluginManifest.host,
      author: pluginManifest.author
    }
  }
}

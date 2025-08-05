import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'
import { PluginManifestDocument } from '../models/plugin-manifest'

export class MongoPluginManifestMapper {
  static toEntity(
    pluginManifestDocument: PluginManifestDocument
  ): PluginManifestEntity {
    return {
      id: pluginManifestDocument.id,
      name: pluginManifestDocument.name,
      title: pluginManifestDocument.title,
      description: pluginManifestDocument.description,
      version: pluginManifestDocument.version,
      route: pluginManifestDocument.route,
      entry: pluginManifestDocument.entry,
      healthcheck: pluginManifestDocument.healthcheck,
      host: pluginManifestDocument.host,
      icon: pluginManifestDocument.icon,
      enabled: pluginManifestDocument.enabled,
      author: pluginManifestDocument.author
    }
  }
}

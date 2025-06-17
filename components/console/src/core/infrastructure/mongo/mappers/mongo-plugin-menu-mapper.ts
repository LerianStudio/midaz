import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'
import { PluginManifestDocument } from '../models/plugin-manifest'

export class MongoPluginMenuMapper {
  static toEntity(
    pluginMenuDocument: PluginManifestDocument
  ): PluginManifestEntity {
    return {
      id: pluginMenuDocument.id,
      name: pluginMenuDocument.name,
      title: pluginMenuDocument.title,
      description: pluginMenuDocument.description,
      version: pluginMenuDocument.version,
      route: pluginMenuDocument.route,
      entry: pluginMenuDocument.entry,
      healthcheck: pluginMenuDocument.healthcheck,
      host: pluginMenuDocument.host,
      icon: pluginMenuDocument.icon,
      enabled: pluginMenuDocument.enabled,
      author: pluginMenuDocument.author
    }
  }
}

import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'

export class MongoPluginMenuMapper {
  static toEntity(pluginMenuDocument: any): PluginManifestEntity {
    return {
      id: pluginMenuDocument._id.toString(),
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

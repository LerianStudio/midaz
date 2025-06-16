import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'
import { PluginManifestRepository } from '@/core/domain/repositories/plugin/plugin-manifest-repository'
import { LoggerAggregator } from '@lerianstudio/lib-logs'
import { inject } from 'inversify'
import { handleDatabaseError } from '../../utils/database-error-handler'
import { MongoPluginMenuMapper } from '../mappers/mongo-plugin-menu-mapper'
import PluginMenu from '../models/plugin-manifest'
import { DBConfig } from '../mongo-config'
import { NotFoundApiException } from '@/lib/http'
import { NotFoundDatabaseException } from '../exceptions/database-exception'

export class MongoPluginManifestRepository
  implements PluginManifestRepository<typeof PluginMenu>
{
  constructor(
    @inject(LoggerAggregator)
    private readonly logger: LoggerAggregator,
    @inject(DBConfig)
    private readonly mongoConfig: DBConfig<typeof PluginMenu>
  ) {}

  public get model(): typeof PluginMenu {
    return PluginMenu
  }

  async create(
    pluginMenu: PluginManifestEntity
  ): Promise<PluginManifestEntity> {
    try {
      const pluginMenuDocument = new this.model(pluginMenu)

      return pluginMenuDocument.save()
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginMenuRepository.create', {
        error,
        context: 'mongo'
      })

      throw handleDatabaseError(error)
    }
  }

  async update(
    pluginMenuId: string,
    pluginMenu: PluginManifestEntity
  ): Promise<PluginManifestEntity> {
    try {
      const pluginMenuDocument = await this.model.findById(pluginMenuId)

      if (!pluginMenuDocument) {
        throw new NotFoundDatabaseException(
          'Plugin menu not found',
          'Plugin Menu'
        )
      }
      pluginMenuDocument.set(pluginMenu)

      return pluginMenuDocument.save()
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginMenuRepository.update', {
        error,
        context: 'mongo'
      })

      throw handleDatabaseError(error)
    }
  }

  async delete(pluginMenuId: string): Promise<void> {
    try {
      await this.model.findByIdAndDelete(pluginMenuId)
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginMenuRepository.delete', {
        error,
        context: 'mongo'
      })

      throw handleDatabaseError(error)
    }
  }

  async fetchById(
    pluginMenuId: string
  ): Promise<PluginManifestEntity | undefined> {
    try {
      const pluginMenuDocument = await this.model.findById(pluginMenuId)

      const pluginMenuEntity = pluginMenuDocument
        ? MongoPluginMenuMapper.toEntity(pluginMenuDocument)
        : undefined

      return pluginMenuEntity
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginMenuRepository.fetchById', {
        error,
        context: 'mongo'
      })

      throw handleDatabaseError(error)
    }
  }

  async fetchAll(): Promise<PluginManifestEntity[]> {
    try {
      const pluginMenuDocuments = await this.model.find()

      const pluginMenuEntities = pluginMenuDocuments.map(
        MongoPluginMenuMapper.toEntity
      )

      return pluginMenuEntities
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginMenuRepository.fetchAll', {
        error,
        context: 'mongo'
      })

      throw handleDatabaseError(error)
    }
  }
}

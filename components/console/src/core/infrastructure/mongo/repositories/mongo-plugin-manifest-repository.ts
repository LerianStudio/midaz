import { PluginManifestEntity } from '@/core/domain/entities/plugin-manifest-entity'
import { PluginManifestRepository } from '@/core/domain/repositories/plugin/plugin-manifest-repository'
import { LoggerAggregator } from '@lerianstudio/lib-logs'
import { inject } from 'inversify'
import { handleDatabaseError } from '../../utils/database-error-handler'
import { NotFoundDatabaseException } from '../exceptions/database-exception'
import { MongoPluginManifestMapper } from '../mappers/mongo-plugin-manifest-mapper'
import PluginManifest from '../models/plugin-manifest'
import { DBConfig } from '../mongo-config'

export class MongoPluginManifestRepository
  implements PluginManifestRepository<typeof PluginManifest>
{
  constructor(
    @inject(LoggerAggregator)
    private readonly logger: LoggerAggregator,
    @inject(DBConfig)
    private readonly mongoConfig: DBConfig<typeof PluginManifest>
  ) {}

  public get model(): typeof PluginManifest {
    return PluginManifest
  }

  async create(
    pluginManifest: PluginManifestEntity
  ): Promise<PluginManifestEntity> {
    try {
      const result = await this.model.create(pluginManifest)
      const pluginManifestEntity = MongoPluginManifestMapper.toEntity(result)

      return pluginManifestEntity
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginManifestRepository.create', {
        error,
        context: 'mongo'
      })

      throw await handleDatabaseError(error)
    }
  }

  async update(
    pluginManifestId: string,
    pluginManifest: Partial<PluginManifestEntity>
  ): Promise<PluginManifestEntity> {
    try {
      const pluginManifestDocument = await this.model.findById(pluginManifestId)

      if (!pluginManifestDocument) {
        throw new NotFoundDatabaseException(
          'Plugin manifest not found',
          'Plugin Manifest'
        )
      }

      pluginManifestDocument.set({ ...pluginManifest })

      const result = await pluginManifestDocument.save()
      const pluginManifestEntity = MongoPluginManifestMapper.toEntity(result)

      return pluginManifestEntity
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginManifestRepository.update', {
        error,
        context: 'mongo'
      })

      throw await handleDatabaseError(error)
    }
  }

  async delete(pluginManifestId: string): Promise<void> {
    try {
      await this.model.findByIdAndDelete(pluginManifestId)
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginManifestRepository.delete', {
        error,
        context: 'mongo'
      })

      throw await handleDatabaseError(error)
    }
  }

  async fetchById(
    pluginManifestId: string
  ): Promise<PluginManifestEntity | undefined> {
    try {
      const pluginManifestDocument = await this.model.findById(pluginManifestId)

      const pluginManifestEntity = pluginManifestDocument
        ? MongoPluginManifestMapper.toEntity(pluginManifestDocument)
        : undefined

      return pluginManifestEntity
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginManifestRepository.fetchById', {
        error,
        context: 'mongo'
      })

      throw await handleDatabaseError(error)
    }
  }

  async fetchAll(): Promise<PluginManifestEntity[]> {
    try {
      const pluginManifestDocuments = await this.model.find()

      const pluginManifestEntities = pluginManifestDocuments.map(
        MongoPluginManifestMapper.toEntity
      )

      return pluginManifestEntities
    } catch (error) {
      this.logger.error('[ERROR] - MongoPluginManifestRepository.fetchAll', {
        error,
        context: 'mongo'
      })

      throw await handleDatabaseError(error)
    }
  }
}

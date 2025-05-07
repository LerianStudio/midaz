/**
 * @file MongoDB Organization Avatar Repository
 * @description Implementation of the OrganizationAvatarRepository interface for MongoDB
 */

import { OrganizationAvatarEntity } from '@/core/domain/entities/organization-avatar-entity'
import { OrganizationAvatarRepository } from '@/core/domain/repositories/organization-avatar-repository'
import { inject, injectable } from 'inversify'
import { LoggerAggregator } from '../../logger/logger-aggregator'
import { handleDatabaseError } from '../../utils/database-error-handler'
import OrganizationAvatar from '../models/organization-avatar'
import { MongoConfig } from '../mongo-config'
import { OrganizationAvatarMapper } from '../mappers/mongo-organization-avatar-mapper'

/**
 * MongoOrganizationAvatarRepository handles CRUD operations for organization avatars in MongoDB.
 * @class
 * @implements {OrganizationAvatarRepository}
 * @description Provides MongoDB-specific implementation of the OrganizationAvatarRepository interface
 * for persisting and retrieving organization avatar data.
 */
@injectable()
export class MongoOrganizationAvatarRepository
  implements OrganizationAvatarRepository<typeof OrganizationAvatar>
{
  /**
   * Creates a new instance of the MongoOrganizationAvatarRepository.
   * @param logger - The logger instance for logging errors and other events.
   * @param mongoConfig - The MongoDB configuration instance.
   */
  constructor(
    @inject(LoggerAggregator)
    private readonly logger: LoggerAggregator,
    @inject(MongoConfig) private readonly mongoConfig: MongoConfig
  ) {}

  /**
   * Gets the Mongoose model for organization avatars.
   * @returns The OrganizationAvatar Mongoose model.
   * @public
   */
  public get model(): typeof OrganizationAvatar {
    return OrganizationAvatar
  }

  /**
   * Creates a new organization avatar document in MongoDB.
   * @param organizationAvatar - The organization avatar entity to persist.
   * @returns A promise that resolves to the persisted organization avatar entity.
   * @throws {DatabaseException} If the database operation fails.
   */
  async create(
    organizationAvatar: OrganizationAvatarEntity
  ): Promise<OrganizationAvatarEntity> {
    try {
      const result = await this.model.create(organizationAvatar)
      const organizationAvatarEntity = OrganizationAvatarMapper.toEntity(result)

      return organizationAvatarEntity
    } catch (error) {
      this.logger.error('[ERROR] - MongoOrganizationAvatarRepository.create', {
        error,
        context: 'mongo'
      })

      throw handleDatabaseError(error)
    }
  }

  /**
   * Updates an existing organization avatar document in MongoDB.
   * @param organizationAvatar - The organization avatar entity to update.
   * @returns A promise that resolves to the updated organization avatar entity.
   * @throws {DatabaseException} If the database operation fails.
   */
  async update(
    organizationAvatar: OrganizationAvatarEntity
  ): Promise<OrganizationAvatarEntity> {
    try {
      await this.model.updateOne(
        { organizationId: organizationAvatar.organizationId },
        organizationAvatar
      )

      return organizationAvatar as OrganizationAvatarEntity
    } catch (error) {
      this.logger.error('[ERROR] - MongoOrganizationAvatarRepository.update', {
        error,
        context: 'mongo'
      })

      throw handleDatabaseError(error)
    }
  }

  /**
   * Deletes the organization avatar document by organization ID.
   * @param organizationId - The ID of the organization whose avatar to delete.
   * @returns A Promise that resolves when deletion is complete.
   * @throws {DatabaseException} If the database operation fails.
   */
  async delete(organizationId: string): Promise<void> {
    try {
      await this.model.deleteOne({
        organizationId: organizationId
      })
    } catch (error) {
      this.logger.error('[ERROR] - MongoOrganizationAvatarRepository.delete', {
        error,
        context: 'mongo'
      })

      throw handleDatabaseError(error)
    }
  }

  /**
   * Retrieves the organization avatar by organization ID.
   * @param organizationId - The ID of the organization.
   * @returns A promise that resolves to the found organization avatar entity, or undefined if none exists.
   * @throws {DatabaseException} If the database operation fails.
   */
  async fetchById(
    organizationId: string
  ): Promise<OrganizationAvatarEntity | undefined> {
    try {
      const result = await this.model
        .findOne({
          organizationId
        })
        .lean()

      const organizationAvatarEntity = result
        ? OrganizationAvatarMapper.toEntity(result)
        : undefined

      return organizationAvatarEntity
    } catch (error) {
      this.logger.error(
        '[ERROR] - MongoOrganizationAvatarRepository.fetchById',
        {
          error,
          context: 'mongo'
        }
      )

      throw handleDatabaseError(error)
    }
  }
}

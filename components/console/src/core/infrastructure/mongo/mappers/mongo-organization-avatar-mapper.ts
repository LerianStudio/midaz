/**
 * @file Organization Avatar Mapper
 * @description Provides mapping functionality between domain entities and MongoDB documents
 * for organization avatars, following the Domain-Driven Design pattern.
 */

import { OrganizationAvatarEntity } from '@/core/domain/entities/organization-avatar-entity'
import { OrganizationAvatarDocument } from '../models/organization-avatar'

/**
 * OrganizationAvatarMapper
 * @class
 * @description Maps between domain entities and MongoDB documents for organization avatars.
 * This mapper ensures proper separation between domain and infrastructure layers
 * by providing clean transformation methods in both directions.
 */
export class OrganizationAvatarMapper {
  /**
   * Converts a MongoDB document to a domain entity
   * @param doc - The MongoDB document containing organization avatar data
   * @returns A clean domain entity with only the properties defined in the domain model
   */
  static toEntity(doc: OrganizationAvatarDocument): OrganizationAvatarEntity {
    return {
      organizationId: doc.organizationId,
      imageBase64: doc.imageBase64
    }
  }

  /**
   * Converts a domain entity to a MongoDB document properties object
   * @param entity - The domain entity containing organization avatar data
   * @returns An object with properties ready to be used in MongoDB operations
   */
  static toDomain(entity: OrganizationAvatarEntity) {
    return {
      organizationId: entity.organizationId,
      imageBase64: entity.imageBase64
    }
  }
}

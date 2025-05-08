/**
 * @file Organization Avatar Repository Interface
 * @description Defines the contract for organization avatar persistence operations
 * following the repository pattern in Domain-Driven Design.
 */

import { OrganizationAvatarEntity } from '../entities/organization-avatar-entity'

/**
 * Repository interface for organization avatar operations
 * @interface
 * @template T - The underlying model type used by the implementation
 * @description Defines the contract for CRUD operations on organization avatars.
 * This interface follows the repository pattern from Domain-Driven Design,
 * abstracting the persistence details from the domain layer.
 */
export abstract class OrganizationAvatarRepository<T = unknown> {
  /**
   * Creates a new organization avatar
   * @param organizationAvatar - The organization avatar entity to persist
   * @returns A promise that resolves to the persisted organization avatar entity
   */
  abstract create(
    organizationAvatar: OrganizationAvatarEntity
  ): Promise<OrganizationAvatarEntity>

  /**
   * Updates an existing organization avatar
   * @param organizationAvatar - The organization avatar entity with updated values
   * @returns A promise that resolves to the updated organization avatar entity
   */
  abstract update(
    organizationAvatar: OrganizationAvatarEntity
  ): Promise<OrganizationAvatarEntity>

  /**
   * Deletes an organization avatar by its organization ID
   * @param organizationAvatarId - The ID of the organization whose avatar to delete
   * @returns A promise that resolves when deletion is complete
   */
  abstract delete(organizationAvatarId: string): Promise<void>

  /**
   * Retrieves an organization avatar by its organization ID
   * @param organizationAvatarId - The ID of the organization
   * @returns A promise that resolves to the found organization avatar entity,
   * or undefined if none exists
   */
  abstract fetchById(
    organizationAvatarId: string
  ): Promise<OrganizationAvatarEntity | undefined>

  /**
   * The underlying model used by the repository implementation
   * This allows access to the model for specialized operations
   * while maintaining the repository abstraction
   */
  abstract readonly model: T
}

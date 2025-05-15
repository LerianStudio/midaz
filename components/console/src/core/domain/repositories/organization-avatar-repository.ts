import { OrganizationAvatarEntity } from '../entities/organization-avatar-entity'

export abstract class OrganizationAvatarRepository<T = unknown> {
  abstract create(
    organizationAvatar: OrganizationAvatarEntity
  ): Promise<OrganizationAvatarEntity>

  abstract update(
    organizationAvatar: OrganizationAvatarEntity
  ): Promise<OrganizationAvatarEntity>

  abstract delete(organizationAvatarId: string): Promise<void>

  abstract fetchById(
    organizationAvatarId: string
  ): Promise<OrganizationAvatarEntity | undefined>

  abstract fetchByOrganizationId(
    organizationIds: string[] | string
  ): Promise<OrganizationAvatarEntity[]>

  abstract readonly model: T
}

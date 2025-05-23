import { OrganizationEntity } from '../../entities/organization-entity'

export abstract class CreateOrganizationRepository {
  abstract create: (
    organization: OrganizationEntity
  ) => Promise<OrganizationEntity>
}

import { OrganizationEntity } from '../../entities/organization-entity'

export abstract class UpdateOrganizationRepository {
  abstract updateOrganization: (
    organizationId: string,
    organization: Partial<OrganizationEntity>
  ) => Promise<OrganizationEntity>
}

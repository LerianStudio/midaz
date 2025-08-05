import {
  OrganizationEntity,
  OrganizationSearchEntity
} from '../entities/organization-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class OrganizationRepository {
  abstract create: (
    organization: OrganizationEntity
  ) => Promise<OrganizationEntity>
  abstract fetchAll: (
    filters: OrganizationSearchEntity
  ) => Promise<PaginationEntity<OrganizationEntity>>
  abstract fetchById: (id: string) => Promise<OrganizationEntity>
  abstract update: (
    organizationId: string,
    organization: Partial<OrganizationEntity>
  ) => Promise<OrganizationEntity>
  abstract delete: (organizationId: string) => Promise<void>
}

import { OrganizationEntity } from '../../entities/organization-entity'
import { PaginationEntity } from '../../entities/pagination-entity'

export abstract class FetchAllOrganizationsRepository {
  abstract fetchAll: (
    limit: number,
    page: number
  ) => Promise<PaginationEntity<OrganizationEntity>>
}

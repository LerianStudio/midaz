import { PaginationEntity } from '../../entities/pagination-entity'
import { PortfolioEntity } from '../../entities/portfolios-entity'

export abstract class FetchAllPortfoliosRepository {
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<PortfolioEntity>>
}

import { PortfolioEntity } from '../../entities/portfolios-entity'

export abstract class FetchPortfolioByIdRepository {
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ) => Promise<PortfolioEntity>
}

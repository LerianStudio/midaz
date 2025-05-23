import { PortfolioEntity } from '../../entities/portfolios-entity'

export abstract class UpdatePortfolioRepository {
  abstract update: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string,
    portfolio: Partial<PortfolioEntity>
  ) => Promise<PortfolioEntity>
}

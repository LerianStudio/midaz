import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'

export abstract class CreatePortfolioRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    segment: PortfolioEntity
  ) => Promise<PortfolioEntity>
}

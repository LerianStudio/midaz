import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class PortfolioRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    segment: PortfolioEntity
  ) => Promise<PortfolioEntity>
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<PortfolioEntity>>
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ) => Promise<PortfolioEntity>
  abstract update: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string,
    portfolio: Partial<PortfolioEntity>
  ) => Promise<PortfolioEntity>
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ) => Promise<void>
}

import {
  PortfolioEntity,
  PortfolioSearchEntity
} from '@/core/domain/entities/portfolios-entity'
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
    filters: PortfolioSearchEntity
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
  abstract count: (
    organizationId: string,
    ledgerId: string
  ) => Promise<{ total: number }>
}

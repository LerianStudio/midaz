import { PaginationDto } from '../../dto/pagination-dto'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import { PortfolioDto } from '../../dto/portfolio-dto'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { type PortfolioSearchEntity } from '@/core/domain/entities/portfolios-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface FetchAllPortfolios {
  execute: (
    organizationId: string,
    ledgerId: string,
    filters: PortfolioSearchEntity
  ) => Promise<PaginationDto<PortfolioDto>>
}

@injectable()
export class FetchAllPortfoliosUseCase implements FetchAllPortfolios {
  constructor(
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    filters: PortfolioSearchEntity
  ): Promise<PaginationDto<PortfolioDto>> {
    const portfoliosResult = await this.portfolioRepository.fetchAll(
      organizationId,
      ledgerId,
      filters
    )

    return PortfolioMapper.toPaginationResponseDto(portfoliosResult)
  }
}

import { PaginationDto } from '../../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import { PortfolioDto } from '../../dto/portfolio-dto'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAllPortfolios {
  execute: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
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
    limit: number,
    page: number
  ): Promise<PaginationDto<PortfolioDto>> {
    const portfoliosResult: PaginationEntity<PortfolioEntity> =
      await this.portfolioRepository.fetchAll(
        organizationId,
        ledgerId,
        page,
        limit
      )

    return PortfolioMapper.toPaginationResponseDto(portfoliosResult)
  }
}

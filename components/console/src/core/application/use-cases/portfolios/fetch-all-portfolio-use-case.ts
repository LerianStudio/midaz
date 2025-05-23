import { PaginationDto } from '../../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import { PortfolioResponseDto } from '../../dto/portfolios-dto'
import { FetchAllPortfoliosRepository } from '@/core/domain/repositories/portfolios/fetch-all-portfolio-repository'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchAllPortfolios {
  execute: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationDto<PortfolioResponseDto>>
}

@injectable()
export class FetchAllPortfoliosUseCase implements FetchAllPortfolios {
  constructor(
    @inject(FetchAllPortfoliosRepository)
    private readonly fetchAllPortfoliosRepository: FetchAllPortfoliosRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<PortfolioResponseDto>> {
    const portfoliosResult: PaginationEntity<PortfolioEntity> =
      await this.fetchAllPortfoliosRepository.fetchAll(
        organizationId,
        ledgerId,
        page,
        limit
      )

    return PortfolioMapper.toPaginationResponseDto(portfoliosResult)
  }
}

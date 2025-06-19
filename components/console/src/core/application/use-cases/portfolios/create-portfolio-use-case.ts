import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import type { CreatePortfolioDto, PortfolioDto } from '../../dto/portfolio-dto'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface CreatePortfolio {
  execute: (
    organizationId: string,
    ledgerId: string,
    portfolio: CreatePortfolioDto
  ) => Promise<PortfolioDto>
}

@injectable()
export class CreatePortfolioUseCase implements CreatePortfolio {
  constructor(
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    portfolio: CreatePortfolioDto
  ): Promise<PortfolioDto> {
    const portfolioEntity: PortfolioEntity = PortfolioMapper.toDomain(portfolio)
    const portfolioCreated = await this.portfolioRepository.create(
      organizationId,
      ledgerId,
      portfolioEntity
    )

    return PortfolioMapper.toResponseDto(portfolioCreated)
  }
}

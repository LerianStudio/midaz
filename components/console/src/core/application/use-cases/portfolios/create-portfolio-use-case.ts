import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import type {
  CreatePortfolioDto,
  PortfolioResponseDto
} from '../../dto/portfolio-dto'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface CreatePortfolio {
  execute: (
    organizationId: string,
    ledgerId: string,
    portfolio: CreatePortfolioDto
  ) => Promise<PortfolioResponseDto>
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
  ): Promise<PortfolioResponseDto> {
    portfolio.status = {
      code: 'ACTIVE',
      description: 'Teste Portfolio'
    }
    const portfolioEntity: PortfolioEntity = PortfolioMapper.toDomain(portfolio)
    const portfolioCreated = await this.portfolioRepository.create(
      organizationId,
      ledgerId,
      portfolioEntity
    )

    return PortfolioMapper.toResponseDto(portfolioCreated)
  }
}

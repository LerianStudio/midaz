import { CreatePortfolioRepository } from '@/core/domain/repositories/portfolios/create-portfolio-repository'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import type {
  CreatePortfolioDto,
  PortfolioResponseDto
} from '../../dto/portfolios-dto'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(CreatePortfolioRepository)
    private readonly createPortfolioRepository: CreatePortfolioRepository
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
    const portfolioCreated = await this.createPortfolioRepository.create(
      organizationId,
      ledgerId,
      portfolioEntity
    )

    return PortfolioMapper.toResponseDto(portfolioCreated)
  }
}

import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import {
  CreatePortfolioDto,
  PortfolioResponseDto,
  UpdatePortfolioDto
} from '../../dto/portfolio-dto'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface UpdatePortfolio {
  execute: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string,
    portfolio: Partial<UpdatePortfolioDto>
  ) => Promise<PortfolioResponseDto>
}

@injectable()
export class UpdatePortfolioUseCase implements UpdatePortfolio {
  constructor(
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    portfolioId: string,
    portfolio: Partial<UpdatePortfolioDto>
  ): Promise<PortfolioResponseDto> {
    portfolio.status = {
      code: 'ACTIVE',
      description: 'Teste Portfolio'
    }
    const portfolioEntity: Partial<PortfolioEntity> = PortfolioMapper.toDomain(
      portfolio as CreatePortfolioDto
    )
    const updatedPortfolio: PortfolioEntity =
      await this.portfolioRepository.update(
        organizationId,
        ledgerId,
        portfolioId,
        portfolioEntity
      )

    return PortfolioMapper.toResponseDto(updatedPortfolio)
  }
}

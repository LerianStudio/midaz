import { UpdatePortfolioRepository } from '@/core/domain/repositories/portfolios/update-portfolio-repository'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import {
  CreatePortfolioDto,
  PortfolioResponseDto,
  UpdatePortfolioDto
} from '../../dto/portfolios-dto'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(UpdatePortfolioRepository)
    private readonly updatePortfolioRepository: UpdatePortfolioRepository
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
      await this.updatePortfolioRepository.update(
        organizationId,
        ledgerId,
        portfolioId,
        portfolioEntity
      )

    return PortfolioMapper.toResponseDto(updatedPortfolio)
  }
}

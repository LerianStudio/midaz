import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import { PortfolioDto } from '../../dto/portfolio-dto'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchPortfolioById {
  execute: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ) => Promise<PortfolioDto>
}

@injectable()
export class FetchPortfolioByIdUseCase implements FetchPortfolioById {
  constructor(
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<PortfolioDto> {
    const portfolio = await this.portfolioRepository.fetchById(
      organizationId,
      ledgerId,
      portfolioId
    )

    return PortfolioMapper.toResponseDto(portfolio)
  }
}

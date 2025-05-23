import { FetchPortfolioByIdRepository } from '@/core/domain/repositories/portfolios/fetch-portfolio-by-id-repository'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import { PortfolioResponseDto } from '../../dto/portfolios-dto'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchPortfolioById {
  execute: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ) => Promise<PortfolioResponseDto>
}

@injectable()
export class FetchPortfolioByIdUseCase implements FetchPortfolioById {
  constructor(
    @inject(FetchPortfolioByIdRepository)
    private readonly fetchPortfolioByIdRepository: FetchPortfolioByIdRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<PortfolioResponseDto> {
    const portfolio = await this.fetchPortfolioByIdRepository.fetchById(
      organizationId,
      ledgerId,
      portfolioId
    )

    return PortfolioMapper.toResponseDto(portfolio)
  }
}

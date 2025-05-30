import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeletePortfolio {
  execute: (
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ) => Promise<void>
}

@injectable()
export class DeletePortfolioUseCase implements DeletePortfolio {
  constructor(
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<void> {
    await this.portfolioRepository.delete(organizationId, ledgerId, portfolioId)
  }
}

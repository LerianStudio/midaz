import { DeletePortfolioRepository } from '@/core/domain/repositories/portfolios/delete-portfolio-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(DeletePortfolioRepository)
    private readonly deletePortfolioRepository: DeletePortfolioRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<void> {
    await this.deletePortfolioRepository.delete(
      organizationId,
      ledgerId,
      portfolioId
    )
  }
}

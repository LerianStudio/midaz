import { inject, injectable } from 'inversify'
import { HomeMetricsDto } from '../../dto/home-metrics-dto'
import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators'
import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { SegmentRepository } from '@/core/domain/repositories/segment-repository'

export interface FetchHomeMetrics {
  execute: (organizationId: string, ledgerId: string) => Promise<HomeMetricsDto> // HomeMetricsDto
}

@injectable()
export class FetchHomeMetricsUseCase implements FetchHomeMetrics {
  constructor(
    @inject(AccountRepository)
    private readonly accountRepository: AccountRepository,
    @inject(AssetRepository)
    private readonly assetRepository: AssetRepository,
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository,
    @inject(SegmentRepository)
    private readonly segmentRepository: SegmentRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string
  ): Promise<HomeMetricsDto> {
    const [accountsResult, assetsResult, portfoliosResult, segmentsResult] =
      await Promise.allSettled([
        this.accountRepository.count(organizationId, ledgerId),
        this.assetRepository.count(organizationId, ledgerId),
        this.portfolioRepository.count(organizationId, ledgerId),
        this.segmentRepository.count(organizationId, ledgerId)
      ])

    return {
      totalAccounts:
        accountsResult.status === 'fulfilled' ? accountsResult.value.total : 0,
      totalAssets:
        assetsResult.status === 'fulfilled' ? assetsResult.value.total : 0,
      totalPortfolios:
        portfoliosResult.status === 'fulfilled'
          ? portfoliosResult.value.total
          : 0,
      totalSegments:
        segmentsResult.status === 'fulfilled' ? segmentsResult.value.total : 0
    }
  }
}

import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { groupBy } from 'lodash'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { type PortfolioSearchEntity } from '@/core/domain/entities/portfolios-entity'

export interface FetchPortfoliosWithAccounts {
  execute: (
    organizationId: string,
    ledgerId: string,
    filters: PortfolioSearchEntity
  ) => Promise<PaginationDto<any>>
}

@injectable()
export class FetchPortfoliosWithAccountsUseCase
  implements FetchPortfoliosWithAccounts
{
  constructor(
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository,
    @inject(AccountRepository)
    private readonly accountRepository: AccountRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    filters: PortfolioSearchEntity
  ): Promise<PaginationDto<any>> {
    const portfoliosResult = await this.portfolioRepository.fetchAll(
      organizationId,
      ledgerId,
      filters
    )

    const allAccountsResult = await this.accountRepository.fetchAll(
      organizationId,
      ledgerId,
      {
        limit: 100,
        page: 1
      }
    )

    const accountsGrouped = groupBy(
      allAccountsResult.items,
      (account) => account.portfolioId || 'no_portfolio'
    )

    const portfoliosWithAccounts =
      portfoliosResult?.items?.map((portfolio) =>
        PortfolioMapper.toDtoWithAccounts(
          portfolio,
          accountsGrouped[portfolio.id!] ?? []
        )
      ) || []

    const responseDTO: PaginationDto<any> = {
      items: portfoliosWithAccounts,
      limit: portfoliosResult.limit,
      page: portfoliosResult.page
    }

    return responseDTO
  }
}

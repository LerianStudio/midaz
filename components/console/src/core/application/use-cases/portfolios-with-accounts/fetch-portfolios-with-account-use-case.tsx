import { FetchAllAccountsRepository } from '@/core/domain/repositories/accounts/fetch-all-accounts-repository'
import { FetchAllPortfoliosRepository } from '@/core/domain/repositories/portfolios/fetch-all-portfolio-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { AccountMapper } from '../../mappers/account-mapper'
import { inject, injectable } from 'inversify'
import { groupBy } from 'lodash'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchPortfoliosWithAccounts {
  execute: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationDto<any>>
}

@injectable()
export class FetchPortfoliosWithAccountsUseCase
  implements FetchPortfoliosWithAccounts
{
  constructor(
    @inject(FetchAllPortfoliosRepository)
    private readonly fetchAllPortfoliosRepository: FetchAllPortfoliosRepository,
    @inject(FetchAllAccountsRepository)
    private readonly fetchAllAccountsRepository: FetchAllAccountsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<any>> {
    const portfoliosResult = await this.fetchAllPortfoliosRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    const allAccountsResult = await this.fetchAllAccountsRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    const accountsGrouped = groupBy(
      allAccountsResult.items,
      (account) => account.portfolioId || 'no_portfolio'
    )

    const portfoliosWithAccounts =
      portfoliosResult?.items?.map((portfolio) =>
        PortfolioMapper.toDtoWithAccounts(
          portfolio,
          (accountsGrouped[portfolio.id!] || []).map(AccountMapper.toDto)
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

import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { groupBy } from 'lodash'
import { PortfolioMapper } from '../../mappers/portfolio-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

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
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository,
    @inject(AccountRepository)
    private readonly accountRepository: AccountRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<any>> {
    const portfoliosResult = await this.portfolioRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    const allAccountsResult = await this.accountRepository.fetchAll(
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

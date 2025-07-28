import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { PortfolioDto } from '../../dto/portfolio-dto'
import { AccountMapper } from '../../mappers/account-mapper'
import { inject, injectable } from 'inversify'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { BalanceMapper } from '../../mappers/balance-mapper'
import { LoggerAggregator } from '@lerianstudio/lib-logs'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export type AccountWithPortfolioParams = {
  organizationId: string
  ledgerId: string
  limit: number
  page: number
  alias?: string
}

export interface FetchAccountsWithPortfolios {
  execute: (
    params: AccountWithPortfolioParams
  ) => Promise<PaginationDto<PortfolioDto>>
}

@injectable()
export class FetchAccountsWithPortfoliosUseCase
  implements FetchAccountsWithPortfolios
{
  constructor(
    @inject(PortfolioRepository)
    private readonly portfolioRepository: PortfolioRepository,
    @inject(AccountRepository)
    private readonly accountRepository: AccountRepository,
    @inject(BalanceRepository)
    private readonly balanceRepository: BalanceRepository,
    @inject(LoggerAggregator)
    private readonly midazLogger: LoggerAggregator
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    params: AccountWithPortfolioParams
  ): Promise<PaginationDto<PortfolioDto>> {
    const accountsResult = await this.accountRepository.fetchAll(
      params.organizationId,
      params.ledgerId,
      params
    )

    if (!accountsResult?.items?.length) {
      return { items: [], limit: params.limit, page: params.page }
    }

    const portfolioMap = await this.fetchAndCreatePortfolioMap(
      params.organizationId,
      params.ledgerId,
      params.limit,
      params.page
    )

    let accountsWithPortfolio: any[] = []

    accountsWithPortfolio = await this.mergeAccountData(
      accountsResult.items,
      portfolioMap,
      params.organizationId,
      params.ledgerId
    )

    return {
      items: accountsWithPortfolio,
      limit: accountsResult?.limit || 0,
      page: accountsResult?.page || 0
    }
  }

  private async fetchAndCreatePortfolioMap(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<Map<string, PortfolioEntity>> {
    const portfoliosResult = await this.portfolioRepository.fetchAll(
      organizationId,
      ledgerId,
      {
        limit,
        page
      }
    )

    const portfolioMap = new Map<string, PortfolioEntity>()
    if (portfoliosResult?.items) {
      portfoliosResult.items.forEach((portfolio) => {
        if (portfolio && portfolio.id) {
          portfolioMap.set(portfolio.id, portfolio)
        }
      })
    }

    return portfolioMap
  }

  private async mergeAccountData(
    accounts: AccountEntity[],
    portfolioMap: Map<string, PortfolioEntity>,
    organizationId: string,
    ledgerId: string
  ): Promise<any[]> {
    return Promise.all(
      accounts.map(async (account) => {
        const portfolio = this.findRelatedPortfolio(account, portfolioMap)

        const balanceData = await this.fetchBalanceForAccount(
          account,
          organizationId,
          ledgerId
        )

        return this.createAccountWithPortfolioDto(
          account,
          portfolio,
          balanceData
        )
      })
    )
  }

  private findRelatedPortfolio(
    account: AccountEntity,
    portfolioMap: Map<string, PortfolioEntity>
  ): PortfolioEntity | null {
    return account?.portfolioId
      ? portfolioMap.get(account.portfolioId) || null
      : null
  }

  private async fetchBalanceForAccount(
    account: AccountEntity,
    organizationId: string,
    ledgerId: string
  ): Promise<Record<string, any>> {
    if (!account?.id) {
      return {}
    }

    try {
      const balances = await this.balanceRepository.getByAccountId(
        organizationId,
        ledgerId,
        account.id
      )

      const balanceItem = balances?.items?.[0]
      if (balanceItem) {
        return BalanceMapper.toDomain(balanceItem)
      }
      return {}
    } catch (error) {
      this.midazLogger.error({
        layer: 'application',
        operation: 'fetch_account_balance_failed',
        message: 'Error processing balance data for account',
        error,
        context: { accountId: account?.id, organizationId, ledgerId }
      })
      return {}
    }
  }

  private createAccountWithPortfolioDto(
    account: AccountEntity,
    portfolio: PortfolioEntity | null,
    balanceData: Record<string, any>
  ): any {
    const accountDto = AccountMapper.toDto({ ...account, ...balanceData })

    let portfolioInfo = null

    if (portfolio) {
      portfolioInfo = { id: portfolio.id || '', name: portfolio.name || '' }
    }

    return { ...accountDto, portfolio: portfolioInfo }
  }
}

import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { CreatePortfolioRepository } from '@/core/domain/repositories/portfolios/create-portfolio-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { LoggerAggregator } from '@/core/application/logger/logger-aggregator'

@injectable()
export class MidazCreatePortfolioRepository
  implements CreatePortfolioRepository
{
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils,
    @inject(LoggerAggregator)
    private readonly loggerAggregator: LoggerAggregator
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    portfolio: PortfolioEntity
  ): Promise<PortfolioEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<PortfolioEntity>({
        url,
        method: HTTP_METHODS.POST,
        body: JSON.stringify(portfolio)
      })

    return response
  }
}

import { UpdatePortfolioRepository } from '@/core/domain/repositories/portfolios/update-portfolio-repository'
import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'
import { injectable, inject } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazUpdatePortfolioRepository
  implements UpdatePortfolioRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  async update(
    organizationId: string,
    ledgerId: string,
    portfolioId: string,
    portfolio: Partial<PortfolioEntity>
  ): Promise<PortfolioEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<PortfolioEntity>({
        url,
        method: HTTP_METHODS.PATCH,
        body: JSON.stringify(portfolio)
      })

    return response
  }
}

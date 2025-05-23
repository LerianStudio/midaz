import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { FetchPortfolioByIdRepository } from '@/core/domain/repositories/portfolios/fetch-portfolio-by-id-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchPortfolioByIdRepository
  implements FetchPortfolioByIdRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  async fetchById(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<PortfolioEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<PortfolioEntity>({
        url,
        method: HTTP_METHODS.GET
      })

    return response
  }
}

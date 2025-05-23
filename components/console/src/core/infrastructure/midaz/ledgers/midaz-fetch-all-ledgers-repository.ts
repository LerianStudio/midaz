import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { FetchAllLedgersRepository } from '@/core/domain/repositories/ledgers/fetch-all-ledgers-repository'
import { injectable } from 'inversify'
import { inject } from 'inversify'
import { HttpFetchUtils, HTTP_METHODS } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchAllLedgersRepository
  implements FetchAllLedgersRepository
{
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchAll(
    organizationId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<LedgerEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers?limit=${limit}&page=${page}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      PaginationEntity<LedgerEntity>
    >({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}

import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { FetchAllAccountsRepository } from '@/core/domain/repositories/accounts/fetch-all-accounts-repository'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils, HTTP_METHODS } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchAllAccountsRepository
  implements FetchAllAccountsRepository
{
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<AccountEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts?limit=${limit}&page=${page}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      PaginationEntity<AccountEntity>
    >({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}

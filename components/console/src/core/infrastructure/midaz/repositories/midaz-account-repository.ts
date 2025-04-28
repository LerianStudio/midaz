import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HttpMethods } from '@/lib/http'

@injectable()
export class MidazAccountRepository implements AccountRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    account: AccountEntity
  ): Promise<AccountEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<AccountEntity>({
        url,
        method: HttpMethods.POST,
        body: JSON.stringify(account)
      })

    return response
  }

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
      method: HttpMethods.GET
    })

    return response
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<AccountEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<AccountEntity>({
        url,
        method: HttpMethods.GET
      })

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    accountId: string,
    account: Partial<AccountEntity>
  ): Promise<AccountEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<AccountEntity>({
        url,
        method: HttpMethods.PATCH,
        body: JSON.stringify(account)
      })

    return response
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`
    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HttpMethods.DELETE
    })

    return
  }
}

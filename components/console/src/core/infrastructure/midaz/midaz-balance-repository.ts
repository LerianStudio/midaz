import { BalanceEntity } from '@/core/domain/entities/balance-entity'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { inject, injectable } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../container-registry/midaz-http-fetch-module'
import { HTTP_METHODS, HttpFetchUtils } from '../utils/http-fetch-utils'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'

@injectable()
export class MidazBalanceRepository implements BalanceRepository {
  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async getByAccountId(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<PaginationEntity<BalanceEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}/balances`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      PaginationEntity<BalanceEntity>
    >({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    accountId: string,
    balance: BalanceEntity
  ): Promise<BalanceEntity> {
    const balanceResponse = await this.getByAccountId(
      organizationId,
      ledgerId,
      accountId
    )

    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/balances/${balanceResponse?.items[0]?.id}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<BalanceEntity>({
        url,
        method: HTTP_METHODS.PATCH,
        body: JSON.stringify(balance)
      })

    return response
  }
}

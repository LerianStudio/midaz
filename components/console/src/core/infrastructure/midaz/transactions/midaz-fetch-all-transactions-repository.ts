import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import { FetchAllTransactionsRepository } from '@/core/domain/repositories/transactions/fetch-all-transactions-repository'
import { inject, injectable } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'

@injectable()
export class MidazFetchAllTransactionsRepository
  implements FetchAllTransactionsRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<TransactionEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions?limit=${limit}&page=${page}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      PaginationEntity<TransactionEntity>
    >({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}

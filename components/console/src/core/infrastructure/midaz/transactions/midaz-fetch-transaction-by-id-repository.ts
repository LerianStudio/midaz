import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import { FetchTransactionByIdRepository } from '@/core/domain/repositories/transactions/fetch-transaction-by-id-repository'
import { inject, injectable } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'

@injectable()
export class MidazFetchTransactionByIdRepository
  implements FetchTransactionByIdRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string

  async fetchById(
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ): Promise<TransactionEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<TransactionEntity>({
        url,
        method: HTTP_METHODS.GET
      })

    return response
  }
}

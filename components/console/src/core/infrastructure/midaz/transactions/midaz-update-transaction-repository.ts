import { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import { UpdateTransactionRepository } from '@/core/domain/repositories/transactions/update-transaction-repository'
import { inject, injectable } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'

@injectable()
export class MidazUpdateTransactionRepository
  implements UpdateTransactionRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string
  async update(
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<TransactionEntity>
  ): Promise<TransactionEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<TransactionEntity>({
        url,
        method: HTTP_METHODS.PATCH,
        body: JSON.stringify(transaction)
      })

    return response
  }
}

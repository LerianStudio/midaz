import {
  TransactionCreateEntity,
  TransactionEntity
} from '@/core/domain/entities/transaction-entity'
import { CreateTransactionRepository } from '@/core/domain/repositories/transactions/create-transaction-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'
import { inject, injectable } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'

@injectable()
export class MidazCreateTransactionRepository
  implements CreateTransactionRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string
  async create(
    organizationId: string,
    ledgerId: string,
    transaction: TransactionCreateEntity
  ): Promise<TransactionEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/json`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<TransactionEntity>({
        url,
        method: HTTP_METHODS.POST,
        body: JSON.stringify(transaction)
      })

    return response
  }
}

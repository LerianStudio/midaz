import {
  TransactionCreateEntity,
  TransactionEntity
} from '@/core/domain/entities/transaction-entity'
import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { inject, injectable } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'

@injectable()
export class MidazTransactionRepository implements TransactionRepository {
  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string
  async create(
    organizationId: string,
    ledgerId: string,
    transaction: TransactionCreateEntity
  ): Promise<TransactionEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/json`

    const response = await this.httpService.post<TransactionEntity>(url, {
      body: JSON.stringify(transaction)
    })

    return response
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<TransactionEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions?limit=${limit}&page=${page}`

    const response =
      await this.httpService.get<PaginationEntity<TransactionEntity>>(url)

    return response
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ): Promise<TransactionEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`

    const response = await this.httpService.get<TransactionEntity>(url)

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<TransactionEntity>
  ): Promise<TransactionEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`

    const response = await this.httpService.patch<TransactionEntity>(url, {
      body: JSON.stringify(transaction)
    })

    return response
  }
}

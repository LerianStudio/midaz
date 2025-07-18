import {
  TransactionEntity,
  TransactionSearchEntity
} from '@/core/domain/entities/transaction-entity'
import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { inject, injectable } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazTransactionMapper } from '../mappers/midaz-transaction-mapper'
import { MidazTransactionDto } from '../dto/midaz-transaction-dto'
import { createQueryString } from '@/lib/search'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'

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
    transaction: TransactionEntity
  ): Promise<TransactionEntity> {
    const dto = MidazTransactionMapper.toCreateDto(transaction)
    const response = await this.httpService.post<MidazTransactionDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/json`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazTransactionMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    filters: TransactionSearchEntity = { limit: 10, page: 1 }
  ): Promise<PaginationEntity<TransactionEntity>> {
    if (filters.id) {
      try {
        const response = await this.fetchById(
          organizationId,
          ledgerId,
          filters.id
        )

        return {
          items: [response],
          limit: filters.limit ?? 10,
          page: filters.page ?? 1
        }
      } catch (error) {
        return {
          items: [],
          limit: filters.limit ?? 10,
          page: filters.page ?? 1
        }
      }
    }

    const response = await this.httpService.get<
      MidazPaginationDto<MidazTransactionDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions${createQueryString(filters)}`
    )

    return MidazTransactionMapper.toPaginationEntity(response)
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ): Promise<TransactionEntity> {
    const response = await this.httpService.get<MidazTransactionDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`
    )
    return MidazTransactionMapper.toEntity(response)
  }

  async update(
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<TransactionEntity>
  ): Promise<TransactionEntity> {
    const dto = MidazTransactionMapper.toUpdateDto(transaction)
    const response = await this.httpService.patch<MidazTransactionDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazTransactionMapper.toEntity(response)
  }
}

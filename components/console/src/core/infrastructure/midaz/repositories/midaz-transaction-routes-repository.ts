import {
  TransactionRoutesEntity,
  TransactionRoutesSearchEntity
} from '@/core/domain/entities/transaction-routes-entity'
import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import { injectable, inject } from 'inversify'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazTransactionRoutesDto } from '../dto/midaz-transaction-routes-dto'
import { MidazCursorPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazTransactionRoutesMapper } from '../mappers/midaz-transaction-routes-mapper'
import { createQueryString } from '@/lib/search'
import { MidazApiException } from '../exceptions/midaz-exceptions'

@injectable()
export class MidazTransactionRoutesRepository
  implements TransactionRoutesRepository
{
  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    transactionRoute: TransactionRoutesEntity
  ): Promise<TransactionRoutesEntity> {
    const dto = MidazTransactionRoutesMapper.toCreateDto(transactionRoute)

    const response = await this.httpService.post<MidazTransactionRoutesDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes`,
      {
        body: JSON.stringify(dto)
      }
    )

    return MidazTransactionRoutesMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchEntity
  ): Promise<CursorPaginationEntity<TransactionRoutesEntity>> {
    const {
      id,
      cursor,
      limit = 10,
      sortBy = 'createdAt',
      sortOrder = 'desc'
    } = query ?? {}

    const queryParams = createQueryString({
      id,
      cursor,
      limit: limit.toString(),
      sort_by: sortBy,
      sort_order: sortOrder
    })

    const response = await this.httpService.get<
      MidazCursorPaginationDto<MidazTransactionRoutesDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes?${queryParams}`
    )

    return MidazTransactionRoutesMapper.toCursorPaginationEntity({
      ...response,
      items: response.items
    })
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string
  ): Promise<TransactionRoutesEntity> {
    try {
      const response = await this.httpService.get<MidazTransactionRoutesDto>(
        `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes/${transactionRouteId}`
      )

      return MidazTransactionRoutesMapper.toEntity(response)
    } catch (error) {
      if (error instanceof MidazApiException) {
        if (error.code === '0105') {
          return {} as TransactionRoutesEntity
        }
      }
      throw error
    }
  }

  async update(
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string,
    transactionRoute: Partial<TransactionRoutesEntity>
  ): Promise<TransactionRoutesEntity> {
    const dto = MidazTransactionRoutesMapper.toUpdateDto(transactionRoute)

    const response = await this.httpService.patch<MidazTransactionRoutesDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes/${transactionRouteId}`,
      {
        body: JSON.stringify(dto)
      }
    )

    return MidazTransactionRoutesMapper.toEntity(response)
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string
  ): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/transaction-routes/${transactionRouteId}`
    )
  }
}

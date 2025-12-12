import { CursorPaginationDto } from '../../dto/pagination-dto'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import { TransactionRoutesMapper } from '../../mappers/transaction-routes-mapper'
import { TransactionRoutesEntity } from '@/core/domain/entities/transaction-routes-entity'
import {
  TransactionRoutesDto,
  type TransactionRoutesSearchParamDto
} from '../../dto/transaction-routes-dto'
import { TransactionRoutesRepository } from '@/core/domain/repositories/transaction-routes-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface FetchAllTransactionRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchParamDto
  ) => Promise<CursorPaginationDto<TransactionRoutesDto>>
}

@injectable()
export class FetchAllTransactionRoutesUseCase implements FetchAllTransactionRoutes {
  constructor(
    @inject(TransactionRoutesRepository)
    private readonly transactionRoutesRepository: TransactionRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchParamDto
  ): Promise<CursorPaginationDto<TransactionRoutesDto>> {
    // Map DTO to domain search entity
    const searchEntity = TransactionRoutesMapper.toSearchDomain(query || {})

    const transactionRoutesResult: CursorPaginationEntity<TransactionRoutesEntity> =
      await this.transactionRoutesRepository.fetchAll(
        organizationId,
        ledgerId,
        searchEntity
      )

    const result = TransactionRoutesMapper.toCursorPaginationResponseDto(
      transactionRoutesResult
    )

    return result
  }
}
